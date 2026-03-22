package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// summitVisitResponse is the JSON representation of a visited summit.
type summitVisitResponse struct {
	OsmID        *int64   `json:"osm_id"`
	Name         string   `json:"name"`
	ElevationM   *float64 `json:"elevation_m"`
	VisitCount   int      `json:"visit_count"`
	FirstVisit   string   `json:"first_visit"`
	LastVisit    string   `json:"last_visit"`
	Lat          *float64 `json:"lat"`
	Lng          *float64 `json:"lng"`
	CountryCodes []string `json:"country_codes"`
}

// GetSummitCollection returns all summits visited by the authenticated user,
// grouped by osm_id with visit statistics.
// Optional query parameters: country (comma-separated ISO codes), min_elevation, max_elevation, year.
func (h *Handler) GetSummitCollection(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	q := r.URL.Query()

	var f database.SummitCollectionFilter

	if country := q.Get("country"); country != "" {
		for c := range strings.SplitSeq(country, ",") {
			if c = strings.TrimSpace(c); c != "" {
				f.Countries = append(f.Countries, c)
			}
		}
	}
	if minElev := q.Get("min_elevation"); minElev != "" {
		if v, err := strconv.ParseFloat(minElev, 64); err == nil {
			f.MinElevM = &v
		}
	}
	if maxElev := q.Get("max_elevation"); maxElev != "" {
		if v, err := strconv.ParseFloat(maxElev, 64); err == nil {
			f.MaxElevM = &v
		}
	}
	if yearStr := q.Get("year"); yearStr != "" {
		if v, err := strconv.Atoi(yearStr); err == nil {
			f.Year = &v
		}
	}

	summits, err := h.DB.GetSummitCollection(r.Context(), userID, f)
	if err != nil {
		slog.Error("summit collection query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to query summit collection", r.URL.Path)
		return
	}

	resp := make([]summitVisitResponse, 0, len(summits))
	for _, s := range summits {
		codes := s.CountryCodes
		if codes == nil {
			codes = []string{}
		}
		resp = append(resp, summitVisitResponse{
			OsmID:        s.OsmID,
			Name:         s.Name,
			ElevationM:   s.ElevationM,
			VisitCount:   s.VisitCount,
			FirstVisit:   s.FirstVisit.Format("2006-01-02T15:04:05Z"),
			LastVisit:    s.LastVisit.Format("2006-01-02T15:04:05Z"),
			Lat:          s.Lat,
			Lng:          s.Lng,
			CountryCodes: codes,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

type summitActivityResponse struct {
	ActivityID     string   `json:"activity_id"`
	Title          string   `json:"title"`
	StartedAt      string   `json:"started_at"`
	SportType      string   `json:"sport_type"`
	DistanceM      *float64 `json:"distance_m"`
	ElevationGainM *float64 `json:"elevation_gain_m"`
}

// GetSummitActivities returns all activities linked to a summit.
// URL param: osm_id (int) — falls back to query param "name" when osm_id is 0.
func (h *Handler) GetSummitActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	// WICHTIG: Chi-Router, NICHT r.PathValue()!
	osmIDStr := chi.URLParam(r, "osm_id")
	name := r.URL.Query().Get("name")

	var osmID *int64
	if osmIDStr != "" && osmIDStr != "0" {
		if v, err := strconv.ParseInt(osmIDStr, 10, 64); err == nil {
			osmID = &v
		}
	}

	acts, err := h.DB.GetSummitActivities(r.Context(), userID, osmID, name)
	if err != nil {
		slog.Error("summit activities query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to query summit activities", r.URL.Path)
		return
	}

	resp := make([]summitActivityResponse, 0, len(acts))
	for _, a := range acts {
		resp = append(resp, summitActivityResponse{
			ActivityID:     a.ActivityID,
			Title:          a.Title,
			StartedAt:      a.StartedAt.Format("2006-01-02T15:04:05Z"),
			SportType:      a.SportType,
			DistanceM:      a.DistanceM,
			ElevationGainM: a.ElevationGainM,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
