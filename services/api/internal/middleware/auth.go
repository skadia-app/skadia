package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// writeJSONError writes a JSON error response. Used by all middleware in this package.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// JWTAuth validates HS256 JWTs issued by this backend.
type JWTAuth struct {
	jwtSecret []byte
}

// NewJWTAuth creates JWT validation middleware.
func NewJWTAuth(jwtSecret []byte) *JWTAuth {
	return &JWTAuth{jwtSecret: jwtSecret}
}

// Middleware validates JWT from Authorization header.
func (j *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		token, err := jwt.Parse([]byte(tokenStr),
			jwt.WithKey(jwa.HS256, j.jwtSecret),
			jwt.WithValidate(true),
			jwt.WithAudience("skadia-api"),
			jwt.WithIssuer("skadia"),
		)
		if err != nil {
			slog.Debug("JWT validation failed", "error", err)
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		sub := token.Subject()
		if sub == "" {
			writeJSONError(w, http.StatusUnauthorized, "token missing sub claim")
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, sub)

		// Extract custom claims from JWT into context
		isAdminVal, _ := token.Get("is_admin")
		isAdmin, _ := isAdminVal.(bool)
		emailVerifiedVal, _ := token.Get("email_verified")
		emailVerified, _ := emailVerifiedVal.(bool)
		mustChangePWVal, _ := token.Get("must_change_password")
		mustChangePW, _ := mustChangePWVal.(bool)

		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		ctx = context.WithValue(ctx, EmailVerifiedKey, emailVerified)
		ctx = context.WithValue(ctx, MustChangePWKey, mustChangePW)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
