package auth

import (
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	JWTAudience = "skadia-api"
	JWTIssuer   = "skadia"
	JWTExpiry   = 15 * time.Minute
)

// IssueJWT creates a signed HS256 JWT for the given user.
func IssueJWT(userID string, secret []byte) (string, error) {
	now := time.Now()
	token, err := jwt.NewBuilder().
		Subject(userID).
		Issuer(JWTIssuer).
		Audience([]string{JWTAudience}).
		IssuedAt(now).
		NotBefore(now).
		Expiration(now.Add(JWTExpiry)).
		Build()
	if err != nil {
		return "", fmt.Errorf("build JWT: %w", err)
	}

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, secret))
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return string(signed), nil
}
