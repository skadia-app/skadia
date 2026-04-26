package handler

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type activityTrackSummaryResponse struct {
	ForestPct       *float64                         `json:"forest_pct"`
	ProtectedPct    *float64                         `json:"protected_pct"`
	ForestRanges    []edgetags.CoordRange            `json:"forest_ranges"`
	ProtectedRanges []edgetags.CoordRange            `json:"protected_ranges"`
	SacRanges       map[string][]edgetags.CoordRange `json:"sac_ranges"`
	MtbRanges       map[string][]edgetags.CoordRange `json:"mtb_ranges"`
	VfRanges        map[string][]edgetags.CoordRange `json:"vf_ranges"`
	VfUnknownRanges []edgetags.CoordRange            `json:"vf_unknown_ranges"`
	Surfaces        []surfaceItem                    `json:"surfaces"`
	Highways        []surfaceItem                    `json:"highways"`
	Summits         []geoTagItem                     `json:"summits"`
	Waters          []waterItem                      `json:"waters"`
	Settlements     []geoTagItem                     `json:"settlements"`
	Settings        trackSummarySettingsSnapshot     `json:"settings"`
}

type trackSummarySettingsSnapshot struct {
	EnableDifficultyScales bool `json:"enable_difficulty_scales"`
}

type surfaceItem struct {
	Type       string  `json:"type"`
	Percentage float64 `json:"percentage"`
	LengthM    float64 `json:"length_m"`
}

type geoTagItem struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Name     string   `json:"name"`
	OSMID    *int64   `json:"osm_id"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	EleM     *float64 `json:"ele_m"`
	DistM    *float64 `json:"dist_m"`
	TrackPos *float64 `json:"track_pos"`
}

type waterItem struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	OSMID    *int64   `json:"osm_id"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	EleM     *float64 `json:"ele_m"`
	DistM    *float64 `json:"dist_m"`
	TrackPos *float64 `json:"track_pos"`
}

type trackSummaryEnrichmentRow struct {
	Category   string
	Type       string
	Percentage float64
	LengthM    *float64
	Ranges     []edgetags.CoordRange
}

type trackSummaryGeoTagRow struct {
	ID       string
	Category string
	Name     string
	OSMID    *int64
	Lat      *float64
	Lng      *float64
	EleM     *float64
	DistM    *float64
	TrackPos *float64
}

// ActivityTrackSummary returns the consolidated per-track enrichment summary for one activity.
func (h *Handler) ActivityTrackSummary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	var ownerID string
	if err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT user_id::text FROM activities WHERE id = $1`,
		activityID,
	).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("activity track summary: owner lookup failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if ownerID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "activity belongs to another user", r.URL.Path)
		return
	}

	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		slog.Error("activity track summary: settings lookup failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load enrichment settings", r.URL.Path)
		return
	}

	enrichmentRows, err := h.readTrackSummaryEnrichments(r, activityID)
	if err != nil {
		slog.Error("activity track summary: enrichment lookup failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity enrichments", r.URL.Path)
		return
	}
	geoTagRows, err := h.readTrackSummaryGeoTags(r, activityID)
	if err != nil {
		slog.Error("activity track summary: geo tag lookup failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity geo tags", r.URL.Path)
		return
	}

	summary, err := buildTrackSummary(settings, enrichmentRows, geoTagRows)
	if err != nil {
		slog.Error("activity track summary: build failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to build track summary", r.URL.Path)
		return
	}

	body, err := json.Marshal(summary)
	if err != nil {
		slog.Error("activity track summary: marshal failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to encode track summary", r.URL.Path)
		return
	}
	sum := sha1.Sum(body)
	etag := fmt.Sprintf(`"%x"`, sum[:])

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		slog.Error("activity track summary: write failed", "activity_id", activityID, "error", err)
	}
}

func (h *Handler) readTrackSummaryEnrichments(r *http.Request, activityID string) ([]trackSummaryEnrichmentRow, error) {
	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT category, type, percentage, length_m, ranges
		FROM activity_enrichments
		WHERE activity_id = $1
		ORDER BY category, type
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []trackSummaryEnrichmentRow
	for rows.Next() {
		var row trackSummaryEnrichmentRow
		var rangesJSON []byte
		if err := rows.Scan(&row.Category, &row.Type, &row.Percentage, &row.LengthM, &rangesJSON); err != nil {
			return nil, err
		}
		ranges, err := decodeTrackSummaryRanges(rangesJSON)
		if err != nil {
			return nil, fmt.Errorf("decode ranges for %s/%s: %w", row.Category, row.Type, err)
		}
		row.Ranges = ranges
		out = append(out, row)
	}
	return out, rows.Err()
}

func (h *Handler) readTrackSummaryGeoTags(r *http.Request, activityID string) ([]trackSummaryGeoTagRow, error) {
	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT DISTINCT ON (g.osm_id, g.category)
			g.id::text,
			g.category,
			g.name,
			g.osm_id,
			g.lat,
			g.lng,
			COALESCE(g.ele_m, g.ele) AS ele_m,
			g.dist_m,
			CASE
				WHEN g.lat IS NOT NULL AND g.lng IS NOT NULL AND t.track_full IS NOT NULL
				THEN ST_LineLocatePoint(t.track_full, ST_SetSRID(ST_MakePoint(g.lng, g.lat), 4326))
				ELSE NULL
			END AS track_pos
		FROM activity_geo_tags g
		JOIN activity_tracks t ON t.activity_id = g.activity_id
		WHERE g.activity_id = $1
		  AND g.category IN ('summit', 'pass', 'still_water', 'flowing_water', 'settlement')
		ORDER BY g.osm_id, g.category, g.dist_m NULLS LAST
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []trackSummaryGeoTagRow
	for rows.Next() {
		var row trackSummaryGeoTagRow
		if err := rows.Scan(
			&row.ID,
			&row.Category,
			&row.Name,
			&row.OSMID,
			&row.Lat,
			&row.Lng,
			&row.EleM,
			&row.DistM,
			&row.TrackPos,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func decodeTrackSummaryRanges(raw []byte) ([]edgetags.CoordRange, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var ranges []edgetags.CoordRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil, err
	}
	return ranges, nil
}

func buildTrackSummary(
	settings *model.EnrichmentSettings,
	enrichments []trackSummaryEnrichmentRow,
	geoTags []trackSummaryGeoTagRow,
) (*activityTrackSummaryResponse, error) {
	out := &activityTrackSummaryResponse{
		Settings: trackSummarySettingsSnapshot{
			EnableDifficultyScales: settings != nil && settings.EnableDifficultyScales,
		},
	}

	for _, row := range enrichments {
		switch row.Category {
		case "forest_pct":
			out.ForestPct = &row.Percentage
			out.ForestRanges = row.Ranges
		case "protected_pct":
			out.ProtectedPct = &row.Percentage
			out.ProtectedRanges = row.Ranges
		case "surface":
			out.Surfaces = append(out.Surfaces, surfaceItem{
				Type:       row.Type,
				Percentage: row.Percentage,
				LengthM:    derefFloat64(row.LengthM),
			})
		case "highway":
			out.Highways = append(out.Highways, surfaceItem{
				Type:       row.Type,
				Percentage: row.Percentage,
				LengthM:    derefFloat64(row.LengthM),
			})
		case "sac_scale":
			if out.SacRanges == nil {
				out.SacRanges = make(map[string][]edgetags.CoordRange)
			}
			out.SacRanges[row.Type] = row.Ranges
		case "mtb_scale":
			if out.MtbRanges == nil {
				out.MtbRanges = make(map[string][]edgetags.CoordRange)
			}
			out.MtbRanges[row.Type] = row.Ranges
		case "vf_scale":
			if out.VfRanges == nil {
				out.VfRanges = make(map[string][]edgetags.CoordRange)
			}
			out.VfRanges[row.Type] = row.Ranges
		case "vf_unknown":
			out.VfUnknownRanges = row.Ranges
		}
	}

	for _, row := range geoTags {
		switch row.Category {
		case "summit", "pass":
			out.Summits = append(out.Summits, geoTagItemFromRow(row))
		case "still_water":
			out.Waters = append(out.Waters, waterItemFromRow(row, "still"))
		case "flowing_water":
			out.Waters = append(out.Waters, waterItemFromRow(row, "flowing"))
		case "settlement":
			out.Settlements = append(out.Settlements, geoTagItemFromRow(row))
		}
	}

	return out, nil
}

func geoTagItemFromRow(row trackSummaryGeoTagRow) geoTagItem {
	return geoTagItem{
		ID:       row.ID,
		Category: row.Category,
		Name:     row.Name,
		OSMID:    row.OSMID,
		Lat:      row.Lat,
		Lng:      row.Lng,
		EleM:     row.EleM,
		DistM:    row.DistM,
		TrackPos: row.TrackPos,
	}
}

func waterItemFromRow(row trackSummaryGeoTagRow, kind string) waterItem {
	return waterItem{
		ID:       row.ID,
		Category: row.Category,
		Kind:     kind,
		Name:     row.Name,
		OSMID:    row.OSMID,
		Lat:      row.Lat,
		Lng:      row.Lng,
		EleM:     row.EleM,
		DistM:    row.DistM,
		TrackPos: row.TrackPos,
	}
}
