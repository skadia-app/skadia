package main

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// approxEq reports whether a and b are within tol metres of each other.
// Used to compare haversine results against hand-computed expectations
// without depending on every digit of the Earth-radius constant.
func approxEq(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// TestHavMKnownDistances pins the haversine helper to two reference cases:
// one degree of latitude (≈ 111.2 km anywhere) and one degree of longitude
// at the equator (≈ 111.3 km, then shrinks with cos(latitude)). If a future
// edit changes the formula or the radius constant, these break.
func TestHavMKnownDistances(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lng1, lat2, lng2 float64
		want                   float64
		tol                    float64
	}{
		{"one degree latitude", 0, 0, 1, 0, 111195, 100},
		{"one degree longitude at equator", 0, 0, 0, 1, 111195, 100},
		{"identical points", 47.0, 11.0, 47.0, 11.0, 0, 0.001},
		{"100m east at 47N", 47.0, 11.0, 47.0, 11.001317, 100, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := havM(tc.lat1, tc.lng1, tc.lat2, tc.lng2)
			if !approxEq(got, tc.want, tc.tol) {
				t.Errorf("havM = %.2f, want %.2f ±%.2f", got, tc.want, tc.tol)
			}
		})
	}
}

// TestToleranceBand pins the four-band glyph thresholds documented in
// geo-lift-detection.md. The recipe table in the docs is read by humans,
// the band labels by tools — a silent renumber here would make eval reports
// disagree with the docs. Each boundary is checked exactly so off-by-one
// shifts in the comparator (`<` vs `<=`) surface immediately.
func TestToleranceBand(t *testing.T) {
	cases := []struct {
		offset float64
		want   string
	}{
		{0, "✓"},
		{20, "✓"},
		{20.001, "○"},
		{30, "○"},
		{30.001, "⚠"},
		{80, "⚠"},
		{80.001, "✗"},
		{500, "✗"},
	}
	for _, c := range cases {
		if got := toleranceBand(c.offset); got != c.want {
			t.Errorf("toleranceBand(%v) = %q, want %q", c.offset, got, c.want)
		}
	}
}

// TestMatchRideOrientationAware checks the matcher's two contracts:
// (1) it tries both endpoint orientations and keeps the smaller maximum
// offset, (2) it picks the lift with the smaller resulting offset across
// the candidate pool. Coordinates are synthetic; only Δ-relationships
// matter for the assertions, not absolute geography.
func TestMatchRideOrientationAware(t *testing.T) {
	// A synthetic lift that runs from p1 (south) to p2 (north).
	lifts := []boundLift{
		{ID: "alpha", P1: []float64{0.0, 0.0}, P2: []float64{0.01, 0.0}},
		// A second lift far away — should never win in these tests.
		{ID: "beta", P1: []float64{1.0, 1.0}, P2: []float64{1.01, 1.0}},
	}

	t.Run("forward orientation: ride start at p1, end at p2", func(t *testing.T) {
		// Ride starts within ~5 m of p1 and ends within ~5 m of p2.
		r := rideEndpoints{
			sLat: 0.00004, sLng: 0.00000, // ~4.4 m north of p1
			eLat: 0.01004, eLng: 0.00000, // ~4.4 m north of p2
		}
		id, orient, sOff, eOff, maxOff := matchRide(r, lifts)
		if id != "alpha" {
			t.Errorf("id = %q, want alpha", id)
		}
		if orient != "s→p1,e→p2" {
			t.Errorf("orient = %q, want s→p1,e→p2", orient)
		}
		if !approxEq(sOff, 4.45, 0.5) || !approxEq(eOff, 4.45, 0.5) {
			t.Errorf("offsets s=%.2f e=%.2f, want ~4.45m each", sOff, eOff)
		}
		if !approxEq(maxOff, 4.45, 0.5) {
			t.Errorf("maxOff = %.2f, want ~4.45m", maxOff)
		}
	})

	t.Run("reverse orientation: ride start at p2, end at p1", func(t *testing.T) {
		// Ride descends — start near p2 (top), end near p1 (bottom).
		r := rideEndpoints{
			sLat: 0.01000, sLng: 0.00001, // ~1.1 m east of p2
			eLat: 0.00000, eLng: 0.00001, // ~1.1 m east of p1
		}
		id, orient, sOff, eOff, maxOff := matchRide(r, lifts)
		if id != "alpha" {
			t.Errorf("id = %q, want alpha", id)
		}
		if orient != "s→p2,e→p1" {
			t.Errorf("orient = %q, want s→p2,e→p1 (descending ride)", orient)
		}
		// Smaller max wins — both endpoint distances should be ~1.1 m, not ~1100 m
		// (which is what we'd get if the matcher only tried one orientation).
		if maxOff > 5 {
			t.Errorf("maxOff = %.2f m — matcher likely failed to try reversed orientation", maxOff)
		}
		_ = sOff
		_ = eOff
	})

	t.Run("picks closer lift across pool", func(t *testing.T) {
		// Ride sits on alpha, far from beta — alpha must win even though both
		// are reachable in principle.
		r := rideEndpoints{
			sLat: 0.0, sLng: 0.0,
			eLat: 0.01, eLng: 0.0,
		}
		id, _, _, _, maxOff := matchRide(r, lifts)
		if id != "alpha" {
			t.Errorf("id = %q, want alpha (beta is ~157 km away)", id)
		}
		if maxOff > 1 {
			t.Errorf("maxOff = %.2f, want ~0m", maxOff)
		}
	})

	t.Run("malformed lift entries are skipped", func(t *testing.T) {
		// p1/p2 with the wrong arity must not crash the matcher.
		dirty := []boundLift{
			{ID: "broken", P1: []float64{0.0}, P2: []float64{0.01, 0.0}},
			{ID: "alpha", P1: []float64{0.0, 0.0}, P2: []float64{0.01, 0.0}},
		}
		r := rideEndpoints{sLat: 0, sLng: 0, eLat: 0.01, eLng: 0}
		id, _, _, _, _ := matchRide(r, dirty)
		if id != "alpha" {
			t.Errorf("id = %q, malformed lift must be skipped", id)
		}
	})
}

// TestPrintBoundariesIncludesGlyphAndCounts smoke-tests the output writer:
// every detected ride must appear in the table with its tolerance glyph,
// and the per-lift summary must reflect both matched and unmatched lifts.
// This catches regressions that silently drop a column or skip the summary.
func TestPrintBoundariesIncludesGlyphAndCounts(t *testing.T) {
	lifts := []boundLift{
		{ID: "alpha", P1: []float64{0.0, 0.0}, P2: []float64{0.01, 0.0}},
		{ID: "beta", P1: []float64{1.0, 1.0}, P2: []float64{1.01, 1.0}},
	}
	rides := []rideEndpoints{
		{idx: 0, sLat: 0.0, sLng: 0.0, eLat: 0.01, eLng: 0.0, ascM: 100, descM: 0},
		{idx: 1, sLat: 0.0, sLng: 0.0, eLat: 0.01, eLng: 0.0, ascM: 100, descM: 0},
	}

	var buf bytes.Buffer
	printBoundaries(&buf, rides, lifts)
	out := buf.String()

	for _, want := range []string{"alpha", "✓", "Per-Lift detected count:", "rides=[0 1]"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// beta has no matching ride; its summary line must still appear with count 0.
	if !strings.Contains(out, "beta") {
		t.Errorf("unmatched lift beta missing from per-lift summary\n--- output ---\n%s", out)
	}
}
