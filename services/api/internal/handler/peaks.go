package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// peakVisitResponse is the JSON representation of a visited peak.
type peakVisitResponse struct {
	OsmID      *int64   `json:"osm_id"`
	Name       string   `json:"name"`
	ElevationM *float64 `json:"elevation_m"`
	VisitCount int      `json:"visit_count"`
	FirstVisit string   `json:"first_visit"`
	LastVisit  string   `json:"last_visit"`
	Lat        *float64 `json:"lat"`
	Lng        *float64 `json:"lng"`
}

// GetPeakCollection returns all peaks visited by the authenticated user,
// grouped by osm_id with visit statistics.
func (h *Handler) GetPeakCollection(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	peaks, err := h.DB.GetPeakCollection(r.Context(), userID)
	if err != nil {
		slog.Error("peak collection query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to query peak collection", r.URL.Path)
		return
	}

	resp := make([]peakVisitResponse, 0, len(peaks))
	for _, p := range peaks {
		resp = append(resp, peakVisitResponse{
			OsmID:      p.OsmID,
			Name:       p.Name,
			ElevationM: p.ElevationM,
			VisitCount: p.VisitCount,
			FirstVisit: p.FirstVisit.Format("2006-01-02T15:04:05Z"),
			LastVisit:  p.LastVisit.Format("2006-01-02T15:04:05Z"),
			Lat:        p.Lat,
			Lng:        p.Lng,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
