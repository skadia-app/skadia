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

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// TestEnrichmentHandler_400_TooManyCoordinates verifies that the handler
// rejects requests with more than maxCoordinates (5000) coordinate pairs.
// No DB required — the check fires before any graph/PostGIS call.
func TestEnrichmentHandler_400_TooManyCoordinates(t *testing.T) {
	// 5001 coordinates exceed the maxCoordinates = 5000 limit.
	coords := make([][2]float64, 5001)
	for i := range coords {
		coords[i] = [2]float64{float64(i) * 0.0001, 47.0}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": coords,
		"seg_type":    "trail",
	})

	h := &Handler{DB: &database.DB{Pool: nil}}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := "00000000-0000-0000-0000-000000000001"
	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "too many coordinates") {
		t.Errorf("response body %q does not contain 'too many coordinates'", body)
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
	h := newTestHandler(t, pool)

	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := insertTestUser(t, pool)
	// Coordinates in a remote area unlikely to have OSM ways in a test DB.
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{0.0, 0.0}, {0.001, 0.001}},
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
	if resp.Reason == "" {
		t.Errorf("Reason is empty, want non-empty reason (e.g. 'no_graph') when OSM data absent")
	}
}

