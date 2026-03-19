package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// CorrectActivityDistance recalculates the activity's distance from its GPS track
// using the Haversine formula over all stored stream points. Updates distance_m in
// the activities table. Runs as a background job.
//
// POST /api/v1/activities/{id}/correct-distance
//
// Response: 202 Accepted  { "job_id": "<uuid>", "status": "started" }
func (h *Handler) CorrectActivityDistance(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if !activity.HasGPS {
		writeError(w, http.StatusUnprocessableEntity, "No GPS", "activity has no GPS track", r.URL.Path)
		return
	}

	activityName := activity.Title
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "distance_correction", &activityID, activityName, 1)
	if err != nil {
		slog.Error("correct-distance: failed to insert background job", "activity_id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "could not create background job", r.URL.Path)
		return
	}

	go h.runDistanceCorrectionJob(jobID, activityID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"job_id": jobID,
		"status": "started",
	})
}

func (h *Handler) runDistanceCorrectionJob(jobID, activityID string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("distance correction job panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		}
	}()

	slog.Info("distance correction: starting", "job_id", jobID, "activity_id", activityID)

	// Load GPS stream points.
	gpsPoints, err := h.DB.GetActivityGPSStream(ctx, activityID)
	if err != nil {
		slog.Error("distance correction: failed to load GPS stream", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}
	if len(gpsPoints) < 2 {
		slog.Warn("distance correction: insufficient GPS points", "activity_id", activityID, "points", len(gpsPoints))
		h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
		return
	}

	// Calculate total distance via Haversine.
	var totalM float64
	for i := 1; i < len(gpsPoints); i++ {
		totalM += haversineDistanceM(
			gpsPoints[i-1].Lat, gpsPoints[i-1].Lng,
			gpsPoints[i].Lat, gpsPoints[i].Lng,
		)
	}

	// Update distance_m and distance_source on the activity.
	if err := h.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"distance_m": totalM, "distance_source": "computed"}); err != nil {
		slog.Error("distance correction: failed to update distance_m", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}

	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)
	slog.Info("distance correction: complete", "job_id", jobID, "activity_id", activityID,
		"distance_m", totalM)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// ResetActivityDistance restores the original parsed distance by re-parsing the
// source file (streams + metrics scope). This undoes any distance correction and
// reverts to the device-recorded or parser-calculated distance.
//
// POST /api/v1/activities/{id}/reset-distance
//
// Response: 202 Accepted  { "job_id": "<uuid>", "status": "started" }
func (h *Handler) ResetActivityDistance(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	activityName := activity.Title
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "distance_reset", &activityID, activityName, 1)
	if err != nil {
		slog.Error("reset-distance: failed to insert background job", "activity_id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "could not create background job", r.URL.Path)
		return
	}

	go h.runDistanceResetJob(jobID, activityID, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"job_id": jobID,
		"status": "started",
	})
}

func (h *Handler) runDistanceResetJob(jobID, activityID, userID string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("distance reset job panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		}
	}()

	slog.Info("distance reset: starting", "job_id", jobID, "activity_id", activityID)

	// Re-parse original file (streams + metrics scope) to restore device-recorded distance.
	scope := map[string]bool{"streams": true, "metrics": true}
	if err := h.reprocessActivity(ctx, activityID, userID, scope, 0); err != nil {
		slog.Error("distance reset: reprocess failed", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}

	// Reset distance_source to device after restoring original distance.
	if err := h.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"distance_source": "device"}); err != nil {
		slog.Warn("distance reset: failed to update distance_source", "error", err, "activity_id", activityID)
	}

	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)
	slog.Info("distance reset: complete", "job_id", jobID, "activity_id", activityID)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// haversineDistanceM returns the great-circle distance in metres between two
// WGS-84 coordinates using the Haversine formula.
func haversineDistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
