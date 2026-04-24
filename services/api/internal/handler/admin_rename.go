// services/api/internal/handler/admin_rename.go
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// PostAdminRenameUser triggers a per-user+type rename executed in the background.
// The foreground resolves schema + library-dir snapshots so that admin mid-job
// changes do not affect an already-started rename job.
// Relies on the unique partial index idx_bg_jobs_one_running (user_id, job_type)
// WHERE status='running' — duplicate INSERT returns 23505 → 200 + rename_in_progress.
// POST /api/v1/admin/users/{id}/files/{type}/rename-execute
func (h *Handler) PostAdminRenameUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	fileType := chi.URLParam(r, "type")

	if !isAllowedFileType(fileType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown file type", r.URL.Path)
		return
	}
	if h.Config == nil || h.Config.StorageBackend != "local" {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"renamed": 0,
			"note":    "rename not applicable for non-local backends",
		})
		return
	}

	basePath := h.Config.LibraryBasePath
	if basePath == "" {
		slog.Error("admin-rename: LibraryBasePath not configured", "target_id", targetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "library base path not configured", r.URL.Path)
		return
	}

	ctx := r.Context()

	// Foreground-Snapshot: resolve schema + library-dir once so background
	// goroutine uses a stable view even if admin changes settings mid-job.
	renderer := shadowfs.NewSchemaRenderer()
	schemaSnap, err := shadowfs.ResolveUserSchema(ctx, h.DB.Pool, h.Settings, renderer, targetID, fileType)
	if err != nil {
		slog.Error("admin-rename: resolve schema", "error", err, "target_id", targetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "resolve", r.URL.Path)
		return
	}
	dirSnap, err := shadowfs.ResolveLibraryDirName(ctx, h.DB.Pool, targetID)
	if errors.Is(err, shadowfs.ErrUserNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "user not found", r.URL.Path)
		return
	}
	if err != nil {
		slog.Error("admin-rename: resolve library dir", "error", err, "target_id", targetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "resolve", r.URL.Path)
		return
	}

	// Insert background job. Unique partial index idx_bg_jobs_one_running
	// rejects a second concurrent running shadowfs_rename for this user.
	var jobID string
	err = h.DB.Pool.QueryRow(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status)
		 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')
		 RETURNING id`,
		targetID).Scan(&jobID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "rename_in_progress"})
			return
		}
		slog.Error("admin-rename: insert job", "error", err, "target_id", targetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "job_id": jobID})

	shutdown.Go("admin-rename:"+targetID+":"+fileType, func(bgCtx context.Context) {
		h.executeFileRenameBackground(bgCtx, targetID, fileType, basePath, jobID, schemaSnap, dirSnap)
	})
}

// PostAdminRenameBulk kicks off a rename for all active users for one fileType.
//
// Design (v2):
//   - Cluster-wide serialization via pg_try_advisory_lock on a constant key
//     — avoids requiring a new background_jobs row with NULL user_id
//     (column is NOT NULL) and avoids 23505 races on a dedicated unique index.
//   - Batch-JOIN against users ⟕ user_file_schemas to pre-resolve ALL user
//     snapshots in one round-trip (scales to 100k users).
//   - Per-user shadowfs_rename rows are inserted iteratively in the
//     background goroutine. 23505 from idx_bg_jobs_one_running = user has a
//     running per-user job already (user-triggered or from a previous bulk);
//     skip — do not block the rest of the bulk run.
//
// POST /api/v1/admin/rename-execute-bulk
// Body: {"file_type": "activity"}
func (h *Handler) PostAdminRenameBulk(w http.ResponseWriter, r *http.Request) {
	body, ok := httputil.DecodeJSON[struct {
		FileType string `json:"file_type"`
	}](w, r)
	if !ok {
		return
	}
	if !isAllowedFileType(body.FileType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown file type", r.URL.Path)
		return
	}
	if h.Config == nil || h.Config.StorageBackend != "local" {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "note": "not applicable"})
		return
	}
	if h.Config.LibraryBasePath == "" {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "library base path not configured", r.URL.Path)
		return
	}

	ctx := r.Context()

	// Acquire a pinned connection for the advisory lock. The lock is
	// session-scoped (pg_advisory_lock), so the connection MUST be held
	// across foreground + background goroutine and only released when
	// the bulk run finishes.
	conn, err := h.DB.Pool.Acquire(ctx)
	if err != nil {
		slog.Error("admin-rename-bulk: acquire conn", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "acquire conn", r.URL.Path)
		return
	}

	var locked bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`, bulkRenameAdvisoryLockKey).Scan(&locked); err != nil {
		conn.Release()
		slog.Error("admin-rename-bulk: try lock", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "lock", r.URL.Path)
		return
	}
	if !locked {
		conn.Release()
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "bulk_rename_in_progress"})
		return
	}

	// Batch-JOIN: one query, all users + their schema override (if any).
	canEditSchemas := h.Settings != nil && h.Settings.Get("library.user_can_edit_schemas").Value == "true"
	var globalDefault string
	if h.Settings != nil {
		globalDefault = h.Settings.Get("library.default_schema." + body.FileType).Value
	}

	rows, err := h.DB.Pool.Query(ctx, `
		SELECT u.id,
		       COALESCE(u.library_dir_name, u.id::text) AS effective_dir,
		       ufs.schema_template
		FROM users u
		LEFT JOIN user_file_schemas ufs
		  ON ufs.user_id = u.id AND ufs.file_type = $1
		WHERE u.is_active = true AND u.deleted_at IS NULL
	`, body.FileType)
	if err != nil {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey)
		conn.Release()
		slog.Error("admin-rename-bulk: batch query", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "batch query", r.URL.Path)
		return
	}

	type snap struct{ uid, dir, schema string }
	snaps := make([]snap, 0, 1024)
	renderer := shadowfs.NewSchemaRenderer()
	builtIn := renderer.DefaultSchema(body.FileType)

	for rows.Next() {
		var uid, effDir string
		var userSchema *string
		if err := rows.Scan(&uid, &effDir, &userSchema); err != nil {
			slog.Warn("admin-rename-bulk: scan row", "error", err)
			continue
		}

		schema := builtIn
		if globalDefault != "" {
			schema = globalDefault
		}
		if canEditSchemas && userSchema != nil && *userSchema != "" {
			schema = *userSchema
		}
		snaps = append(snaps, snap{uid, effDir, schema})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey)
		conn.Release()
		slog.Error("admin-rename-bulk: rows err", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "rows err", r.URL.Path)
		return
	}

	// candidate_count = users matched before the background loop; not all may
	// actually start (some skipped via 23505 when a per-user rename is already
	// running). started_count is only knowable after the background goroutine
	// completes — see logs for the full breakdown.
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"candidate_count": len(snaps),
	})

	basePath := h.Config.LibraryBasePath
	fileType := body.FileType

	shutdown.Go("admin-rename-bulk:"+fileType, func(bgCtx context.Context) {
		// Release advisory lock at end (independent of per-user outcomes).
		// Use a detached context so the unlock still fires even when bgCtx is
		// cancelled during graceful shutdown (MED #15).
		defer func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey)
			conn.Release()
		}()

		skipped := 0
		renamed := 0

		for _, s := range snaps {
			// Try to register per-user job. 23505 = user already has a
			// running shadowfs_rename job (unique partial index). Skip.
			var perUserJobID string
			err := h.DB.Pool.QueryRow(bgCtx,
				`INSERT INTO background_jobs (id, job_type, user_id, status)
				 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')
				 RETURNING id`, s.uid).Scan(&perUserJobID)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					// Unique partial index idx_bg_jobs_one_running: user already has
					// a running shadowfs_rename job — skip without error.
					skipped++
					continue
				}
				// NIT #39: pgx.ErrNoRows is impossible for INSERT...RETURNING id
				// (an INSERT either succeeds, conflicts, or errors — never ErrNoRows).
				// That dead branch has been removed.
				slog.Warn("admin-rename-bulk: insert per-user job", "error", err, "uid", s.uid)
				skipped++
				continue
			}

			h.executeFileRenameBackground(bgCtx, s.uid, fileType, basePath, perUserJobID, s.schema, s.dir)
			renamed++
		}

		slog.Info("admin-rename-bulk: done",
			"file_type", fileType,
			"total", len(snaps),
			"renamed", renamed,
			"skipped", skipped)
	})
}
