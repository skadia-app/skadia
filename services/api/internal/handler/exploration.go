package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetExplorationCoverage returns a GeoJSON polygon of explored area
// (union of buffered heatmap lines) and the total explored area in km².
func (h *Handler) GetExplorationCoverage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Optional bbox filter: ?bbox=west,south,east,north
	var bbox *[4]float64
	if bboxStr := r.URL.Query().Get("bbox"); bboxStr != "" {
		parts := strings.Split(bboxStr, ",")
		if len(parts) == 4 {
			var coords [4]float64
			valid := true
			for i, p := range parts {
				v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					valid = false
					break
				}
				coords[i] = v
			}
			if valid {
				bbox = &coords
			}
		}
	}

	geojson, areaKm2, err := h.DB.GetExplorationCoverage(r.Context(), userID, bbox)
	if err != nil {
		slog.Error("exploration coverage query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to compute exploration coverage", r.URL.Path)
		return
	}

	// Return raw GeoJSON string to avoid double-encoding
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"explored_area":%s,"area_km2":%.2f}`, geojson, areaKm2)
}
