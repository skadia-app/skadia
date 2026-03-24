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
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			var isAdmin bool
			err := db.Pool.QueryRow(r.Context(),
				"SELECT is_admin FROM users WHERE id = $1", userID,
			).Scan(&isAdmin)

			if errors.Is(err, pgx.ErrNoRows) {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if err != nil {
				slog.Error("admin check failed", "error", err)
				writeJSONError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if !isAdmin {
				writeJSONError(w, http.StatusForbidden, "admin required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
