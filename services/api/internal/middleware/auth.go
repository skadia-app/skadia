package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

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
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
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
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		sub := token.Subject()
		if sub == "" {
			http.Error(w, `{"error":"token missing sub claim"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
