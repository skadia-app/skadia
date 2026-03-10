package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// POIResult is a point-of-interest search result returned to the client.
type POIResult struct {
	Name   string     `json:"name"`
	Type   string     `json:"type"`
	Ele    *float64   `json:"ele,omitempty"`
	LatLng [2]float64 `json:"latlng"`
	OsmID  int64      `json:"osm_id"`
}

// SearchPOIs handles GET /api/v1/search/pois?q=...&types=...&limit=...
// Searches peaks and amenities using pg_trgm similarity matching.
func (h *Handler) SearchPOIs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "query too short (min 2 chars)", r.URL.Path)
		return
	}

	typesStr := r.URL.Query().Get("types")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	var results []POIResult

	if typesStr == "" || strings.Contains(typesStr, "peak") || strings.Contains(typesStr, "volcano") || strings.Contains(typesStr, "saddle") {
		peaks, err := h.DB.SearchPeaks(r.Context(), q, limit)
		if err == nil {
			results = append(results, poiRowsToResults(peaks)...)
		}
	}

	if typesStr == "" || containsAnyAmenityType(typesStr) {
		amenities, err := h.DB.SearchAmenities(r.Context(), q, typesStr, limit)
		if err == nil {
			results = append(results, poiRowsToResults(amenities)...)
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}
	if results == nil {
		results = []POIResult{}
	}

	writeJSON(w, http.StatusOK, results)
}

// poiRowsToResults converts database POIRows to handler POIResults.
func poiRowsToResults(rows []database.POIRow) []POIResult {
	out := make([]POIResult, len(rows))
	for i, r := range rows {
		out[i] = POIResult{
			Name:   r.Name,
			Type:   r.Type,
			Ele:    r.Ele,
			LatLng: [2]float64{r.Lat, r.Lng},
			OsmID:  r.OsmID,
		}
	}
	return out
}

func containsAnyAmenityType(types string) bool {
	amenityTypes := []string{"alpine_hut", "restaurant", "cafe", "shelter", "viewpoint", "drinking_water"}
	for _, t := range amenityTypes {
		if strings.Contains(types, t) {
			return true
		}
	}
	return false
}
