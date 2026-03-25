package middleware

import (
	"context"
	"net/http"
)

type contextKey string

const UserIDKey contextKey = "user_id"
const MustChangePWKey contextKey = "must_change_password"
const IsAdminKey contextKey = "is_admin"
const EmailVerifiedKey contextKey = "email_verified"

// DevAuthMiddleware is a development-only middleware that injects a fixed user ID.
// Only active when DEV_AUTH=true. Use JWTAuth for production.
func DevAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), UserIDKey, "00000000-0000-0000-0000-000000000001")
		ctx = context.WithValue(ctx, IsAdminKey, true)
		ctx = context.WithValue(ctx, EmailVerifiedKey, true)
		ctx = context.WithValue(ctx, MustChangePWKey, false)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

func GetIsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(IsAdminKey).(bool)
	return v
}

func GetEmailVerified(ctx context.Context) bool {
	v, _ := ctx.Value(EmailVerifiedKey).(bool)
	return v
}

func GetMustChangePW(ctx context.Context) bool {
	v, _ := ctx.Value(MustChangePWKey).(bool)
	return v
}
