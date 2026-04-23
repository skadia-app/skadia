package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

// TestGetSegment_IncludeGeoJSON_FlagControlsResponseField verifies that the
// include_geojson query parameter controls whether TrackGeoJSON is populated
// in the GetSegment response.
//
//   - Without the flag → TrackGeoJSON is nil (field omitted from JSON).
//   - With include_geojson=true → TrackGeoJSON contains a GeoJSON LineString.
//
// Requires DB. The test inserts a segment with a known track geometry so that
// the GeoJSON content can be asserted on.
func TestGetSegment_IncludeGeoJSON_FlagControlsResponseField(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	r := chi.NewRouter()
	r.Get("/api/v1/segments/{id}", h.GetSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track: "LINESTRING(10.0 47.0, 10.1 47.1)",
	})

	t.Run("without_flag_track_geojson_is_nil", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/segments/"+segID, nil)
		req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var seg model.Segment
		if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if seg.TrackGeoJSON != nil {
			t.Errorf("TrackGeoJSON = %q, want nil when include_geojson not set", *seg.TrackGeoJSON)
		}
	})

	t.Run("with_flag_track_geojson_is_linestring", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/segments/"+segID+"?include_geojson=true", nil)
		req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var seg model.Segment
		if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if seg.TrackGeoJSON == nil {
			t.Fatal("TrackGeoJSON is nil, want GeoJSON LineString string")
		}
		if !strings.Contains(*seg.TrackGeoJSON, `"LineString"`) {
			t.Errorf("TrackGeoJSON = %q, want string containing \"LineString\"", *seg.TrackGeoJSON)
		}
	})
}

func TestNormalizeSurfaceLabel(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"", ""},
		{"Asphalt", "Asphalt"},
		{"Pflaster", "Pflaster"},
		{"Schotter", "Schotter (grob)"},        // historic: promoted to grob
		{"Schotter (fein)", "Schotter (fein)"},
		{"Schotter (grob)", "Schotter (grob)"},
		{"Erde", "Naturweg"},                   // historic
		{"Naturweg", "Naturweg"},
		{"Naturweg (fest)", "Naturweg (fest)"}, // user-only sub-type: pass through
		{"Holz", "Holz"},
		{"Sportbelag", "Sportbelag"},
		{"Schnee", "Schnee"},
		{"Wechselnd", "Wechselnd"},             // editor-only: pass through
		{"   Asphalt  ", "Asphalt"},            // trim whitespace
		{"Unbekannt", "Unbekannt"},
		{"NonsenseValue", "NonsenseValue"},     // unknown stays raw (UI falls back)
	}
	for _, c := range cases {
		got := normalizeSurfaceLabel(c.input)
		if got != c.want {
			t.Errorf("normalizeSurfaceLabel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
