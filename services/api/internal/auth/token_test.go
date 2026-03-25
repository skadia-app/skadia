package auth

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	// Must be 43 base64url chars (32 bytes, no padding).
	if len(token) != 43 {
		t.Errorf("GenerateToken() length = %d, want 43", len(token))
	}

	// Must be valid base64url.
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Errorf("GenerateToken() not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("GenerateToken() decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerateTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error: %v", err)
		}
		if seen[token] {
			t.Fatal("GenerateToken() produced duplicate token")
		}
		seen[token] = true
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	input := "test-token-abc123"
	h1 := HashToken(input)
	h2 := HashToken(input)
	if h1 != h2 {
		t.Errorf("HashToken() not deterministic: %q != %q", h1, h2)
	}

	// Must be 64 hex chars (SHA-256 = 32 bytes).
	if len(h1) != 64 {
		t.Errorf("HashToken() length = %d, want 64", len(h1))
	}
}

func TestHashTokenDifferentInputs(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if h1 == h2 {
		t.Error("HashToken() produced same hash for different inputs")
	}
}

func TestGenerateFamily(t *testing.T) {
	family, err := GenerateFamily()
	if err != nil {
		t.Fatalf("GenerateFamily() returned error: %v", err)
	}

	// Must be a valid UUID.
	if _, err := uuid.Parse(family); err != nil {
		t.Errorf("GenerateFamily() not a valid UUID: %v", err)
	}
}

func TestGenerateFamilyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		f, err := GenerateFamily()
		if err != nil {
			t.Fatalf("GenerateFamily() returned error: %v", err)
		}
		if seen[f] {
			t.Fatal("GenerateFamily() produced duplicate family ID")
		}
		seen[f] = true
	}
}
