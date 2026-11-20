package auth

import (
	"crypto/rand"
	"slices"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
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
		jwt.WithKey(jwa.HS256(), secret),
		jwt.WithValidate(true))
	if err != nil {
		t.Fatalf("jwt.Parse failed: %v", err)
	}

	if sub, ok := token.Subject(); !ok || sub != "user-123" {
		t.Errorf("expected subject user-123, got %q (ok=%v)", sub, ok)
	}

	aud, ok := token.Audience()
	if !ok || !slices.Contains(aud, JWTAudience) {
		t.Errorf("expected audience to contain %q, got %v (ok=%v)", JWTAudience, aud, ok)
	}

	if iss, ok := token.Issuer(); !ok || iss != JWTIssuer {
		t.Errorf("expected issuer %q, got %q (ok=%v)", JWTIssuer, iss, ok)
	}

	expectedExp := time.Now().Add(JWTExpiry)
	exp, ok := token.Expiration()
	if !ok {
		t.Fatal("expected expiration claim to be present")
	}
	if diff := expectedExp.Sub(exp); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("expiration off by %v (expected ~%v, got %v)", diff, expectedExp, exp)
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
