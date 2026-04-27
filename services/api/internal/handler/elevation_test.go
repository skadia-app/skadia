package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// makeAtVerticesRequest builds a POST body for /elevation/profile with
// at_vertices=true and the given GeoJSON LineString coordinates.
func makeAtVerticesRequest(coords [][]float64, extraFields map[string]any) []byte {
	geom := map[string]any{
		"type":        "LineString",
		"coordinates": coords,
	}
	geomBytes, _ := json.Marshal(geom)

	body := map[string]any{
		"geometry":    json.RawMessage(geomBytes),
		"at_vertices": true,
	}
	for k, v := range extraFields {
		body[k] = v
	}
	b, _ := json.Marshal(body)
	return b
}

// TestElevationProfile_AtVertices_HappyPath verifies that two vertices produce
// two profile points with valid elevations.
func TestElevationProfile_AtVertices_HappyPath(t *testing.T) {
	const fixedDm int32 = 1234 * 10 // 1234 m in decimeters
	h := &Handler{DEMSvc: &mockDEMService{elevDm: fixedDm}}
	coords := [][]float64{{8.5, 47.5}, {8.6, 47.6}}
	body := makeAtVerticesRequest(coords, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/elevation/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Inject a fake user ID so RequireUserID passes.
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))

	w := httptest.NewRecorder()
	h.ElevationProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Profile []database.ElevationPoint `json:"profile"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Profile) != 2 {
		t.Fatalf("profile len = %d, want 2", len(resp.Profile))
	}

	// First point: fraction=0, dist=0
	pt0 := resp.Profile[0]
	if pt0.Elevation == nil {
		t.Error("profile[0].elevation is nil, want non-nil")
	} else if math.Abs(*pt0.Elevation-1234.0) > 0.01 {
		t.Errorf("profile[0].elevation = %f, want 1234.0", *pt0.Elevation)
	}
	if pt0.Fraction != 0 {
		t.Errorf("profile[0].fraction = %f, want 0", pt0.Fraction)
	}

	// Second point: fraction=1
	pt1 := resp.Profile[1]
	if pt1.Elevation == nil {
		t.Error("profile[1].elevation is nil, want non-nil")
	}
	if math.Abs(pt1.Fraction-1.0) > 0.001 {
		t.Errorf("profile[1].fraction = %f, want 1.0", pt1.Fraction)
	}
}

// TestElevationProfile_AtVertices_OverCap_400 verifies that more than 50000
// vertices returns 400 with code "polyline_too_long".
func TestElevationProfile_AtVertices_OverCap_400(t *testing.T) {
	h := &Handler{DEMSvc: &mockDEMService{}}
	// Build 50001 coordinates.
	coords := make([][]float64, 50001)
	for i := range coords {
		coords[i] = []float64{8.5, 47.5}
	}
	body := makeAtVerticesRequest(coords, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/elevation/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))

	w := httptest.NewRecorder()
	h.ElevationProfile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var errResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&errResp); err == nil {
		// WriteError uses title as the error identifier; code is not a field.
		if title, _ := errResp["title"].(string); title != "polyline_too_long" {
			t.Errorf("error title = %q, want \"polyline_too_long\"", title)
		}
	}
}

// TestElevationProfile_AtVertices_ExactlyAtCap_NoError verifies that exactly
// 50000 vertices returns 200 OK (strict > boundary, not >=).
func TestElevationProfile_AtVertices_ExactlyAtCap_NoError(t *testing.T) {
	h := &Handler{DEMSvc: &mockDEMService{elevDm: 500 * 10}}
	coords := make([][]float64, 50000)
	for i := range coords {
		coords[i] = []float64{8.5, 47.5}
	}
	body := makeAtVerticesRequest(coords, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/elevation/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))

	w := httptest.NewRecorder()
	h.ElevationProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for exactly 50000 vertices; body: %s", w.Code, w.Body.String())
	}
}

// TestElevationProfile_AtVerticesIgnoresNumPoints verifies that num_points is
// silently ignored when at_vertices=true (no error returned).
func TestElevationProfile_AtVerticesIgnoresNumPoints(t *testing.T) {
	h := &Handler{DEMSvc: &mockDEMService{elevDm: 1000 * 10}}
	coords := [][]float64{{8.5, 47.5}, {8.6, 47.6}}
	body := makeAtVerticesRequest(coords, map[string]any{"num_points": 100})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/elevation/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))

	w := httptest.NewRecorder()
	h.ElevationProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

// TestElevationProfile_LegacyResample_Unchanged verifies that at_vertices=false
// (or absent) does NOT trigger the at_vertices branch (no polyline_too_long error
// for a small geometry, no cap check applied).
//
// The legacy resample path requires a real *sql.DB which is not available in unit
// tests. Rather than silently swallowing a nil-DB panic via recover() — which masks
// regressions by producing a false-green — we skip explicitly with a clear message.
//
// TODO: legacy resample path requires *sql.DB; mock currently not built.
// Skip rather than recover() to avoid silent green on regression.
//
// Plan-End-Critic finding #12 (T0 elevation-profile-gap-fill).
func TestElevationProfile_LegacyResample_Unchanged(t *testing.T) {
	t.Skip("legacy resample test requires DB mock — see plan-end critic finding #12")
}

// --- test helpers ---

// mockDEMService implements a minimal interface for testing ElevationProfile.
// It returns elevDm for every lookup.
type mockDEMService struct {
	elevDm int32
}

func (m *mockDEMService) ElevationBatch(lats, lons []int32) ([]int32, error) {
	out := make([]int32, len(lats))
	for i := range out {
		out[i] = m.elevDm
	}
	return out, nil
}

func (m *mockDEMService) Health() dem.Health {
	return dem.Health{}
}

func (m *mockDEMService) Close() {}
