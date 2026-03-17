package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetGeoTags returns geo tags for an activity.
func (h *Handler) GetGeoTags(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get geo tags: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
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

// BackfillWeather fetches weather data for all activities missing it (background job).
func (h *Handler) BackfillWeather(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "weather_backfill", nil, nil, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "started"})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := h.runWeatherBackfill(ctx, userID, jobID); err != nil {
			slog.Warn("weather backfill failed", "job_id", jobID, "error", err)
			_ = h.DB.CompleteBackgroundJob(ctx, jobID, true)
		}
	}()
}

func (h *Handler) runWeatherBackfill(ctx context.Context, userID, jobID string) error {
	prefs, err := h.DB.GetWeatherPrefs(ctx, userID)
	if err != nil {
		prefs = model.DefaultWeatherPrefs()
	}

	candidates, err := h.DB.GetActivitiesForWeatherBackfill(ctx, userID)
	if err != nil {
		return fmt.Errorf("get candidates: %w", err)
	}

	_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)
	_ = h.DB.ResetBackgroundJobProgress(ctx, jobID, int64(len(candidates)))
	var succeeded, failed int

	for i, a := range candidates {
		if ctx.Err() != nil {
			break
		}
		reqs := h.buildWeatherRequestsForCandidate(ctx, a, prefs)
		if len(reqs) == 0 {
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1))
			continue
		}
		readings, fetchErr := enrichment.FetchWeatherReadings(ctx, reqs)
		if fetchErr != nil {
			slog.Warn("weather backfill fetch failed", "activity_id", a.ID, "error", fetchErr)
			failed++
		} else if insertErr := h.DB.InsertWeatherReadings(ctx, a.ID, readings); insertErr != nil {
			slog.Warn("weather backfill insert failed", "activity_id", a.ID, "error", insertErr)
			failed++
		} else {
			// Sync weather tags to activity_tags junction table
			if prefs.AutoTags {
				var allWeatherTags []string
				for _, r := range readings {
					allWeatherTags = append(allWeatherTags, r.Tags...)
				}
				if syncErr := h.DB.SyncWeatherTagsToActivityTags(ctx, a.ID, userID, allWeatherTags); syncErr != nil {
					slog.Warn("weather backfill: sync weather tags failed", "activity_id", a.ID, "error", syncErr)
				}
			}
			succeeded++
		}
		_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1))
	}

	slog.Info("weather backfill complete", "job_id", jobID, "succeeded", succeeded, "failed", failed)
	return h.DB.CompleteBackgroundJob(ctx, jobID, false)
}

func (h *Handler) buildWeatherRequestsForCandidate(
	ctx context.Context,
	a database.ActivityWeatherCandidate,
	prefs model.WeatherPrefs,
) []enrichment.WeatherRequest {
	if !a.HasGPS {
		return nil
	}

	var elapsed int
	if a.ElapsedTimeS != nil {
		elapsed = *a.ElapsedTimeS
	}

	var minAlt, maxAlt float64
	if a.MinElevationM != nil {
		minAlt = *a.MinElevationM
	}
	if a.MaxElevationM != nil {
		maxAlt = *a.MaxElevationM
	}

	c := enrichment.DetermineWeatherCase(elapsed, minAlt, maxAlt, prefs)

	var summitLat, summitLng *float64
	var summitAlt float64
	var summitAt *time.Time
	var midLat, midLng, midMeanAlt float64
	var midAt *time.Time

	if c == enrichment.CaseMountain {
		if slat, slng, salt, sat, ok := enrichment.SummitPoint(ctx, h.DB.Pool, a.ID); ok {
			summitLat, summitLng = &slat, &slng
			summitAlt = salt
			summitAt = &sat
		}
	}
	if c == enrichment.CaseMountain || c == enrichment.CaseLong {
		if mlat, mlng, malt, mat, ok := enrichment.MidpointGPS(ctx, h.DB.Pool, a.ID); ok {
			midLat, midLng, midMeanAlt = mlat, mlng, malt
			midAt = &mat
		}
	}

	return enrichment.BuildWeatherRequests(
		a.StartLat, a.StartLng, a.StartedAt,
		elapsed,
		a.MinElevationM, a.MaxElevationM,
		summitLat, summitLng, summitAlt,
		midLat, midLng, midMeanAlt,
		summitAt, midAt,
		prefs,
	)
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

	var processed int
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

		enrichment.RunStage1(r.Context(), h.DB, userID, a.ID, data, h.Events)
		processed++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"processed": processed,
		"errors":    0,
	})
}

// BackfillSunriseSunset calculates sunrise/sunset times for all activities.
// For events that occur during an activity, it queries the closest GPS stream
// point to refine the calculation based on the track position at that time.
func (h *Handler) BackfillSunriseSunset(w http.ResponseWriter, r *http.Request) {
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
		if a.Lat == 0 || a.Lng == 0 || a.StartedAt.IsZero() || a.EndedAt.IsZero() {
			continue
		}

		// Compute preliminary sun times at start point
		sun := enrichment.CalculateSunTimes(
			a.Lat, a.Lng, a.EndLat, a.EndLng,
			a.StartedAt, a.EndedAt,
			nil, // no in-memory streams — use DB lookups below
		)

		// Refine: if sunrise/sunset is during the activity, query the closest
		// GPS stream point from the DB instead of loading all streams into memory
		if sun.SunriseAt != nil && sun.SunriseAt.After(a.StartedAt) && sun.SunriseAt.Before(a.EndedAt) {
			if sp, err := h.DB.GetStreamPointNearTime(r.Context(), a.ID, *sun.SunriseAt); err == nil && sp != nil && sp.Lat != nil {
				refined := enrichment.CalculateSunTimes(
					*sp.Lat, *sp.Lng, a.EndLat, a.EndLng,
					a.StartedAt, a.EndedAt, nil,
				)
				if refined.SunriseAt != nil {
					sun.SunriseAt = refined.SunriseAt
				}
			}
		}
		if sun.SunsetAt != nil && sun.SunsetAt.After(a.StartedAt) && sun.SunsetAt.Before(a.EndedAt) {
			if sp, err := h.DB.GetStreamPointNearTime(r.Context(), a.ID, *sun.SunsetAt); err == nil && sp != nil && sp.Lat != nil {
				refined := enrichment.CalculateSunTimes(
					a.Lat, a.Lng, *sp.Lat, *sp.Lng,
					a.StartedAt, a.EndedAt, nil,
				)
				if refined.SunsetAt != nil {
					sun.SunsetAt = refined.SunsetAt
				}
			}
		}

		fields := map[string]any{}
		if sun.SunriseAt != nil {
			fields["sunrise_at"] = *sun.SunriseAt
		}
		if sun.SunsetAt != nil {
			fields["sunset_at"] = *sun.SunsetAt
		}
		if len(fields) == 0 {
			continue
		}

		if err := h.DB.UpdateEnrichmentFields(r.Context(), a.ID, fields); err != nil {
			slog.Warn("sunrise/sunset update failed", "activity_id", a.ID, "error", err)
			continue
		}
		updated++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(activities),
		"updated": updated,
	})
}

// GetActivityClimbs returns detected climbs for an activity.
func (h *Handler) GetActivityClimbs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity climbs: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	climbs, err := h.DB.GetActivityClimbs(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity climbs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity climbs", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, climbs)
}

// GetActivityStops returns detected stops for an activity.
func (h *Handler) GetActivityStops(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity stops: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	stops, err := h.DB.GetActivityStops(r.Context(), activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get stops", r.URL.Path)
		return
	}
	if stops == nil {
		stops = []model.ActivityStop{}
	}
	writeJSON(w, http.StatusOK, stops)
}

// GetActivitySurfaces returns detected surfaces for an activity.
func (h *Handler) GetActivitySurfaces(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity surfaces: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	surfaces, err := h.DB.GetActivitySurfaces(r.Context(), activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get surfaces", r.URL.Path)
		return
	}
	if surfaces == nil {
		surfaces = []model.ActivitySurface{}
	}
	writeJSON(w, http.StatusOK, surfaces)
}

// GetEnrichmentStatus returns per-module enrichment status for an activity.
func (h *Handler) GetEnrichmentStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get enrichment status: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	modules, err := h.DB.GetEnrichmentModuleStatus(r.Context(), activityID)
	if err != nil {
		slog.Error("get enrichment status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get enrichment status", r.URL.Path)
		return
	}

	// If enrichment_status is pending/processing, mark all no_data modules as pending
	if activity.EnrichmentStatus == "pending" || activity.EnrichmentStatus == "processing" {
		for i := range modules {
			if modules[i].Status == "no_data" {
				modules[i].Status = "pending"
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enrichment_status": activity.EnrichmentStatus,
		"modules":           modules,
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

	// Decode the incoming JSON into a raw map so we can distinguish an
	// explicit false from a missing field (Go bool zero-value problem).
	var rawPatch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawPatch); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	// Load existing settings from DB (upserts defaults if not yet present).
	existing, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		slog.Error("update enrichment settings: load existing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load settings", r.URL.Path)
		return
	}

	// Re-encode existing settings to JSON, then merge the patch on top so
	// only the keys present in the request body overwrite existing values.
	// This prevents a partial send from zeroing out unmentioned bool fields.
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "marshal failed", r.URL.Path)
		return
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(existingJSON, &merged); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "unmarshal failed", r.URL.Path)
		return
	}
	for k, v := range rawPatch {
		merged[k] = v
	}
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "merge failed", r.URL.Path)
		return
	}

	var settings model.EnrichmentSettings
	if err := json.Unmarshal(mergedJSON, &settings); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid merged JSON", r.URL.Path)
		return
	}
	settings.UserID = userID

	// Guard constrained fields against zero-values from partial updates.
	if settings.VideoClipFrames < 1 {
		settings.VideoClipFrames = 4
	}
	if settings.PhotoPreviewSize == "" {
		settings.PhotoPreviewSize = "wqhd"
	}
	if settings.PhashThresholdBits < 1 || settings.PhashThresholdBits > 10 {
		settings.PhashThresholdBits = 3
	}

	if err := h.DB.UpdateEnrichmentSettings(r.Context(), userID, &settings); err != nil {
		slog.Error("update enrichment settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update settings", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}
