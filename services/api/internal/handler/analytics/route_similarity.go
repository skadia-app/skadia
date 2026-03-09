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
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	SportType  string    `json:"sport_type"`
	StartedAt  string    `json:"started_at"`
	OverlapPct float64   `json:"overlap_pct"`
}

// GetSimilarRoutes returns routes similar to the given activity using a 2-stage
// PostGIS filter: bounding-box + start/end proximity pre-filter, then overlap percentage.
//
// The overlap metric measures what percentage of each candidate track lies within
// a 50m corridor of the target track (and vice versa), taking the better of the
// two directions. This is far more meaningful than Hausdorff distance, which is
// dominated by single worst-case deviations (e.g. a short detour).
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

	// Stage 1: Pre-filter by start/end proximity and length ratio.
	// Stage 2: Compute bidirectional overlap percentage using ST_Buffer + ST_Intersection
	//          in EPSG:3857 (meters). A 50m corridor around each track is used.
	//          GREATEST(A→B, B→A) picks the better direction (handles out-and-back etc.).
	//          Only candidates with ≥30% overlap pass.
	const query = `
		WITH target AS (
			SELECT a.id,
			       ST_Transform(at2.track_simplified, 3857) AS track_m,
			       ST_StartPoint(at2.track_simplified) AS start_pt,
			       ST_EndPoint(at2.track_simplified)   AS end_pt,
			       ST_Length(at2.track_simplified::geography) AS track_len
			FROM activities a
			JOIN activity_tracks at2 ON at2.activity_id = a.id
			WHERE a.id = $1 AND at2.track_simplified IS NOT NULL
		),
		candidates AS (
			SELECT a.id, a.title, a.sport_type, a.started_at,
			       ST_Transform(at2.track_simplified, 3857) AS track_m
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
		),
		scored AS (
			SELECT c.id, c.title, c.sport_type, c.started_at,
			       GREATEST(
			           ST_Length(ST_Intersection(c.track_m, ST_Buffer(t.track_m, 50)))
			               / NULLIF(ST_Length(c.track_m), 0),
			           ST_Length(ST_Intersection(t.track_m, ST_Buffer(c.track_m, 50)))
			               / NULLIF(ST_Length(t.track_m), 0)
			       ) * 100.0 AS overlap_pct
			FROM candidates c
			CROSS JOIN target t
		)
		SELECT id, title, sport_type, started_at, overlap_pct
		FROM scored
		WHERE overlap_pct >= 30
		ORDER BY overlap_pct DESC
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
		if err := rows.Scan(&sr.ID, &sr.Title, &sr.SportType, &startedAt, &sr.OverlapPct); err != nil {
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
