package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

// RequireAdmin returns middleware that verifies the authenticated user has admin privileges.
// Must be placed AFTER JWT auth middleware.
func RequireAdmin(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			var isAdmin bool
			err := db.Pool.QueryRow(r.Context(),
				"SELECT is_admin FROM users WHERE id = $1", userID,
			).Scan(&isAdmin)

			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if err != nil {
				slog.Error("admin check failed", "error", err)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if !isAdmin {
				http.Error(w, `{"error":"admin required"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
