package database

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
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

func TestValidateSegmentTypeChangePredicatesUsesEffectiveSegmentType(t *testing.T) {
	mountain := "mountain"
	err := validateSegmentTypeChangePredicates("wintersport", []sport.Predicate{{Sport: "cycling", SubType: &mountain}})
	if err == nil {
		t.Fatal("validateSegmentTypeChangePredicates accepted cycling:mountain for wintersport")
	}
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("validateSegmentTypeChangePredicates error = %v, want ErrSegmentValidation", err)
	}
	if err := validateSegmentTypeChangePredicates("wintersport", []sport.Predicate{{Sport: "snowshoeing"}}); err != nil {
		t.Fatalf("validateSegmentTypeChangePredicates rejected snowshoeing for wintersport: %v", err)
	}
}

func TestSegmentPredicatesForCreateDerivesSpecializedTypeDefaults(t *testing.T) {
	preds, err := segmentPredicatesForCreate("wintersport", nil)
	if err != nil {
		t.Fatalf("segmentPredicatesForCreate(winter): %v", err)
	}
	if len(preds) == 0 {
		t.Fatal("wintersport defaults are empty")
	}
	if err := validateSegmentPredicatesForType("wintersport", preds); err != nil {
		t.Fatalf("wintersport defaults are not valid: %v", err)
	}
	for _, pred := range preds {
		if pred.Sport == "cycling" {
			t.Fatalf("wintersport defaults include cycling: %#v", preds)
		}
	}

	preds, err = segmentPredicatesForCreate("standard", nil)
	if err != nil {
		t.Fatalf("segmentPredicatesForCreate(standard): %v", err)
	}
	if preds != nil {
		t.Fatalf("standard nil predicates = %#v, want nil legacy match-all", preds)
	}
}

func TestSegmentPredicatesForUpdateDerivesSpecializedTypeDefaultsForNullOnly(t *testing.T) {
	nullRaw := json.RawMessage("null")
	preds, shouldSet, err := segmentPredicatesForUpdate("wassersport", nullRaw)
	if err != nil {
		t.Fatalf("segmentPredicatesForUpdate(null): %v", err)
	}
	if !shouldSet {
		t.Fatal("segmentPredicatesForUpdate(null) should set derived predicates")
	}
	if len(preds) == 0 {
		t.Fatal("wassersport defaults are empty")
	}
	if err := validateSegmentPredicatesForType("wassersport", preds); err != nil {
		t.Fatalf("wassersport defaults are not valid: %v", err)
	}

	preds, shouldSet, err = segmentPredicatesForUpdate("wintersport", nil)
	if err != nil {
		t.Fatalf("segmentPredicatesForUpdate(omitted): %v", err)
	}
	if shouldSet || preds != nil {
		t.Fatalf("omitted wintersport predicates = %#v, shouldSet=%v; want no update", preds, shouldSet)
	}
}

func TestSpatialCandidateSQLSkipsEmptyAllowedPredicates(t *testing.T) {
	source, err := os.ReadFile("segments.go")
	if err != nil {
		t.Fatalf("read segments.go: %v", err)
	}
	sourceText := strings.Join(strings.Fields(string(source)), " ")
	if !strings.Contains(sourceText, "jsonb_array_length(s.allowed_predicates) > 0") {
		t.Fatalf("GetSpatialCandidateSegments must prefilter allowed_predicates=[] before expanding predicates")
	}
	if !strings.Contains(sourceText, "AND s.segment_type NOT IN ('wintersport', 'wassersport')") {
		t.Fatalf("GetSpatialCandidateSegments must not let specialized null-predicate segments match all sports")
	}
}
