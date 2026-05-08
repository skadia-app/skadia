package applicablemodifiers

import (
	"strings"
	"testing"
)

func TestValidateKeysFailsOnUnknownKey(t *testing.T) {
	keys := []string{"is_electric", "is_electrik"}
	err := ValidateKeys(keys)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "is_electrik") {
		t.Fatalf("error should name unknown key: %v", err)
	}
}

func TestValidateKeysPassesForKnownKeys(t *testing.T) {
	keys := []string{"is_electric", "is_indoor", "is_virtual"}
	if err := ValidateKeys(keys); err != nil {
		t.Fatalf("unexpected error for known keys: %v", err)
	}
}

func TestValidateKeysPassesForEmptySlice(t *testing.T) {
	if err := ValidateKeys([]string{}); err != nil {
		t.Fatalf("unexpected error for empty keys: %v", err)
	}
}
