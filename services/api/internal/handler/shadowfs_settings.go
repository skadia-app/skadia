package handler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// allowedFileTypes mirrors shadowfs.UserFacingFileTypes (canonical source).
// Declared as package-level var to avoid repeated shadowfs.UserFacingFileTypes
// references in isAllowedFileType and handler loops.
var allowedFileTypes = shadowfs.UserFacingFileTypes

// GetFileSchemas returns all per-type file schemas for the current user.
// GET /api/v1/settings/file-schemas
func (h *Handler) GetFileSchemas(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT file_type, schema_template FROM user_file_schemas WHERE user_id = $1 ORDER BY file_type`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query file schemas", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	defer rows.Close()

	stored := make(map[string]string)
	for rows.Next() {
		var ft, tmpl string
		if err := rows.Scan(&ft, &tmpl); err != nil {
			slog.Error("failed to scan file schema row", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
			return
		}
		stored[ft] = tmpl
	}
	if err := rows.Err(); err != nil {
		slog.Error("file schemas rows error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	renderer := shadowfs.NewSchemaRenderer()
	type schemaEntry struct {
		FileType        string `json:"file_type"`
		Schema          string `json:"schema"`
		EffectiveSchema string `json:"effective_schema"`
		IsUserOverride  bool   `json:"is_user_override"`
	}
	canEditSchemas := h.Settings != nil && h.Settings.Get("library.user_can_edit_schemas").Value == "true"
	result := make([]schemaEntry, 0, len(allowedFileTypes))
	for _, ft := range allowedFileTypes {
		userSchema := stored[ft]
		effective, err := shadowfs.ResolveUserSchema(r.Context(), h.DB.Pool, h.Settings, renderer, userID, ft)
		if err != nil {
			slog.Error("get file-schemas: resolve", "error", err, "file_type", ft)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "resolve error", r.URL.Path)
			return
		}
		result = append(result, schemaEntry{
			FileType:        ft,
			Schema:          userSchema,
			EffectiveSchema: effective,
			IsUserOverride:  userSchema != "" && canEditSchemas,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// GetFileSchemasContext returns editable/backend/admin_managed_reason metadata for
// the file-schemas settings UI. Separated from GET /file-schemas to keep that
// endpoint's top-level array shape backward-compatible.
// GET /api/v1/settings/file-schemas-context
//
// NOTE: Route needs registration: GET /settings/file-schemas-context
func (h *Handler) GetFileSchemasContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := httputil.RequireUserID(w, r); !ok {
		return
	}

	backend := h.storageBackend()

	canEdit := h.Settings != nil && h.Settings.Get("library.user_can_edit_schemas").Value == "true"
	editable := canEdit

	// Priority: s3 > admin_locked > null
	var reason *string
	if backend == "s3" {
		v := "s3_backend"
		reason = &v
	} else if !canEdit {
		v := "admin_locked"
		reason = &v
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"editable":             editable,
		"backend":              backend,
		"admin_managed_reason": reason,
	})
}

// UpdateFileSchema sets a custom schema template for a specific file type.
// PUT /api/v1/settings/file-schemas/{type}
func (h *Handler) UpdateFileSchema(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Gate: user_can_edit_schemas must be enabled by admin.
	if h.Settings == nil || h.Settings.Get("library.user_can_edit_schemas").Value != "true" {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden",
			"file schemas are managed by administrator", r.URL.Path)
		return
	}

	fileType := chi.URLParam(r, "type")
	if !isAllowedFileType(fileType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown file type", r.URL.Path)
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		Schema string `json:"schema"`
	}](w, r)
	if !ok {
		return
	}

	// PII check: reject schemas that embed personal data into file paths.
	if piiKey := shadowfs.ContainsPIIVariable(body.Schema); piiKey != "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			fmt.Sprintf("schema must not contain PII variable {%s}", piiKey), r.URL.Path)
		return
	}

	renderer := shadowfs.NewSchemaRenderer()
	warnings, err := renderer.ValidateSchema(fileType, body.Schema)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	_, dbErr := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO user_file_schemas (user_id, file_type, schema_template)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, file_type) DO UPDATE SET schema_template = EXCLUDED.schema_template`,
		userID, fileType, body.Schema,
	)
	if dbErr != nil {
		slog.Error("failed to upsert file schema", "error", dbErr, "user_id", userID, "file_type", fileType)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"file_type": fileType,
		"schema":    body.Schema,
		"warnings":  warnings,
	})
}

// GetLibraryDir returns the user's library directory name plus admin-model metadata.
// GET /api/v1/settings/library-dir
func (h *Handler) GetLibraryDir(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var libraryDirName *string
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT library_dir_name FROM users WHERE id = $1`,
		userID,
	).Scan(&libraryDirName)
	if err != nil {
		slog.Error("failed to query library_dir_name", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	effectiveName := userID
	if libraryDirName != nil && *libraryDirName != "" {
		effectiveName = *libraryDirName
	}

	backend := h.storageBackend()

	canEdit := h.Settings != nil && h.Settings.Get("library.user_can_edit_dir_name").Value == "true"
	editable := canEdit && backend == "local"

	// Priority: s3 > admin_locked > null
	var reason *string
	if backend == "s3" {
		r := "s3_backend"
		reason = &r
	} else if !canEdit {
		r := "admin_locked"
		reason = &r
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"library_dir_name":     libraryDirName, // *string, null if unset
		"effective_name":       effectiveName,
		"editable":             editable,
		"backend":              backend,
		"admin_managed_reason": reason,
	})
}

// UpdateLibraryDir sets the user's library directory name.
// PUT /api/v1/settings/library-dir
func (h *Handler) UpdateLibraryDir(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Gate: S3 backend does not support per-user directory names.
	if h.storageBackend() == "s3" {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden",
			"library_dir_name not available for S3 backend", r.URL.Path)
		return
	}

	// Gate: admin must allow user edits.
	if h.Settings == nil || h.Settings.Get("library.user_can_edit_dir_name").Value != "true" {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden",
			"library_dir_name is managed by administrator", r.URL.Path)
		return
	}

	// Pointer body: null → explicit reset (MED #18 — match admin-endpoint semantics).
	body, ok := httputil.DecodeJSON[struct {
		LibraryDirName *string `json:"library_dir_name"`
	}](w, r)
	if !ok {
		return
	}

	// Null → reset to NULL (UUID default will be used on read)
	if body.LibraryDirName == nil {
		_, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET library_dir_name = NULL WHERE id = $1`,
			userID,
		)
		if err != nil {
			slog.Error("failed to reset library_dir_name", "error", err, "user_id", userID)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	name := strings.TrimSpace(*body.LibraryDirName)

	// Empty string after trim → treat as reset.
	if name == "" {
		_, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET library_dir_name = NULL WHERE id = $1`,
			userID,
		)
		if err != nil {
			slog.Error("failed to reset library_dir_name", "error", err, "user_id", userID)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Shared validation: path-safety + clash-before-uuid-format-reject (HIGH #7+#8).
	if validateLibraryDirName(r.Context(), w, r, h.DB.Pool, name, userID) {
		return
	}

	_, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET library_dir_name = $1 WHERE id = $2`,
		name, userID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// HTTP-Fehlerkonvention: 200 OK + status body for app-logic conflicts
			httputil.WriteJSON(w, http.StatusOK, map[string]string{
				"status": "library_dir_name_taken",
			})
			return
		}
		slog.Error("failed to update library_dir_name", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	// Return status-only (consistent with reset paths + admin endpoint).
	// Frontend calls invalidateQueries(['library-dir']) on success to re-fetch
	// the full LibraryDirResponse shape from GET — no body consumption needed (R2 MED #7).
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// PreviewFileRename computes which active files would be moved when the user's
// schema template for a given type is changed.
// POST /api/v1/settings/file-schemas/{type}/rename-preview
func (h *Handler) PreviewFileRename(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	fileType := chi.URLParam(r, "type")
	if !isAllowedFileType(fileType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown file type", r.URL.Path)
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		Schema string `json:"schema"`
	}](w, r)
	if !ok {
		return
	}

	renderer := shadowfs.NewSchemaRenderer()
	if _, err := renderer.ValidateSchema(fileType, body.Schema); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	// Resolve user directory name once.
	userDir, err := shadowfs.ResolveLibraryDirName(r.Context(), h.DB.Pool, userID)
	if errors.Is(err, shadowfs.ErrUserNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "user not found", r.URL.Path)
		return
	}
	if err != nil {
		slog.Error("rename-preview: resolve library dir", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	// Query all active files of this type.
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, storage_path, file_name FROM user_files
		 WHERE user_id = $1 AND file_type = $2 AND status = 'active'`,
		userID, fileType,
	)
	if err != nil {
		slog.Error("rename-preview: query user_files", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	defer rows.Close()

	type renamePair struct {
		FileID  string `json:"file_id"`
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	var pairs []renamePair

	typeDirPrefix := filepath.Join(userDir, shadowfs.DirForType(fileType))

	for rows.Next() {
		var id, storagePath, fileName string
		if err := rows.Scan(&id, &storagePath, &fileName); err != nil {
			slog.Error("rename-preview: scan row", "error", err)
			continue
		}

		if fileType == "activity" {
			rendered, renderErr := shadowfs.RenderActivityFilePath(r.Context(), shadowfs.ActivityFileRefreshRequest{
				Pool:           h.DB.Pool,
				Settings:       h.Settings,
				Renderer:       renderer,
				UserID:         userID,
				FileID:         id,
				SchemaOverride: body.Schema,
			})
			if renderErr != nil {
				slog.Warn("rename-preview: render activity path", "file_id", id, "error", renderErr)
				continue
			}
			if rendered.FileID != "" && rendered.NewPath != rendered.OldPath {
				pairs = append(pairs, renamePair{
					FileID:  id,
					OldPath: rendered.OldPath,
					NewPath: rendered.NewPath,
				})
			}
			if rendered.FileID != "" {
				continue
			}
		}

		ext := filepath.Ext(storagePath)

		// Strip the user/type prefix to recover just the rendered portion.
		rendered := storagePath
		if rel, found := strings.CutPrefix(storagePath, typeDirPrefix+string(filepath.Separator)); found {
			rendered = rel
		}
		stem := strings.TrimSuffix(rendered, ext)

		tags := extractTagsFromPath(stem, fileName, fileType)

		newRendered := renderer.RenderPath(fileType, body.Schema, tags, ext)
		newPath := filepath.Join(typeDirPrefix, newRendered)

		if newPath != storagePath {
			pairs = append(pairs, renamePair{
				FileID:  id,
				OldPath: storagePath,
				NewPath: newPath,
			})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("rename-preview: rows error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	if pairs == nil {
		pairs = []renamePair{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"renames": pairs,
	})
}

// ExecuteFileRename renames all active files of the given type to paths
// computed from the current (already-saved) schema template.
// Uses a pg_advisory_lock on a pinned connection and registers a background_job
// entry for ActiveJobsIndicator visibility and crash-safety.
// POST /api/v1/settings/file-schemas/{type}/rename-execute
func (h *Handler) ExecuteFileRename(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// MED #16: Gate on admin lockdown. When user_can_edit_schemas is false,
	// the admin owns effective schema via admin-rename endpoints. Allowing
	// user-triggered renames during lockdown would bypass admin control.
	if h.Settings == nil || h.Settings.Get("library.user_can_edit_schemas").Value != "true" {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden",
			"rename-execute is managed by administrator during lockdown", r.URL.Path)
		return
	}

	fileType := chi.URLParam(r, "type")
	if !isAllowedFileType(fileType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown file type", r.URL.Path)
		return
	}

	basePath := h.Config.LibraryBasePath
	ctx := r.Context()
	if basePath == "" {
		var localCount int
		if err := h.DB.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM user_files
			 WHERE user_id = $1 AND file_type = $2 AND storage_backend = 'local' AND status = 'active'`,
			userID, fileType,
		).Scan(&localCount); err != nil {
			slog.Error("rename-execute: count local files", "error", err, "user_id", userID)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
			return
		}
		if localCount == 0 {
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status":  "ok",
				"renamed": 0,
				"note":    "rename not applicable for non-local backends",
			})
			return
		}
		slog.Error("rename-execute: LibraryBasePath not configured for local files", "user_id", userID, "file_type", fileType, "local_files", localCount)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "library base path not configured", r.URL.Path)
		return
	}

	// Guard: check for an already-running rename job for this user+type.
	var existingCount int
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs
		 WHERE job_type = 'shadowfs_rename' AND user_id = $1 AND status = 'running'`,
		userID,
	).Scan(&existingCount); err != nil {
		slog.Error("rename-execute: check existing job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	if existingCount > 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "rename_in_progress"})
		return
	}

	// Register background job for ActiveJobsIndicator visibility.
	var jobID string
	if err := h.DB.Pool.QueryRow(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status)
		 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')
		 RETURNING id`,
		userID,
	).Scan(&jobID); err != nil {
		slog.Error("rename-execute: insert background_job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	// Foreground: resolve schema and library dir once, pass as snapshot to background.
	// This ensures admin mid-job changes do not affect an already-started rename job.
	renderer := shadowfs.NewSchemaRenderer()
	schemaSnapshot, err := shadowfs.ResolveUserSchema(ctx, h.DB.Pool, h.Settings, renderer, userID, fileType)
	if err != nil {
		slog.Error("rename-execute: resolve schema", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	libraryDirSnapshot, err := shadowfs.ResolveLibraryDirName(ctx, h.DB.Pool, userID)
	if errors.Is(err, shadowfs.ErrUserNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "user not found", r.URL.Path)
		return
	}
	if err != nil {
		slog.Error("rename-execute: resolve library dir", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}

	// Return immediately — actual rename work runs in background.
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"job_id": jobID,
	})

	// Run the rename work in background via shutdown.Go for graceful-shutdown awareness.
	shutdown.Go("rename-execute:"+userID+":"+fileType, func(bgCtx context.Context) {
		h.executeFileRenameBackground(bgCtx, userID, fileType, basePath, jobID, schemaSnapshot, libraryDirSnapshot)
	})
}

// executeFileRenameBackground performs the actual file rename work in a background goroutine.
// schemaSnapshot and libraryDirSnapshot are resolved in the foreground before the background
// goroutine starts, ensuring admin mid-job changes do not affect an already-started rename job.
func (h *Handler) executeFileRenameBackground(ctx context.Context, userID, fileType, basePath, jobID,
	schemaSnapshot, libraryDirSnapshot string) {
	// Use detached context so cleanup updates survive graceful shutdown.
	// If ctx is cancelled (shutdown), the background_jobs status update and
	// is_renaming reset must still run to avoid stranded rows. Mirrors
	// admin_rename.go MED #15 fix (R2 HIGH #1).
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := h.DB.Pool.Exec(cleanupCtx,
			`UPDATE background_jobs SET status = 'completed', finished_at = NOW() WHERE id = $1`,
			jobID,
		); err != nil {
			slog.Warn("rename-execute: complete job failed", "job_id", jobID, "error", err)
		}
	}()

	// Pinned connection for advisory lock — must not be returned to pool mid-lock.
	conn, err := h.DB.Pool.Acquire(ctx)
	if err != nil {
		slog.Error("rename-execute: acquire conn", "error", err, "user_id", userID)
		return
	}
	defer conn.Release()

	h64 := fnv.New64a()
	h64.Write([]byte(userID + ":" + fileType)) //nolint:errcheck
	lockKey := int64(h64.Sum64())
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		slog.Error("rename-execute: advisory lock", "error", err, "user_id", userID)
		return
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck

	// Use pre-resolved snapshots (captured at job-start in foreground).
	// Admin changes to schema or library_dir_name after job start do not affect this run.
	renderer := shadowfs.NewSchemaRenderer()
	schema := schemaSnapshot
	userDir := libraryDirSnapshot
	typeDirPrefix := filepath.Join(userDir, shadowfs.DirForType(fileType))

	// Query all active files not currently being renamed.
	fileRows, queryErr := conn.Query(ctx,
		`SELECT id, storage_path, file_name, has_sidecar
		 FROM user_files
		 WHERE user_id = $1 AND file_type = $2 AND storage_backend = 'local' AND status = 'active' AND NOT is_renaming`,
		userID, fileType,
	)
	if queryErr != nil {
		slog.Error("rename-execute: query files", "error", queryErr)
		return
	}

	type fileRow struct {
		id          string
		storagePath string
		fileName    string
		hasSidecar  bool
	}
	var files []fileRow
	for fileRows.Next() {
		var f fileRow
		if err := fileRows.Scan(&f.id, &f.storagePath, &f.fileName, &f.hasSidecar); err != nil {
			slog.Warn("rename-execute: scan row", "error", err)
			continue
		}
		files = append(files, f)
	}
	fileRows.Close()
	if err := fileRows.Err(); err != nil {
		slog.Error("rename-execute: rows error", "error", err)
		return
	}

	// Mark all candidate files as renaming in one batch.
	if len(files) > 0 {
		ids := make([]string, len(files))
		for i, f := range files {
			ids[i] = f.id
		}
		if _, err := conn.Exec(ctx,
			`UPDATE user_files SET is_renaming = true WHERE id = ANY($1::uuid[])`,
			ids,
		); err != nil {
			slog.Error("rename-execute: mark is_renaming", "error", err)
			return
		}
	}

	// Safety net: clear any leftover is_renaming flags on exit (crash recovery).
	// Uses detached context so the reset fires even during graceful shutdown (R2 HIGH #1).
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, clearErr := h.DB.Pool.Exec(cleanupCtx,
			`UPDATE user_files SET is_renaming = false
			 WHERE user_id = $1 AND file_type = $2 AND is_renaming = true`,
			userID, fileType,
		); clearErr != nil {
			slog.Warn("rename-execute: clear is_renaming safety net", "error", clearErr)
		}
	}()

	renamed := 0
	for _, f := range files {
		if fileType == "activity" {
			rendered, err := shadowfs.RenderActivityFilePath(ctx, shadowfs.ActivityFileRefreshRequest{
				Pool:               h.DB.Pool,
				Settings:           h.Settings,
				Renderer:           renderer,
				BasePath:           basePath,
				UserID:             userID,
				FileID:             f.id,
				SchemaOverride:     schema,
				LibraryDirOverride: userDir,
			})
			if err != nil {
				slog.Error("rename-execute: render activity path", "error", err, "id", f.id)
				continue
			}
			if rendered.FileID == "" {
				// Orphaned activity files keep the legacy path-token fallback below.
			} else {
				refreshed, err := shadowfs.RefreshActivityFilePath(ctx, shadowfs.ActivityFileRefreshRequest{
					Pool:               h.DB.Pool,
					Settings:           h.Settings,
					Renderer:           renderer,
					BasePath:           basePath,
					UserID:             userID,
					FileID:             f.id,
					SchemaOverride:     schema,
					LibraryDirOverride: userDir,
				})
				if err != nil {
					slog.Error("rename-execute: refresh activity path", "error", err, "id", f.id)
					continue
				}
				if refreshed {
					renamed++
				}
				continue
			}
		}

		ext := filepath.Ext(f.storagePath)

		// Recover rendered portion by stripping user/type prefix.
		rendered := f.storagePath
		if rel, found := strings.CutPrefix(f.storagePath, typeDirPrefix+string(filepath.Separator)); found {
			rendered = rel
		}
		stem := strings.TrimSuffix(rendered, ext)
		tags := extractTagsFromPath(stem, f.fileName, fileType)

		newRendered := renderer.RenderPath(fileType, schema, tags, ext)
		newRelPath := filepath.Join(typeDirPrefix, newRendered)

		if newRelPath == f.storagePath {
			// No change needed for this file.
			if _, err := conn.Exec(ctx,
				`UPDATE user_files SET is_renaming = false WHERE id = $1`, f.id,
			); err != nil {
				slog.Warn("rename-execute: clear is_renaming (unchanged)", "id", f.id, "error", err)
			}
			continue
		}

		// Resolve collision so we never silently overwrite another file.
		newRelPath = shadowfs.ResolveCollision(basePath, newRelPath)

		// Security: validate new relative path for traversal and symlink escapes.
		if err := shadowfs.ValidateRelPath(basePath, newRelPath); err != nil {
			slog.Error("rename-execute: new path failed validation", "error", err, "rel", newRelPath)
			continue
		}

		oldFullPath := filepath.Join(basePath, f.storagePath)
		newFullPath := filepath.Join(basePath, newRelPath)

		// Security: new path must stay inside basePath.
		if !strings.HasPrefix(filepath.Clean(newFullPath), filepath.Clean(basePath)+string(filepath.Separator)) {
			slog.Error("rename-execute: new path escapes basePath", "new", newFullPath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(newFullPath), 0o755); err != nil {
			slog.Error("rename-execute: mkdir", "error", err, "path", newFullPath)
			continue
		}

		// Atomic strategy: copy → update DB → delete old.
		if err := copyFileRename(oldFullPath, newFullPath); err != nil {
			slog.Error("rename-execute: copy file", "error", err, "from", oldFullPath, "to", newFullPath)
			os.Remove(newFullPath) //nolint:errcheck
			continue
		}

		if _, err := conn.Exec(ctx,
			`UPDATE user_files SET storage_path = $1, is_renaming = false, updated_at = NOW()
			 WHERE id = $2`,
			newRelPath, f.id,
		); err != nil {
			slog.Error("rename-execute: update storage_path", "error", err, "id", f.id)
			os.Remove(newFullPath) //nolint:errcheck
			continue
		}

		// Remove old file only after DB is updated.
		if err := os.Remove(oldFullPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("rename-execute: remove old file", "error", err, "path", oldFullPath)
		}

		// Move sidecars best-effort (no rollback on failure).
		if f.hasSidecar {
			moveRenameSidecar(oldFullPath, newFullPath, fileType)
		}

		renamed++
	}

	slog.Info("rename-execute: done", "user_id", userID, "file_type", fileType, "renamed", renamed)
}

// copyFileRename copies src to dst using O_EXCL — never overwrites an existing file.
func copyFileRename(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer func() {
		if cerr := out.Close(); retErr == nil {
			retErr = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

// moveRenameSidecar moves a sidecar file alongside the renamed main file (best-effort).
func moveRenameSidecar(oldMainPath, newMainPath, fileType string) {
	var oldSidecar, newSidecar string
	switch fileType {
	case "media", "activity":
		oldSidecar = oldMainPath + ".meta"
		newSidecar = newMainPath + ".meta"
	default:
		return
	}
	if _, err := os.Stat(oldSidecar); os.IsNotExist(err) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(newSidecar), 0o755); err != nil {
		slog.Warn("moveRenameSidecar: mkdir", "error", err)
		return
	}
	if err := os.Rename(oldSidecar, newSidecar); err != nil {
		slog.Warn("moveRenameSidecar: rename failed", "error", err, "from", oldSidecar, "to", newSidecar)
	}
}

// extractTagsFromPath tries to recover schema variable values from an existing
// rendered path stem and the original file name. This is best-effort: variables
// that cannot be recovered are left absent so RenderPath fills them with "unknown".
func extractTagsFromPath(stem, fileName, fileType string) map[string]string {
	tags := make(map[string]string)

	// Use the original file name stem as "title" fallback.
	if fileName != "" {
		nameStem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
		if nameStem != "" {
			tags["title"] = nameStem
		}
	}

	// Walk path segments looking for date-like components.
	parts := strings.Split(filepath.ToSlash(stem), "/")
	for _, part := range parts {
		switch {
		case len(part) == 4 && isDigits(part):
			// Pure 4-digit segment → year.
			tags["year"] = part
		case len(part) == 2 && isDigits(part):
			// Pure 2-digit segment → month.
			tags["month"] = part
		case len(part) >= 10 && isDigits(part[:4]) && part[4] == '-':
			// Segment starts with YYYY-MM-DD.
			tags["year"] = part[:4]
			if len(part) >= 7 && isDigits(part[5:7]) {
				tags["month"] = part[5:7]
				if len(part) >= 10 && part[7] == '-' && isDigits(part[8:10]) {
					tags["day"] = part[8:10]
				}
			}
		}
	}

	// File-type specific: extract sport/time from the last path segment.
	if fileType == "activity" && len(parts) > 0 {
		last := parts[len(parts)-1]
		// Typical segment: "2024-01-15_14-30_cycling"
		tokens := strings.Split(last, "_")
		if len(tokens) >= 3 {
			tags["sport"] = tokens[len(tokens)-1]
			if t := tokens[len(tokens)-2]; len(t) == 5 && t[2] == '-' {
				tags["time"] = t
			}
		}
	}

	return tags
}

// isDigits returns true if every byte in s is an ASCII digit.
func isDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isAllowedFileType(ft string) bool {
	return slices.Contains(allowedFileTypes, ft)
}

// storageBackend returns the configured storage backend name ("local" or "s3").
// When h.Config is nil (test-only wiring), it falls back to "local".
//
// LOW #32: centralizing this nil-guard removes ~3 duplicate inline checks and
// documents the nil-Config contract explicitly. Production always has Config
// wired; nil is a test-only artefact.
func (h *Handler) storageBackend() string {
	if h.Config == nil {
		return "local"
	}
	if h.Config.StorageBackend == "s3" {
		return "s3"
	}
	return "local"
}
