package routing

import (
	"encoding/json"
	"testing"
)

func TestIsT0FormCompliant_AllValid(t *testing.T) {
	raw := json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650},{"dist_m":250,"ele_m":720}]`)
	if !IsT0FormCompliant(2, raw) {
		t.Error("expected true for valid T0 profile with 2 segments")
	}
}

func TestIsT0FormCompliant_EmptySegments(t *testing.T) {
	raw := json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650}]`)
	if IsT0FormCompliant(0, raw) {
		t.Error("expected false when numSegments == 0")
	}
}

func TestIsT0FormCompliant_SegmentWithEmptyProfile(t *testing.T) {
	// nil rawProfile
	if IsT0FormCompliant(1, nil) {
		t.Error("expected false for nil rawProfile")
	}
	// null rawProfile
	if IsT0FormCompliant(1, json.RawMessage("null")) {
		t.Error("expected false for null rawProfile")
	}
	// empty JSON array
	if IsT0FormCompliant(1, json.RawMessage("[]")) {
		t.Error("expected false for empty JSON array")
	}
}

func TestIsT0FormCompliant_FirstDistMNonZero(t *testing.T) {
	// Profile starts at dist_m > 0 — approach-exclusive (Pre-T0 layout).
	raw := json.RawMessage(`[{"dist_m":50,"ele_m":593},{"dist_m":150,"ele_m":650}]`)
	if IsT0FormCompliant(1, raw) {
		t.Error("expected false when profile[0].dist_m != 0")
	}
}

func TestIsT0FormCompliant_NonMonotonicDistM(t *testing.T) {
	// Duplicate distance value — not strictly increasing.
	raw := json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650},{"dist_m":100,"ele_m":700}]`)
	if IsT0FormCompliant(1, raw) {
		t.Error("expected false for non-monotonic (equal) dist_m")
	}
	// Decreasing distance.
	raw2 := json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650},{"dist_m":50,"ele_m":700}]`)
	if IsT0FormCompliant(1, raw2) {
		t.Error("expected false for decreasing dist_m")
	}
}

func TestIsT0FormCompliant_NullEleMAllowed(t *testing.T) {
	// null ele_m (DEM no-data) must not disqualify the profile.
	raw := json.RawMessage(`[{"dist_m":0,"ele_m":null},{"dist_m":100,"ele_m":null},{"dist_m":200,"ele_m":650}]`)
	if !IsT0FormCompliant(1, raw) {
		t.Error("expected true — null ele_m is legal per Spec §6 Class A")
	}
}

func TestIsT0FormCompliant_SinglePoint(t *testing.T) {
	// A single point with dist_m == 0 is technically valid (monotonicity trivially holds).
	raw := json.RawMessage(`[{"dist_m":0,"ele_m":593}]`)
	if !IsT0FormCompliant(1, raw) {
		t.Error("expected true for single-point profile starting at dist_m=0")
	}
}

func TestIsT0FormCompliant_MalformedJSON(t *testing.T) {
	raw := json.RawMessage(`not-valid-json`)
	if IsT0FormCompliant(1, raw) {
		t.Error("expected false for malformed JSON")
	}
}
