package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

var allowedFileTypes = []string{"activity", "media", "document", "health", "route"}

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

	renderer := filestore.NewSchemaRenderer()
	type schemaEntry struct {
		FileType string `json:"file_type"`
		Schema   string `json:"schema"`
	}
	result := make([]schemaEntry, 0, len(allowedFileTypes))
	for _, ft := range allowedFileTypes {
		schema, ok := stored[ft]
		if !ok {
			schema = renderer.DefaultSchema(ft)
		}
		result = append(result, schemaEntry{FileType: ft, Schema: schema})
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// UpdateFileSchema sets a custom schema template for a specific file type.
// PUT /api/v1/settings/file-schemas/{type}
func (h *Handler) UpdateFileSchema(w http.ResponseWriter, r *http.Request) {
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

	renderer := filestore.NewSchemaRenderer()
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

// GetLibraryDir returns the user's library directory name.
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

	name := userID
	if libraryDirName != nil {
		name = *libraryDirName
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"library_dir_name": name,
	})
}

// UpdateLibraryDir sets the user's library directory name.
// PUT /api/v1/settings/library-dir
func (h *Handler) UpdateLibraryDir(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		LibraryDirName string `json:"library_dir_name"`
	}](w, r)
	if !ok {
		return
	}

	name := strings.TrimSpace(body.LibraryDirName)

	// Empty → reset to NULL (UUID default will be used on read)
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
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"library_dir_name": userID,
		})
		return
	}

	// Security checks
	if strings.Contains(name, "/") || strings.Contains(name, "\\") ||
		strings.Contains(name, "..") || name == "." ||
		strings.HasPrefix(name, "_") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid library_dir_name", r.URL.Path)
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

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"library_dir_name": name,
	})
}

func isAllowedFileType(ft string) bool {
	for _, allowed := range allowedFileTypes {
		if ft == allowed {
			return true
		}
	}
	return false
}
