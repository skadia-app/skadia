package auth

import (
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	// Must be 64 hex chars (32 bytes).
	if len(token) != 64 {
		t.Errorf("GenerateToken() length = %d, want 64", len(token))
	}

	// Must be valid hex.
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("GenerateToken() not valid hex: %v", err)
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
	family := GenerateFamily()

	// Must be a valid UUID.
	if _, err := uuid.Parse(family); err != nil {
		t.Errorf("GenerateFamily() not a valid UUID: %v", err)
	}
}

func TestGenerateFamilyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		f := GenerateFamily()
		if seen[f] {
			t.Fatal("GenerateFamily() produced duplicate family ID")
		}
		seen[f] = true
	}
}
