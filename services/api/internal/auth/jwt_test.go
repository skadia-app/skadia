package auth

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

func TestIssueJWT_Roundtrip(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	tokenStr, err := IssueJWT("user-123", false, true, false, secret)
	if err != nil {
		t.Fatalf("IssueJWT failed: %v", err)
	}

	// Parse back.
	token, err := jwt.Parse([]byte(tokenStr),
		jwt.WithKey(jwa.HS256, secret),
		jwt.WithValidate(true))
	if err != nil {
		t.Fatalf("jwt.Parse failed: %v", err)
	}

	if token.Subject() != "user-123" {
		t.Errorf("expected subject user-123, got %q", token.Subject())
	}

	foundAud := false
	for _, a := range token.Audience() {
		if a == JWTAudience {
			foundAud = true
			break
		}
	}
	if !foundAud {
		t.Errorf("expected audience to contain %q, got %v", JWTAudience, token.Audience())
	}

	if token.Issuer() != JWTIssuer {
		t.Errorf("expected issuer %q, got %q", JWTIssuer, token.Issuer())
	}

	expectedExp := time.Now().Add(JWTExpiry)
	if diff := expectedExp.Sub(token.Expiration()); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("expiration off by %v (expected ~%v, got %v)", diff, expectedExp, token.Expiration())
	}
}

func TestIssueJWT_DifferentUsers(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)
	t1, _ := IssueJWT("user-1", false, true, false, secret)
	t2, _ := IssueJWT("user-2", false, true, false, secret)
	if t1 == t2 {
		t.Error("different users should produce different JWTs")
	}
}
