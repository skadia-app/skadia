package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetGeoTags returns geo tags for an activity.
func (h *Handler) GetGeoTags(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	tags, err := h.DB.GetGeoTags(r.Context(), activityID)
	if err != nil {
		slog.Error("get geo tags failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get geo tags", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, tags)
}

// BackfillWeather fetches weather data for all activities missing it.
func (h *Handler) BackfillWeather(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	activities, err := h.DB.ListActivitiesForBackfill(r.Context(), userID)
	if err != nil {
		slog.Error("list activities for backfill failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	var updated int
	for _, a := range activities {
		if a.Lat == 0 || a.Lng == 0 || a.StartedAt.IsZero() {
			continue
		}

		weather, err := enrichment.FetchWeather(a.Lat, a.Lng, a.StartedAt)
		if err != nil {
			slog.Warn("weather fetch failed", "activity_id", a.ID, "error", err)
			continue
		}

		fields := map[string]any{
			"weather_temp_c":       weather.TempC,
			"weather_condition":    weather.Condition,
			"weather_humidity":     weather.Humidity,
			"weather_wind_speed_ms": weather.WindSpeedMS,
			"weather_tags":         weather.Tags,
		}
		if err := h.DB.UpdateEnrichmentFields(r.Context(), a.ID, fields); err != nil {
			slog.Warn("weather update failed", "activity_id", a.ID, "error", err)
			continue
		}
		updated++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(activities),
		"updated": updated,
	})
}

// BackfillEnrichment re-runs stage1 enrichment for all activities.
func (h *Handler) BackfillEnrichment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	activities, err := h.DB.ListActivitiesForBackfill(r.Context(), userID)
	if err != nil {
		slog.Error("list activities for backfill failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	var updated int
	for _, a := range activities {
		if a.Lat == 0 || a.Lng == 0 {
			continue
		}

		data := &model.ActivityData{
			StartLat:     a.Lat,
			StartLng:     a.Lng,
			EndLat:       a.EndLat,
			EndLng:       a.EndLng,
			StartedAt:    a.StartedAt,
			EndedAt:      a.EndedAt,
			SportType:    a.SportType,
			AvgHeartRate: a.AvgHR,
			MaxHeartRate: a.MaxHR,
			HasGPS:       true,
		}

		enrichment.RunStage1(r.Context(), h.DB, a.ID, data)
		updated++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(activities),
		"updated": updated,
	})
}

// GetEnrichmentSettings returns the user's enrichment settings.
func (h *Handler) GetEnrichmentSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		slog.Error("get enrichment settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get settings", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// UpdateEnrichmentSettings updates the user's enrichment settings.
func (h *Handler) UpdateEnrichmentSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var settings model.EnrichmentSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	settings.UserID = userID

	if err := h.DB.UpdateEnrichmentSettings(r.Context(), userID, &settings); err != nil {
		slog.Error("update enrichment settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update settings", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}
