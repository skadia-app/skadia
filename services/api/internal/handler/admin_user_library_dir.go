// services/api/internal/handler/admin_user_library_dir.go
package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// GetAdminUserLibraryDir returns the raw library_dir_name for a target user.
// GET /api/v1/admin/users/{id}/library-dir
func (h *Handler) GetAdminUserLibraryDir(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")

	var name *string
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT library_dir_name FROM users WHERE id = $1`, targetID).Scan(&name)
	if err != nil {
		slog.Error("admin get user library dir", "error", err, "target", targetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db error", r.URL.Path)
		return
	}

	effective := targetID
	if name != nil && *name != "" {
		effective = *name
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":          targetID,
		"library_dir_name": name,
		"effective_name":   effective,
	})
}

// UpdateAdminUserLibraryDir sets library_dir_name for a target user.
// PUT /api/v1/admin/users/{id}/library-dir
// Body: {"library_dir_name": null | "my-library"}
// Conventions: 23505 → 200 + {"status":"library_dir_name_taken"}
//
//	UUID clash → 200 + {"status":"library_dir_name_clashes_with_user_uuid"}
func (h *Handler) UpdateAdminUserLibraryDir(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")

	body, ok := httputil.DecodeJSON[struct {
		LibraryDirName *string `json:"library_dir_name"`
	}](w, r)
	if !ok {
		return
	}

	// Null → reset
	if body.LibraryDirName == nil {
		if _, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET library_dir_name = NULL WHERE id = $1`, targetID); err != nil {
			slog.Error("admin reset library dir", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db error", r.URL.Path)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	name := strings.TrimSpace(*body.LibraryDirName)

	// Same validation as user endpoint
	if strings.ContainsRune(name, 0) || len(name) > 100 ||
		strings.Contains(name, "/") || strings.Contains(name, "\\") ||
		strings.Contains(name, "..") || name == "." || strings.HasPrefix(name, "_") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid library_dir_name", r.URL.Path)
		return
	}
	if len(name) == 36 && name[8] == '-' && name[13] == '-' && name[18] == '-' && name[23] == '-' {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"library_dir_name must not be in UUID format", r.URL.Path)
		return
	}

	// UUID-clash check: does the value match another user's UUID?
	var exists bool
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE id::text = $1 AND id::text != $2)`,
		name, targetID).Scan(&exists)
	if exists {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "library_dir_name_clashes_with_user_uuid",
		})
		return
	}

	_, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET library_dir_name = $1 WHERE id = $2`, name, targetID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httputil.WriteJSON(w, http.StatusOK, map[string]string{
				"status": "library_dir_name_taken",
			})
			return
		}
		slog.Error("admin update library dir", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "db error", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
