// services/api/internal/handler/segment_enrichment_test.go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
    
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestEnrichmentHandler_400_TooManyCoordinates(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
    
	// 6000 Koordinaten (> 5000 Limit)
	coords := make([][2]float64, 6000)
	for i := range coords {
		coords[i] = [2]float64{float64(i) * 0.0001, 47.0}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": coords,
		"seg_type":    "trail",
	})
    
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)
    
	userID := insertTestUser(t, pool)
	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
    
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "too many coordinates")
}

func TestEnrichmentHandler_RateLimit429(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
    
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
	require.True(t, gotRateLimited, "expected 429 within 35 requests")
}

func TestEnrichmentHandler_Wassersport_ShortCircuit(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/enrichment-suggest", h.EnrichmentSuggest)

	userID := insertTestUser(t, pool)
	body, _ := json.Marshal(map[string]interface{}{
		"coordinates": [][2]float64{{11.0, 47.0}, {11.1, 47.1}},
		"seg_type":    "wassersport",
	})

	req := httptest.NewRequest("POST", "/api/v1/segments/enrichment-suggest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp enrichment.SegmentEnrichmentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "wassersport", resp.Reason)
}
