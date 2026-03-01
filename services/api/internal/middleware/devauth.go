package middleware

import (
	"context"
	"net/http"
)

type contextKey string

const UserIDKey contextKey = "user_id"

// DevAuth is a development-only middleware that injects a fixed user ID.
// Replace with OIDC JWT validation for production.
func DevAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := "00000000-0000-0000-0000-000000000001"
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}
