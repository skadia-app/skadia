package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// PeakVisit represents a single visited peak with aggregated visit data.
type PeakVisit struct {
	OsmID      *int64  `json:"osm_id"`
	Name       string  `json:"name"`
	ElevationM *float64 `json:"elevation_m"`
	VisitCount int     `json:"visit_count"`
	FirstVisit string  `json:"first_visit"`
	LastVisit  string  `json:"last_visit"`
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

	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT
			g.osm_id,
			g.name,
			g.elevation_m,
			COUNT(DISTINCT g.activity_id) AS visit_count,
			MIN(a.started_at) AS first_visit,
			MAX(a.started_at) AS last_visit,
			ST_Y(p.geom) AS lat,
			ST_X(p.geom) AS lng
		FROM activity_geo_tags g
		JOIN activities a ON g.activity_id = a.id
		LEFT JOIN osm_peaks p ON g.osm_id = p.osm_id
		WHERE g.user_id = $1 AND g.category = 'peak'
		GROUP BY g.osm_id, g.name, g.elevation_m, p.geom
		ORDER BY g.elevation_m DESC NULLS LAST
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to query peak collection", r.URL.Path)
		return
	}
	defer rows.Close()

	peaks := make([]PeakVisit, 0)
	for rows.Next() {
		var p PeakVisit
		var firstVisit, lastVisit interface{}
		if err := rows.Scan(&p.OsmID, &p.Name, &p.ElevationM, &p.VisitCount, &firstVisit, &lastVisit, &p.Lat, &p.Lng); err != nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to scan peak row", r.URL.Path)
			return
		}
		if t, ok := firstVisit.(interface{ Format(string) string }); ok {
			p.FirstVisit = t.Format("2006-01-02T15:04:05Z")
		}
		if t, ok := lastVisit.(interface{ Format(string) string }); ok {
			p.LastVisit = t.Format("2006-01-02T15:04:05Z")
		}
		peaks = append(peaks, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to iterate peak rows", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, peaks)
}
