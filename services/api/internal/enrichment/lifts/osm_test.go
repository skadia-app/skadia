package lifts_test

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func TestBearingDeg(t *testing.T) {
	// Due east: (0,0) → (0,1) should be ~90°
	got := lifts.BearingDeg(0, 0, 0, 1)
	if math.Abs(got-90.0) > 0.5 {
		t.Errorf("BearingDeg(0,0,0,1): got %.4f, want ~90.0", got)
	}

	// Reverse heading: 180° diff from 0° → true
	if !lifts.IsReverseHeading(180, 0) {
		t.Error("IsReverseHeading(180, 0): expected true")
	}

	// Near same heading: 80° diff from 0° → false
	if lifts.IsReverseHeading(80, 0) {
		t.Error("IsReverseHeading(80, 0): expected false")
	}
}
