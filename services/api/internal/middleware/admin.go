package middleware

import (
	"net/http"
)

// RequireAdmin returns middleware that verifies the authenticated user has admin privileges
// via the is_admin JWT claim. Must be placed AFTER JWT auth middleware.
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isAdmin, _ := r.Context().Value(IsAdminKey).(bool)
			if !isAdmin {
				http.Error(w, `{"error":"admin required"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
