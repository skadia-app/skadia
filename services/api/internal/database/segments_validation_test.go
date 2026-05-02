package database

import (
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestIsValidSacList(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"T1", true},
		{"T1,T2,T3", true},
		{"T6", true},
		{"T7", false},    // ungültig
		{"T0", false},    // ungültig
		{"T1,T9", false}, // eines ungültig
		{"'; DROP TABLE", false},
	}
	for _, tt := range tests {
		if got := IsValidSacList(tt.in); got != tt.want {
			t.Errorf("IsValidSacList(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsValidMtbList(t *testing.T) {
	if !IsValidMtbList("S0,S1,S2,S3,S4,S5") {
		t.Error("S0-S5 should be valid")
	}
	if IsValidMtbList("S6") {
		t.Error("S6 should be invalid")
	}
}

func TestIsValidSurfaceList(t *testing.T) {
	if !IsValidSurfaceList("Asphalt,Schotter (grob)") {
		t.Error("standard surfaces should be valid")
	}
	if IsValidSurfaceList("Asphalt,Wechselnd") {
		t.Error("Wechselnd should not be filterable")
	}
	if IsValidSurfaceList("'; DROP") {
		t.Error("injection attempt")
	}
}

func TestSegmentSportGroupFromPredicatesUsesProjection(t *testing.T) {
	if got := segmentSportGroupFromPredicates([]sport.Predicate{{Sport: "snowshoeing"}}); got != "foot" {
		t.Fatalf("snowshoeing segment group = %q, want foot", got)
	}
	if got := segmentSportGroupFromPredicates([]sport.Predicate{{Sport: "kayaking"}}); got != "paddle" {
		t.Fatalf("kayaking segment group = %q, want paddle", got)
	}
	if got := segmentSportGroupFromPredicates([]sport.Predicate{{Sport: "snowshoeing"}, {Sport: "alpine_skiing"}}); got != "generic" {
		t.Fatalf("mixed segment group = %q, want generic", got)
	}
}

func TestMatcherSportGroupFromTupleUsesSegmentProjection(t *testing.T) {
	if got := sportGroupFromTuple("snowshoeing", ""); got != "foot" {
		t.Fatalf("sportGroupFromTuple(snowshoeing) = %q, want foot", got)
	}
	if got := sportGroupFromTuple("running", "trail"); got != "foot" {
		t.Fatalf("sportGroupFromTuple(running, trail) = %q, want foot parent fallback", got)
	}
	if got := sportGroupFromTuple("totally_unknown", ""); got != "generic" {
		t.Fatalf("sportGroupFromTuple(unknown) = %q, want generic", got)
	}
}

func TestSegmentPredicatesForTypeUsesProjection(t *testing.T) {
	if err := validateSegmentPredicatesForType("wassersport", []sport.Predicate{{Sport: "diving"}}); err == nil {
		t.Fatal("validateSegmentPredicatesForType accepted diving for wassersport")
	} else if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("validateSegmentPredicatesForType error = %v, want ErrSegmentValidation", err)
	}
	if err := validateSegmentPredicatesForType("wintersport", []sport.Predicate{{Sport: "cycling"}}); err == nil {
		t.Fatal("validateSegmentPredicatesForType accepted cycling for wintersport")
	}
	if err := validateSegmentPredicatesForType("wintersport", []sport.Predicate{{Sport: "snowshoeing"}}); err != nil {
		t.Fatalf("validateSegmentPredicatesForType rejected snowshoeing for wintersport: %v", err)
	}
	trail := "trail"
	if err := validateSegmentPredicatesForType("standard", []sport.Predicate{{Sport: "running", SubType: &trail}}); err != nil {
		t.Fatalf("validateSegmentPredicatesForType rejected running:trail for standard: %v", err)
	}
	if err := validateSegmentPredicatesForType("lift", []sport.Predicate{{Sport: "cycling"}}); err != nil {
		t.Fatalf("validateSegmentPredicatesForType rejected lift predicate: %v", err)
	}
}
