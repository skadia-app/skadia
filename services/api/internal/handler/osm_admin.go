package handler

import (
	"net/http"
)

// GetOSMStatus returns counts from OSM tables.
// GET /api/v1/admin/osm-status
func (h *Handler) GetOSMStatus(w http.ResponseWriter, r *http.Request) {
	var peaks, passes, lifts, settlements, amenities int64
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_pois WHERE category = 'peak'").Scan(&peaks)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_pois WHERE category = 'pass'").Scan(&passes)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_lifts").Scan(&lifts)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_pois WHERE category = 'settlement'").Scan(&settlements)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_pois WHERE category NOT IN ('peak', 'pass', 'settlement')").Scan(&amenities)
	writeJSON(w, http.StatusOK, map[string]any{
		"peaks": peaks, "passes": passes, "lifts": lifts, "settlements": settlements, "amenities": amenities,
		"has_data": peaks > 0 || lifts > 0 || amenities > 0,
	})
}
