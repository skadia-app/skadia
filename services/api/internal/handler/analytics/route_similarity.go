package analytics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// SimilarRoute represents a route similar to the target activity.
type SimilarRoute struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	SportType     string    `json:"sport_type"`
	StartedAt     string    `json:"started_at"`
	HausdorffDist float64   `json:"hausdorff_dist"`
}

// GetSimilarRoutes returns routes similar to the given activity using a 2-stage
// PostGIS filter: bounding-box + start/end proximity pre-filter, then Hausdorff distance.
//
// GET /api/v1/analytics/similar-routes?activity_id=...&max_results=10
func (h *Handler) GetSimilarRoutes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	activityIDStr := r.URL.Query().Get("activity_id")
	if activityIDStr == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "activity_id required", r.URL.Path)
		return
	}
	activityID, err := uuid.Parse(activityIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid activity_id", r.URL.Path)
		return
	}

	maxResults := 10
	if mr := r.URL.Query().Get("max_results"); mr != "" {
		if n, err := strconv.Atoi(mr); err == nil && n > 0 && n <= 50 {
			maxResults = n
		}
	}

	const query = `
		WITH target AS (
			SELECT a.id, at2.track_simplified, a.sport_type,
			       ST_StartPoint(at2.track_simplified) AS start_pt,
			       ST_EndPoint(at2.track_simplified)   AS end_pt,
			       ST_Length(at2.track_simplified::geography) AS track_len
			FROM activities a
			JOIN activity_tracks at2 ON at2.activity_id = a.id
			WHERE a.id = $1 AND at2.track_simplified IS NOT NULL
		),
		candidates AS (
			SELECT a.id, a.title, a.sport_type, a.started_at, at2.track_simplified,
			       ST_Length(at2.track_simplified::geography) AS track_len
			FROM activities a
			JOIN activity_tracks at2 ON at2.activity_id = a.id
			CROSS JOIN target t
			WHERE a.id != t.id
			  AND a.user_id = $2
			  AND at2.track_simplified IS NOT NULL
			  AND ST_DWithin(ST_StartPoint(at2.track_simplified)::geography, t.start_pt::geography, 500)
			  AND ST_DWithin(ST_EndPoint(at2.track_simplified)::geography, t.end_pt::geography, 500)
			  AND ST_Length(at2.track_simplified::geography)
			        BETWEEN t.track_len * 0.6 AND t.track_len * 1.4
			LIMIT 50
		)
		SELECT c.id, c.title, c.sport_type, c.started_at,
		       ST_HausdorffDistance(c.track_simplified, t.track_simplified) AS hausdorff_dist
		FROM candidates c
		CROSS JOIN target t
		ORDER BY hausdorff_dist
		LIMIT $3
	`

	rows, err := h.DB.Pool.Query(r.Context(), query, activityID, userID, maxResults)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "query failed", r.URL.Path)
		return
	}
	defer rows.Close()

	results := make([]SimilarRoute, 0)
	for rows.Next() {
		var sr SimilarRoute
		var startedAt time.Time
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.SportType, &startedAt, &sr.HausdorffDist); err != nil {
			continue
		}
		sr.StartedAt = startedAt.Format(time.RFC3339)
		results = append(results, sr)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "row iteration failed", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, results)
}
