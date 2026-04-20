package database

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure unit tests — no DB required
// ---------------------------------------------------------------------------

// TestExpandDegForRadius verifies the degree-conversion formula.
// At 80 m radius the expected value is 80*1.5/55660 ≈ 0.002156.
func TestExpandDegForRadius(t *testing.T) {
	got := expandDegForRadius(80)
	want := (80 * 1.5) / 55660.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expandDegForRadius(80) = %v, want %v", got, want)
	}
}

// TestFindSummitsAlongGeom_EmptyGuards verifies early-return behaviour for
// invalid / missing inputs without requiring a database connection.
func TestFindSummitsAlongGeom_EmptyGuards(t *testing.T) {
	// radiusM = 0 → nil, nil regardless of geomWKB content.
	result, err := FindSummitsAlongGeom(nil, nil, []byte{0x01}, 0)
	if err != nil {
		t.Errorf("radiusM=0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM=0: expected nil result, got %v", result)
	}

	// negative radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, []byte{0x01}, -5)
	if err != nil {
		t.Errorf("radiusM<0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM<0: expected nil result, got %v", result)
	}

	// empty geomWKB with valid radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, []byte{}, 80)
	if err != nil {
		t.Errorf("empty WKB: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("empty WKB: expected nil result, got %v", result)
	}

	// nil geomWKB with valid radius → nil, nil.
	result, err = FindSummitsAlongGeom(nil, nil, nil, 80)
	if err != nil {
		t.Errorf("nil WKB: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("nil WKB: expected nil result, got %v", result)
	}
}

// TestFindSummitsAlongActivityTrack_ZeroRadius verifies early-return for
// radiusM ≤ 0 without a DB connection.
func TestFindSummitsAlongActivityTrack_ZeroRadius(t *testing.T) {
	result, err := FindSummitsAlongActivityTrack(nil, nil, "any-uuid", 0)
	if err != nil {
		t.Errorf("radiusM=0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM=0: expected nil result, got %v", result)
	}

	result, err = FindSummitsAlongActivityTrack(nil, nil, "any-uuid", -1)
	if err != nil {
		t.Errorf("radiusM<0: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("radiusM<0: expected nil result, got %v", result)
	}
}

// TestPassedSummitJSON verifies that the struct marshals to the expected
// JSON shape (field names and nullable EleM).
func TestPassedSummitJSON(t *testing.T) {
	ele := 2962.5
	s := PassedSummit{
		OsmID:    123456789,
		Name:     "Großglockner",
		EleM:     &ele,
		Category: "summit",
		DistM:    42,
		Lat:      47.0742,
		Lng:      12.6942,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		`"osm_id":123456789`,
		`"name":"Großglockner"`,
		`"ele_m":2962.5`,
		`"category":"summit"`,
		`"dist_m":42`,
		`"lat":47.0742`,
		`"lng":12.6942`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing %q in: %s", want, js)
		}
	}
}

// TestPassedSummitJSON_NullEle verifies that nil EleM serialises as JSON null.
func TestPassedSummitJSON_NullEle(t *testing.T) {
	s := PassedSummit{
		OsmID:    1,
		Name:     "Unnamed Pass",
		EleM:     nil,
		Category: "pass",
		DistM:    10,
		Lat:      46.0,
		Lng:      11.0,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"ele_m":null`) {
		t.Errorf("expected ele_m:null in: %s", string(b))
	}
}
