package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTAuth validates JWTs.
// When jwtSecret is set, tokens are validated as HS256 (issued by this backend).
// Otherwise JWKS (RS256 from OIDC) is used as fallback.
type JWTAuth struct {
	jwksURL   string
	jwtSecret []byte
	keySet    jwk.Set
	mu        sync.RWMutex
	lastFetch time.Time
}

// NewJWTAuth creates a JWT middleware.
// If jwtSecret is non-empty it is used for HS256 validation; otherwise JWKS is used.
func NewJWTAuth(jwksURL string, jwtSecret []byte) *JWTAuth {
	return &JWTAuth{jwksURL: jwksURL, jwtSecret: jwtSecret}
}

// fetchKeys retrieves the JWKS key set, caching for 5 minutes.
func (j *JWTAuth) fetchKeys(ctx context.Context) (jwk.Set, error) {
	j.mu.RLock()
	if j.keySet != nil && time.Since(j.lastFetch) < 5*time.Minute {
		ks := j.keySet
		j.mu.RUnlock()
		return ks, nil
	}
	j.mu.RUnlock()

	j.mu.Lock()
	defer j.mu.Unlock()

	if j.keySet != nil && time.Since(j.lastFetch) < 5*time.Minute {
		return j.keySet, nil
	}

	ks, err := jwk.Fetch(ctx, j.jwksURL)
	if err != nil {
		if j.keySet != nil {
			slog.Warn("JWKS fetch failed, using stale keys", "error", err)
			return j.keySet, nil
		}
		return nil, err
	}

	j.keySet = ks
	j.lastFetch = time.Now()
	slog.Info("JWKS keys refreshed", "url", j.jwksURL)
	return ks, nil
}

// Middleware returns an http.Handler middleware that validates JWT tokens.
func (j *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		var token jwt.Token
		var err error

		if len(j.jwtSecret) > 0 {
			// HS256 – tokens issued by this backend after OIDC login
			token, err = jwt.Parse(
				[]byte(tokenStr),
				jwt.WithKey(jwa.HS256, j.jwtSecret),
				jwt.WithValidate(true),
			)
		} else {
			// RS256 – tokens issued directly by OIDC (JWKS)
			keySet, fetchErr := j.fetchKeys(r.Context())
			if fetchErr != nil {
				slog.Error("JWKS fetch failed", "error", fetchErr)
				http.Error(w, `{"error":"authentication service unavailable"}`, http.StatusUnauthorized)
				return
			}
			token, err = jwt.Parse(
				[]byte(tokenStr),
				jwt.WithKeySet(keySet, jws.WithInferAlgorithmFromKey(true)),
				jwt.WithValidate(true),
			)
		}

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
