package handler

import (
	"net/http"
)

// GetOSMStatus returns counts from OSM tables.
// GET /api/v1/admin/osm-status
func (h *Handler) GetOSMStatus(w http.ResponseWriter, r *http.Request) {
	var peaks, lifts, settlements int64
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_peaks").Scan(&peaks)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_lifts").Scan(&lifts)
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM osm_settlements").Scan(&settlements)
	writeJSON(w, http.StatusOK, map[string]any{
		"peaks": peaks, "lifts": lifts, "settlements": settlements,
		"has_data": peaks > 0 || lifts > 0,
	})
}
