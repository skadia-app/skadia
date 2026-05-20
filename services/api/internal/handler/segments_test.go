package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/go-chi/chi/v5"
)

func TestCreateSegmentRejectsInvalidSegmentTypeBeforeDB(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)

	body := `{"name":"bad","segment_type":"bogus","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/segments", bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000001", false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSegmentFromActivityDoesNotWaitForSourceMatch_DB(t *testing.T) {
	pool := fusionTestPool(t)
	db := newTestHandler(t, pool).DB
	h := &Handler{DB: db}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	userID := insertTestUser(t, pool)
	pts := [][2]float64{
		{0, 0.00},
		{0, 0.25},
		{0, 0.50},
		{0, 0.75},
		{0, 1.00},
	}
	actID := insertActivityWithTrack(t, db, userID, "cycling", "LINESTRING(0 0, 0.25 0, 0.5 0, 0.75 0, 1 0)", pts)

	body := `{
		"name":"source-match",
		"sport_group":"bike",
		"segment_type":"standard",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[0.25,0],[0.5,0],[0.75,0],[1,0]]}",
		"source_activity_id":"` + actID + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/segments", bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if seg.SourceActivityID == nil || *seg.SourceActivityID != actID {
		t.Fatalf("source_activity_id = %v, want %s", seg.SourceActivityID, actID)
	}

	var effortCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2`,
		actID, seg.ID,
	).Scan(&effortCount); err != nil {
		t.Fatalf("count source effort: %v", err)
	}
	if effortCount != 0 {
		t.Fatalf("source effort count before background match = %d, want 0", effortCount)
	}
}

func TestUpdateSegmentReturnsValidationErrorForIneligiblePredicate_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{SegmentType: "wassersport"})
	body := `{"allowed_predicates":[{"sport":"diving"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/segments/"+segID, bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateSegmentRejectsTypeChangeWithExistingIneligiblePredicate_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{SegmentType: "standard"})
	if _, err := pool.Exec(context.Background(), `
		UPDATE segments
		SET sport_group = 'bike',
		    allowed_predicates = '[{"sport":"cycling","sub_type":"mountain"}]'::jsonb
		WHERE id = $1`, segID); err != nil {
		t.Fatalf("seed segment predicates: %v", err)
	}

	body := `{"segment_type":"wintersport"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/segments/"+segID, bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSubTypeResponsesAreDeterministic(t *testing.T) {
	got := subTypeResponses(sport.SportDef{
		SportType: "test",
		SubTypes: map[string]sport.SubTypeDef{
			"z_last":  {SubType: "z_last", LabelDE: "Z"},
			"a_first": {SubType: "a_first", LabelDE: "A"},
		},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SubType != "a_first" || got[1].SubType != "z_last" {
		t.Fatalf("subtype order = %#v, want sorted by sub_type", got)
	}
}

func TestSubTypeResponsesUseTaxonomyOrder(t *testing.T) {
	cycling, ok := sport.GetSportDef("cycling")
	if !ok {
		t.Fatal("missing cycling sport definition")
	}
	got := subTypeResponses(cycling)
	want := []string{"road", "mountain", "downhill", "recumbent", "cyclocross", "hand_cycling", "track_cycling", "indoor_cycling", "spin", "gravel_cycling", "unicycling", "mixed_surface", "bmx"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, wantSubType := range want {
		if got[i].SubType != wantSubType {
			t.Fatalf("subtype[%d] = %q, want taxonomy order %q", i, got[i].SubType, wantSubType)
		}
	}
}

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
		{"Schotter", "Schotter (grob)"}, // historic: promoted to grob
		{"Schotter (fein)", "Schotter (fein)"},
		{"Schotter (grob)", "Schotter (grob)"},
		{"Erde", "Naturweg"}, // historic
		{"Naturweg", "Naturweg"},
		{"Naturweg (fest)", "Naturweg (fest)"}, // user-only sub-type: pass through
		{"Holz", "Holz"},
		{"Sportbelag", "Sportbelag"},
		{"Schnee", "Schnee"},
		{"Wechselnd", "Wechselnd"},  // editor-only: pass through
		{"   Asphalt  ", "Asphalt"}, // trim whitespace
		{"Unbekannt", "Unbekannt"},
		{"NonsenseValue", "NonsenseValue"}, // unknown stays raw (UI falls back)
	}
	for _, c := range cases {
		got := normalizeSurfaceLabel(c.input)
		if got != c.want {
			t.Errorf("normalizeSurfaceLabel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
