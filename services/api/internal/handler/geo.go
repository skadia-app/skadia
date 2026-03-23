package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetVisitedPOIs returns GeoJSON FeatureCollection of visited POIs.
// GET /api/v1/geo/visited-pois
func (h *Handler) GetVisitedPOIs(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	pois, err := h.DB.GetVisitedPOIs(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get visited pois", r.URL.Path)
		return
	}
	features := make([]map[string]any, 0, len(pois))
	for _, p := range pois {
		features = append(features, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{p.Lng, p.Lat},
			},
			"properties": map[string]any{
				"name":     p.Name,
				"count":    p.Count,
				"poi_type": p.PoiType,
			},
		})
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"type":     "FeatureCollection",
		"features": features,
	})
}
