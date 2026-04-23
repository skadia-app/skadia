package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

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
