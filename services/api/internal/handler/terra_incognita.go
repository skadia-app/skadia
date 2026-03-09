package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetTerraIncognitaCoverage returns 5 concentric GeoJSON ring polygons
// for gradient fog-of-war rendering and the total explored area in km².
func (h *Handler) GetTerraIncognitaCoverage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

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

	// Load terra incognita radius from user settings (default 1000m).
	radiusM := 1000
	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err == nil && settings.TerraIncognitaRadiusM > 0 {
		radiusM = settings.TerraIncognitaRadiusM
	}

	rings, areaKm2, err := h.DB.GetTerraIncognitaCoverage(r.Context(), userID, bbox, radiusM)
	if err != nil {
		slog.Error("terra incognita coverage query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to compute exploration coverage", r.URL.Path)
		return
	}

	// Return raw GeoJSON ring strings to avoid double-encoding.
	// rings[0]=innermost (-0.3R), rings[4]=outermost (+0.6R)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"rings":[%s,%s,%s,%s,%s],"area_km2":%.2f}`,
		rings[0], rings[1], rings[2], rings[3], rings[4], areaKm2)
}
