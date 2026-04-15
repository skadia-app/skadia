package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// SetActivityFusion godoc
// POST /api/v1/activities/{id}/fusion
// Body: {"gps": "uuid-A", "hr": "uuid-B", ...}
func (h *Handler) SetActivityFusion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	sources, ok := httputil.DecodeJSON[database.FusionSources](w, r)
	if !ok {
		return
	}

	// Mark all referenced source activities as fusion sources so they are
	// hidden from the normal activity list.
	for _, srcID := range sources {
		if srcID != activityID {
			if err := h.DB.MarkAsFusionSource(r.Context(), srcID); err != nil {
				slog.Warn("mark fusion source failed", "src_id", srcID, "error", err)
			}
		}
	}

	if err := h.DB.SetFusionSources(r.Context(), activityID, userID, sources); err != nil {
		slog.Error("set fusion sources", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set fusion sources", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// FusionCandidate beschreibt eine Aktivität die als Fusionsquelle in Frage kommt.
type FusionCandidate struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	StartedAt          time.Time `json:"started_at"`
	DistanceM          float64   `json:"distance_m"`
	SportType          string    `json:"sport_type"`
	HasHR              bool      `json:"has_hr"`
	HasPower           bool      `json:"has_power"`
	HasGPS             bool      `json:"has_gps"`
	HasTemperature     bool      `json:"has_temperature"`
	HasCadence         bool      `json:"has_cadence"`
	HasAltitude        bool      `json:"has_altitude"`
	OverlapPct         float64   `json:"overlap_pct"`
	Timezone           *string   `json:"timezone,omitempty"`
	SourceDevice       *string   `json:"source_device,omitempty"`
	Source             *string   `json:"source,omitempty"`
	OriginalFileFormat *string   `json:"original_file_format,omitempty"`
}

// GetFusionCandidates godoc
// GET /api/v1/activities/{id}/fusion-candidates
// Returns activities that are temporally close and share the same sport type,
// making them potential candidates for stream fusion.
// Includes stream availability flags and temporal overlap percentage.
func (h *Handler) GetFusionCandidates(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	settings, err := h.DB.GetImportSettings(r.Context(), userID)
	timeWindowMin := 30
	if err == nil && settings != nil && settings.FusionTimeWindowMin > 0 {
		timeWindowMin = settings.FusionTimeWindowMin
	}

	// Ziel-Aktivität laden um started_at + ended_at für Overlap-Berechnung zu bekommen.
	target, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || target == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT
			b.id,
			COALESCE(b.title, b.title_auto, '') AS title,
			b.started_at,
			COALESCE(b.distance_m, 0) AS distance_m,
			b.sport_type,
			(b.avg_heart_rate IS NOT NULL)    AS has_hr,
			(b.avg_power_w IS NOT NULL)       AS has_power,
			b.has_gps,
			(b.avg_temperature_c IS NOT NULL) AS has_temperature,
			(b.avg_cadence IS NOT NULL)       AS has_cadence,
			(b.elevation_gain_m IS NOT NULL)  AS has_altitude,
			CASE WHEN b.ended_at IS NOT NULL THEN
				GREATEST(0.0,
					EXTRACT(EPOCH FROM (
						LEAST(b.ended_at, $3::timestamptz) -
						GREATEST(b.started_at, $2::timestamptz)
					))
				) / GREATEST(1.0, EXTRACT(EPOCH FROM (b.ended_at - b.started_at)))
			ELSE 0.0 END AS overlap_pct,
			b.timezone,
			b.source_device,
			b.source,
			b.original_file_format
		FROM activities a
		JOIN activities b
			ON b.user_id = a.user_id
			AND b.id != a.id
			AND ABS(EXTRACT(EPOCH FROM (b.started_at - a.started_at))) <= $4
			AND NOT EXISTS (SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = b.id)
			AND b.is_group_secondary = FALSE
		WHERE a.id = $1 AND a.user_id = $5
		ORDER BY ABS(EXTRACT(EPOCH FROM (b.started_at - a.started_at)))
		LIMIT 10`,
		activityID,
		target.StartedAt,
		func() any {
			if target.EndedAt != nil {
				return *target.EndedAt
			}
			return target.StartedAt
		}(),
		timeWindowMin*60,
		userID,
	)
	if err != nil {
		slog.Error("find fusion candidates", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to find fusion candidates", r.URL.Path)
		return
	}
	defer rows.Close()

	result := make([]FusionCandidate, 0)
	for rows.Next() {
		var c FusionCandidate
		if err := rows.Scan(
			&c.ID,
			&c.Title,
			&c.StartedAt,
			&c.DistanceM,
			&c.SportType,
			&c.HasHR,
			&c.HasPower,
			&c.HasGPS,
			&c.HasTemperature,
			&c.HasCadence,
			&c.HasAltitude,
			&c.OverlapPct,
			&c.Timezone,
			&c.SourceDevice,
			&c.Source,
			&c.OriginalFileFormat,
		); err != nil {
			slog.Warn("scan fusion candidate failed", "error", err)
			continue
		}
		result = append(result, c)
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
