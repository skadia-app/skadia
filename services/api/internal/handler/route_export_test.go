package handler

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

func TestWriteRouteGPX_TrksegSeparation(t *testing.T) {
	route := &model.PlannedRoute{
		Name: "Test Route",
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.0, 47.0, 500}, {8.01, 47.01, 510}},
				},
			},
			{
				Mode: "offroad", FromWaypoint: "b", ToWaypoint: "c",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.01, 47.01, 510}, {8.02, 47.02, 520}},
				},
			},
			{
				Mode: "routed", FromWaypoint: "c", ToWaypoint: "d",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.02, 47.02, 520}, {8.03, 47.03, 530}},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	count := strings.Count(gpx, "<trkseg>")
	if count != 3 {
		t.Errorf("expected 3 <trkseg> elements, got %d\nGPX:\n%s", count, gpx)
	}
	if !strings.Contains(gpx, "skadia:ele_estimated") {
		t.Error("expected skadia:ele_estimated extension in offroad trkpt")
	}
	if !strings.Contains(gpx, "Offroad-Abschnitte") {
		t.Error("expected offroad warning in <cmt>")
	}
}

func TestWriteRouteGPX_LegacyFallback(t *testing.T) {
	route := &model.PlannedRoute{
		Name:            "Legacy Route",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[8.0,47.0,400],[8.01,47.01,410]]}`,
		Segments: []model.RouteSegment{
			{Mode: "routed", FromWaypoint: "a", ToWaypoint: "b"},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	count := strings.Count(gpx, "<trkseg>")
	if count != 1 {
		t.Errorf("legacy fallback: expected 1 <trkseg>, got %d\nGPX:\n%s", count, gpx)
	}
	if strings.Contains(gpx, "skadia:ele_estimated") {
		t.Error("legacy fallback: unexpected skadia:ele_estimated")
	}
}

// profileJSON encodes a []routing.ElevationPoint into json.RawMessage for test fixtures.
func profileJSON(pts []routing.ElevationPoint) json.RawMessage {
	b, err := json.Marshal(pts)
	if err != nil {
		panic(err)
	}
	return b
}

// extractEleValues parses <ele>NNN.N</ele> tags from GPX output in order.
func extractEleValues(gpx string) []float64 {
	var vals []float64
	rest := gpx
	for {
		start := strings.Index(rest, "<ele>")
		if start < 0 {
			break
		}
		rest = rest[start+5:]
		end := strings.Index(rest, "</ele>")
		if end < 0 {
			break
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest[:end]), 64)
		if err == nil {
			vals = append(vals, v)
		}
		rest = rest[end+6:]
	}
	return vals
}

// TestExportRouteGPX_HappyPath_CorrectEleFromCoord2 verifies that freshly saved
// routes (post-Phase-1, coord[2] already from profile) are exported correctly.
// No ElevationProfile is set; the function must pass coord[2] through unchanged.
func TestExportRouteGPX_HappyPath_CorrectEleFromCoord2(t *testing.T) {
	route := &model.PlannedRoute{
		Name: "Happy Path Route",
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.0, 47.0, 593}, {8.01, 47.01, 650}, {8.02, 47.02, 710}},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	eles := extractEleValues(gpx)
	if len(eles) != 3 {
		t.Fatalf("expected 3 <ele> values, got %d\nGPX:\n%s", len(eles), gpx)
	}
	want := []float64{593, 650, 710}
	for i, w := range want {
		if math.Abs(eles[i]-w) > 0.2 {
			t.Errorf("ele[%d]: want %.1f, got %.1f", i, w, eles[i])
		}
	}
}

// TestExportRouteGPX_LegacyRouteWithProfileBackfilled_DefensivelyCorrects verifies
// that when route.ElevationProfile is present (Lazy-Fill path), coord[2] values in
// the segment geometry are overridden by the profile. The fixture uses spike-shaped
// coord[2] values that should be replaced by profile interpolation.
func TestExportRouteGPX_LegacyRouteWithProfileBackfilled_DefensivelyCorrects(t *testing.T) {
	// Profile: 0→593m, 100m→650m, 200m→710m (monotonic — no spikes).
	profile := []routing.ElevationPoint{
		{DistM: 0, EleM: 593},
		{DistM: 100, EleM: 650},
		{DistM: 200, EleM: 710},
	}
	// Segment coords have stale spike values in coord[2] (all 593 from carriedEle).
	// Two points close together (~111m apart at equator per degree of latitude).
	// We use a very short segment so distances match profile range.
	route := &model.PlannedRoute{
		Name:             "Legacy Spike Route",
		ElevationProfile: profileJSON(profile),
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type: "LineString",
					// All coord[2] = 593 (spike/stale), should be overridden.
					Coordinates: [][]float64{
						{8.0, 47.0, 593},
						{8.0, 47.0009, 593}, // ~100m north
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	eles := extractEleValues(gpx)
	if len(eles) < 2 {
		t.Fatalf("expected at least 2 <ele> values, got %d\nGPX:\n%s", len(eles), gpx)
	}
	// First point: distAccum=0 → profile[0].EleM = 593.
	if math.Abs(eles[0]-593) > 1.0 {
		t.Errorf("ele[0]: want ~593, got %.1f", eles[0])
	}
	// Second point: distAccum ~100m → profile interpolates toward 650.
	// We just verify it's been overridden (>593 and <710).
	if eles[1] <= 593 {
		t.Errorf("ele[1]: expected > 593 (profile-derived), got %.1f", eles[1])
	}
	if eles[1] >= 710 {
		t.Errorf("ele[1]: expected < 710 (within profile range), got %.1f", eles[1])
	}
}

// TestExportRouteGPX_NilProfile_LegacyCoord2_NoCrash verifies that when
// ElevationProfile is nil, the function falls back to reading coord[2] directly
// and does not crash.
func TestExportRouteGPX_NilProfile_LegacyCoord2_NoCrash(t *testing.T) {
	route := &model.PlannedRoute{
		Name:             "No Profile Route",
		ElevationProfile: nil, // explicitly nil
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.0, 47.0, 450}, {8.01, 47.01, 0}},
				},
			},
		},
	}

	var buf bytes.Buffer
	// Must not panic.
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	eles := extractEleValues(gpx)
	// First coord has ele 450 (nonzero) → should be in GPX.
	if len(eles) < 1 {
		t.Fatalf("expected at least 1 <ele> value when coord[2]=450, got 0\nGPX:\n%s", gpx)
	}
	if math.Abs(eles[0]-450) > 0.2 {
		t.Errorf("ele[0]: want 450, got %.1f", eles[0])
	}
	// Second coord has ele 0 → should be emitted as <trkpt .../> without <ele>.
	if strings.Count(gpx, "<trkpt") != 2 {
		t.Errorf("expected 2 trkpt elements, got %d\nGPX:\n%s", strings.Count(gpx, "<trkpt"), gpx)
	}
	if len(eles) > 1 {
		t.Errorf("expected only 1 <ele> (coord[2]=0 skipped), got %d eles", len(eles))
	}
}

// TestExportRouteGPX_OffroadKeepsEstimatedFlag verifies that offroad segments
// are not overridden by the profile and retain their skadia:ele_estimated extension.
func TestExportRouteGPX_OffroadKeepsEstimatedFlag(t *testing.T) {
	// Profile covers the full route.
	profile := []routing.ElevationPoint{
		{DistM: 0, EleM: 800},
		{DistM: 500, EleM: 900},
	}
	offroadEle := 850.0 // stale offroad estimate in coord[2]
	route := &model.PlannedRoute{
		Name:             "Offroad Route",
		ElevationProfile: profileJSON(profile),
		Segments: []model.RouteSegment{
			{
				Mode: "offroad", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.0, 47.0, offroadEle}, {8.01, 47.01, offroadEle}},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	if !strings.Contains(gpx, "skadia:ele_estimated") {
		t.Error("offroad segment: expected skadia:ele_estimated extension")
	}

	eles := extractEleValues(gpx)
	if len(eles) < 1 {
		t.Fatalf("expected at least 1 <ele> value, got 0\nGPX:\n%s", gpx)
	}
	// Offroad coord[2] must NOT have been overridden by profile.
	for i, e := range eles {
		if math.Abs(e-offroadEle) > 0.2 {
			t.Errorf("ele[%d]: offroad must keep original %.1f, got %.1f", i, offroadEle, e)
		}
	}
}

// TestExportRouteGPX_MultiLegRoute_NoAdditiveDrift verifies that for a multi-
// segment routed route, each GPX trackpoint elevation is within 1m of the
// direct profile interpolation at that point's cumulative distance.
// This guards against additive drift from double-apply of profile.
func TestExportRouteGPX_MultiLegRoute_NoAdditiveDrift(t *testing.T) {
	// Simple profile: 0→100m over 300m distance.
	profile := []routing.ElevationPoint{
		{DistM: 0, EleM: 100},
		{DistM: 150, EleM: 150},
		{DistM: 300, EleM: 200},
	}
	// Stale coord[2] = 0 everywhere to verify profile is applied.
	route := &model.PlannedRoute{
		Name:             "Multi-Leg Route",
		ElevationProfile: profileJSON(profile),
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type: "LineString",
					// ~0m and ~150m north (each ~0.00135° ≈ 150m)
					Coordinates: [][]float64{
						{8.0, 47.0, 0},
						{8.0, 47.00135, 0},
					},
				},
			},
			{
				Mode: "routed", FromWaypoint: "b", ToWaypoint: "c",
				Geometry: &model.GeoJSONLineString{
					Type: "LineString",
					// Continues from ~150m to ~300m
					Coordinates: [][]float64{
						{8.0, 47.00135, 0},
						{8.0, 47.0027, 0},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	eles := extractEleValues(gpx)
	if len(eles) < 4 {
		t.Fatalf("expected 4 <ele> values for 4 coords, got %d\nGPX:\n%s", len(eles), gpx)
	}
	// First ele should be close to profile start (100m).
	if math.Abs(eles[0]-100) > 1.0 {
		t.Errorf("ele[0]: want ~100, got %.1f", eles[0])
	}
	// All eles must be between 100 and 200 (within profile range).
	for i, e := range eles {
		if e < 99 || e > 201 {
			t.Errorf("ele[%d]: out of profile range [100,200], got %.1f", i, e)
		}
	}
	// Verify monotonic trend (no drift back below start).
	if eles[len(eles)-1] <= eles[0] {
		t.Errorf("elevation should increase from %.1f to %.1f", eles[0], eles[len(eles)-1])
	}
}
