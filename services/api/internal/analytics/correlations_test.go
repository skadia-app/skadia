package analytics

import (
	"strings"
	"testing"
)

func TestInterpretRReturnsStableKey(t *testing.T) {
	key := interpretR(0.42, false)

	if key != "analytics.correlation.moderate_positive" {
		t.Fatalf("interpretR() = %q, want stable key", key)
	}
	if strings.Contains(key, "Korrelation") || strings.Contains(key, "starke") {
		t.Fatalf("interpretR() returned localized text: %q", key)
	}
}

func TestInterpretRHandlesInverseDirection(t *testing.T) {
	key := interpretR(0.42, true)

	if key != "analytics.correlation.moderate_negative" {
		t.Fatalf("interpretR() = %q, want inverse direction in key", key)
	}
}

func TestLegacyInterpretRKeepsDisplayTextForLegacyField(t *testing.T) {
	got := legacyInterpretR(0.42, false)

	if got != "moderate positive Korrelation" {
		t.Fatalf("legacyInterpretR() = %q, want legacy display text", got)
	}
}
