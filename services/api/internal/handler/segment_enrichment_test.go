// services/api/internal/handler/segment_enrichment_test.go
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

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// TestEnrichmentHandler_400_TooManyCoordinates verifies that the handler
// rejects requests with more than maxCoordinates (5000) coordinate pairs
// regardless of seg_type — including seg_types that would normally short-circuit
// (wintersport/wassersport). The maxCoordinates check is a Defense-in-Depth
// guard against DoS payloads and MUST fire before the seg_type short-circuit.
// No DB required — the check fires before any graph/PostGIS call.
func TestEnrichmentHandler_400_TooManyCoordinates(t *testing.T) {
	cases := []struct {
		name    string
		segType string
	}{
		{"trail", "trail"},
		{"wintersport", "wintersport"},
		{"wassersport", "wassersport"},
	}

	// 5001 coordinates exceed the maxCoordinates = 5000 limit.
	coords := make([][2]float64, 5001)
	for i := range coords {
		coords[i] = [2]float64{float64(i) * 0.0001, 47.0}
	}

	h := &Handler{DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)
	userID := "00000000-0000-0000-0000-000000000001"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"coordinates": coords,
				"seg_type":    tc.segType,
			})

			req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("seg_type=%q: status = %d, want 400 — maxCoordinates check must precede seg_type short-circuit", tc.segType, rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, "too many coordinates") {
				t.Errorf("seg_type=%q: response body %q does not contain 'too many coordinates'", tc.segType, body)
			}
		})
	}
}

func TestCoreBlocksSegmentEnrichmentSuggestAfterValidation(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}, DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{11.0, 47.0}, {11.1, 47.1}},
		"seg_type":    "trail",
	})

	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000003", false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "feature_unavailable") {
		t.Fatalf("response body %q does not contain feature_unavailable", body)
	}
}

// TestEnrichmentHandler_RateLimit429 verifies that repeated rapid requests
// for the same user are rate-limited to HTTP 429.
// Requires DB because the first requests (within burst) reach PostGIS.
func TestEnrichmentHandler_RateLimit429(t *testing.T) {
	pool := fusionTestPool(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	authRL := middleware.NewRateLimiter(ctx)

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.With(middleware.RateLimitPerUser(authRL, "segment-enrichment:", 30.0/60, 5)).
		Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := insertTestUser(t, pool)
	coords := [][2]float64{{11.0, 47.0}, {11.1, 47.1}}
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": coords,
		"seg_type":    "trail",
	})

	var gotRateLimited bool
	for i := 0; i < 35; i++ {
		req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			gotRateLimited = true
			break
		}
	}
	if !gotRateLimited {
		t.Error("expected HTTP 429 within 35 requests, but never received it")
	}
}

// TestEnrichmentHandler_Wassersport_ShortCircuit verifies that wassersport
// segment requests short-circuit before any OSM/graph call and return 200 OK
// with Reason = "wassersport".
// No DB required — the handler exits before any graph/PostGIS call.
func TestEnrichmentHandler_Wassersport_ShortCircuit(t *testing.T) {
	h := &Handler{DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := "00000000-0000-0000-0000-000000000002"
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{11.0, 47.0}, {11.1, 47.1}},
		"seg_type":    "wassersport",
	})

	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp enrichment.SegmentEnrichmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Reason != "wassersport" {
		t.Errorf("Reason = %q, want %q", resp.Reason, "wassersport")
	}
}

// TestEnrichmentHandler_NoGraph_Returns200 verifies that when no routing graph
// is loaded (GraphManager == nil) and PostGIS finds no OSM ways, the handler
// returns 200 OK with Reason = "no_graph" instead of an error.
// Requires DB because the PostGIS fallback path is exercised.
func TestEnrichmentHandler_NoGraph_Returns200(t *testing.T) {
	pool := fusionTestPool(t)

	// Handler with no GraphManager — simulates "graph not initialised" state.
	// newTestHandler does not set GraphManager, so it stays nil by default.
	h := newTestHandler(t, pool)

	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := insertTestUser(t, pool)
	// Atlantic mid-ocean coords — osm_ways guaranteed empty here.
	// Using -40°W / 0°N (South Atlantic) which contains no OSM way data
	// in any realistic test DB seeded from land-based OSM extracts.
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{-40.0, 0.0}, {-40.001, 0.001}},
		"seg_type":    "standard",
	})

	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — handler must not return 503 when graph is absent", rec.Code)
	}
	var resp enrichment.SegmentEnrichmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Exact assertion: the no-graph/no-OSM path must return "no_graph", not any
	// other reason. If a future refactor moves the short-circuit or changes the
	// reason string, this test will catch it.
	if resp.Reason != "no_graph" {
		t.Errorf("Reason = %q, want %q — graph is nil and OSM ways absent at mid-Atlantic coords", resp.Reason, "no_graph")
	}
}

func TestEnrichmentHandler_PostGISReturnsSurfaceSacMtbSuggestions(t *testing.T) {
	pool := fusionTestPool(t)

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	osmID := -time.Now().UnixNano()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO osm_ways (osm_id, highway, surface, sac_scale, mtb_scale, length_m, geom)
		VALUES (
			$1,
			'path',
			'asphalt',
			'mountain_hiking',
			'2',
			100,
			ST_SetSRID(ST_MakeLine(ARRAY[
				ST_MakePoint(0, 0),
				ST_MakePoint(0, 1)
			]), 4326)
		)
	`, osmID)
	if err != nil {
		t.Fatalf("insert osm_ways fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM osm_ways WHERE osm_id = $1`, osmID)
	})

	userID := insertTestUser(t, pool)
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{0, 0}, {0, 1}},
		"seg_type":    "trail",
	})

	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp enrichment.SegmentEnrichmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Surface == nil || resp.Surface.Dominant != "Asphalt" {
		t.Fatalf("Surface.Dominant = %#v, want Asphalt", resp.Surface)
	}
	if resp.SacScale == nil || resp.SacScale.Dominant != "T2" || resp.SacScale.Max != "T2" {
		t.Fatalf("SacScale = %#v, want dominant/max T2", resp.SacScale)
	}
	if resp.MtbScale == nil || resp.MtbScale.Dominant != "S2" || resp.MtbScale.Max != "S2" {
		t.Fatalf("MtbScale = %#v, want dominant/max S2", resp.MtbScale)
	}
	if resp.CoveragePct != 1 {
		t.Fatalf("CoveragePct = %v, want 1", resp.CoveragePct)
	}
}
