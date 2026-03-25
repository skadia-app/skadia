package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/editor"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// editorBucket returns the S3 bucket used for edit sidecars.
func editorBucket() string {
	b := os.Getenv("GARAGE_MEDIA_BUCKET")
	if b == "" {
		return "skadia-media"
	}
	return b
}

// editorKey returns the S3 key for a user's activity edit sidecar.
func editorKey(userID, activityID string) string {
	return fmt.Sprintf("edits/%s/%s.edits.json", userID, activityID)
}

// GetActivityEdits loads the edit sidecar for an activity from S3.
//
// GET /api/v1/activities/{id}/edits
func (h *Handler) GetActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Try ShadowFS first (if sidecar_file_id is set), then fall back to legacy S3.
	var rawData []byte
	var sidecarFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(sidecar_file_id::text, '') FROM activities WHERE id = $1`,
		activityID,
	).Scan(&sidecarFileID)

	if sidecarFileID != "" && h.ShadowFS != nil {
		rc, getErr := h.ShadowFS.Get(r.Context(), userID, sidecarFileID)
		if getErr == nil {
			rawData, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if rawData == nil && h.Storage != nil {
		// Legacy S3 path.
		key := editorKey(userID, activityID)
		downloaded, _, dlErr := h.Storage.DownloadFromBucket(r.Context(), editorBucket(), key)
		if dlErr != nil {
			// No sidecar yet — return empty sidecar
			empty := &editor.EditSidecar{
				Version:    1,
				ActivityID: activityID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				Edits:      []editor.Edit{},
			}
			httputil.WriteJSON(w, http.StatusOK, empty)
			return
		}
		rawData = downloaded
	}

	if rawData == nil {
		// No sidecar stored anywhere — return empty sidecar.
		empty := &editor.EditSidecar{
			Version:    1,
			ActivityID: activityID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Edits:      []editor.Edit{},
		}
		httputil.WriteJSON(w, http.StatusOK, empty)
		return
	}

	var sidecar editor.EditSidecar
	if err := json.Unmarshal(rawData, &sidecar); err != nil {
		slog.Error("editor: unmarshal sidecar failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to parse edit sidecar", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, &sidecar)
}

// PutActivityEdits saves the edit sidecar for an activity to S3.
//
// PUT /api/v1/activities/{id}/edits
func (h *Handler) PutActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	sidecar, ok := httputil.DecodeJSON[editor.EditSidecar](w, r)
	if !ok {
		return
	}

	// Ensure metadata is consistent
	sidecar.ActivityID = activityID
	sidecar.UpdatedAt = time.Now()
	if sidecar.Version == 0 {
		sidecar.Version = 1
	}
	if sidecar.CreatedAt.IsZero() {
		sidecar.CreatedAt = time.Now()
	}

	raw, err := json.Marshal(&sidecar)
	if err != nil {
		slog.Error("editor: marshal sidecar failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to encode sidecar", r.URL.Path)
		return
	}

	// ShadowFS path (preferred) — legacy S3 as fallback.
	var sidecarFileID string
	if h.ShadowFS != nil {
		fileName := fmt.Sprintf("%s.edits.json", activityID)
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "edit",
			ContentType: "application/json",
			FileName:    fileName,
			SizeBytes:   int64(len(raw)),
			ParentType:  func() *string { s := "activity"; return &s }(),
			ParentID:    &activityID,
		}, bytes.NewReader(raw))
		if putErr != nil {
			slog.Error("editor: ShadowFS.Put sidecar failed", "id", activityID, "error", putErr)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store edit sidecar", r.URL.Path)
			return
		}
		sidecarFileID = rec.ID
	} else if h.Storage != nil {
		key := editorKey(userID, activityID)
		if err := h.Storage.UploadToBucket(r.Context(), editorBucket(), key, raw, "application/json"); err != nil {
			slog.Error("editor: upload sidecar failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store edit sidecar", r.URL.Path)
			return
		}
	}

	// Mark activity as having edits and link sidecar file.
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET has_edits = true WHERE id = $1`,
		activityID,
	)
	if err != nil {
		slog.Warn("editor: failed to set has_edits flag", "id", activityID, "error", err)
	}
	if sidecarFileID != "" {
		if _, dbErr := h.DB.Pool.Exec(r.Context(),
			`UPDATE activities SET sidecar_file_id = $1 WHERE id = $2`,
			sidecarFileID, activityID,
		); dbErr != nil {
			slog.Warn("editor: failed to set sidecar_file_id", "id", activityID, "error", dbErr)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, &sidecar)
}

// DeleteActivityEdits removes the edit sidecar and clears the has_edits flag.
//
// DELETE /api/v1/activities/{id}/edits
func (h *Handler) DeleteActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Resolve sidecar_file_id for ShadowFS deletion (best-effort).
	var sidecarFileID string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(sidecar_file_id::text, '') FROM activities WHERE id = $1`,
		activityID,
	).Scan(&sidecarFileID)

	// ShadowFS delete (preferred); legacy S3 delete as fallback.
	if sidecarFileID != "" && h.ShadowFS != nil {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, sidecarFileID); delErr != nil {
			slog.Warn("editor: ShadowFS.Delete sidecar failed", "id", activityID, "file_id", sidecarFileID, "error", delErr)
		}
	} else if h.Storage != nil {
		key := editorKey(userID, activityID)
		if err := h.Storage.DeleteFromBucket(r.Context(), editorBucket(), key); err != nil {
			slog.Warn("editor: s3 delete sidecar failed", "id", activityID, "error", err)
		}
	}

	// Clear has_edits flag and sidecar_file_id.
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET has_edits = false, edits_applied_at = NULL, sidecar_file_id = NULL WHERE id = $1`,
		activityID,
	)
	if err != nil {
		slog.Warn("editor: failed to clear has_edits flag", "id", activityID, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// DetectHRSpikesHandler detects heart rate spikes in an activity's stream data.
//
// POST /api/v1/activities/{id}/edits/detect-hr-spikes
func (h *Handler) DetectHRSpikesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Parse optional threshold from request body
	var req struct {
		Threshold int `json:"threshold"`
	}
	req.Threshold = 25 // default: 25 bpm jump = spike
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Threshold <= 0 {
			req.Threshold = 25
		}
	}

	// Load streams from DB
	streams, err := h.DB.GetStreams(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get streams failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
		return
	}

	// Convert to editor StreamPoints
	points := make([]editor.StreamPoint, 0, len(streams))
	for _, s := range streams {
		p := editor.StreamPoint{
			Time:      s.Time,
			HeartRate: s.HeartRate,
			Lat:       s.Lat,
			Lng:       s.Lng,
		}
		points = append(points, p)
	}

	spikeTimes := editor.DetectHRSpikes(points, req.Threshold)

	// Format as ISO 8601 strings
	timestamps := make([]string, 0, len(spikeTimes))
	for _, t := range spikeTimes {
		timestamps = append(timestamps, t.Format(time.RFC3339))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"count":      len(timestamps),
		"timestamps": timestamps,
		"threshold":  req.Threshold,
	})
}

// DetectGPSOutliersHandler detects GPS outliers in an activity's stream data.
//
// POST /api/v1/activities/{id}/edits/detect-gps-outliers
func (h *Handler) DetectGPSOutliersHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Load streams from DB
	streams, err := h.DB.GetStreams(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get streams failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
		return
	}

	// Convert to editor StreamPoints
	points := make([]editor.StreamPoint, 0, len(streams))
	for _, s := range streams {
		p := editor.StreamPoint{
			Time:      s.Time,
			HeartRate: s.HeartRate,
			Lat:       s.Lat,
			Lng:       s.Lng,
		}
		points = append(points, p)
	}

	maxSpeed := editor.SportMaxSpeed(activity.SportType)
	outlierTimes := editor.DetectGPSOutliers(points, maxSpeed, 3.0)

	// Format as ISO 8601 strings
	timestamps := make([]string, 0, len(outlierTimes))
	for _, t := range outlierTimes {
		timestamps = append(timestamps, t.Format(time.RFC3339))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"count":        len(timestamps),
		"timestamps":   timestamps,
		"max_speed_ms": maxSpeed,
		"sport_type":   activity.SportType,
	})
}

// ApplyActivityEdits applies the saved edits (HR spike smooth, GPS outlier smooth,
// trim) to the activity streams and triggers a recompute.
//
// POST /api/v1/activities/{id}/edits/apply
func (h *Handler) ApplyActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// 1. Load the sidecar — ShadowFS first, then legacy S3.
	var sidecar editor.EditSidecar
	hasSidecar := false
	{
		var rawSidecar []byte
		var sfID string
		_ = h.DB.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(sidecar_file_id::text, '') FROM activities WHERE id = $1`,
			activityID,
		).Scan(&sfID)
		if sfID != "" && h.ShadowFS != nil {
			if rc, getErr := h.ShadowFS.Get(r.Context(), userID, sfID); getErr == nil {
				rawSidecar, _ = io.ReadAll(rc)
				rc.Close()
			}
		}
		if rawSidecar == nil && h.Storage != nil {
			key := editorKey(userID, activityID)
			rawSidecar, _, _ = h.Storage.DownloadFromBucket(r.Context(), editorBucket(), key)
		}
		if rawSidecar != nil {
			if err := json.Unmarshal(rawSidecar, &sidecar); err != nil {
				slog.Error("editor: unmarshal sidecar failed", "id", activityID, "error", err)
				httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to parse edit sidecar", r.URL.Path)
				return
			}
			hasSidecar = len(sidecar.Edits) > 0
		}
	}

	// 2. Trigger a full recompute from original file (re-parse, re-insert streams)
	if err := h.reprocessActivity(r.Context(), activityID, userID, nil, 0, false, nil); err != nil {
		slog.Error("editor: apply recompute failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Recompute Error", err.Error(), r.URL.Path)
		return
	}

	// 3. Apply sidecar edits to the freshly inserted streams
	if hasSidecar {
		if err := h.applySidecarEdits(r.Context(), activityID, userID, &sidecar); err != nil {
			slog.Error("editor: apply sidecar edits failed", "id", activityID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Apply Error", err.Error(), r.URL.Path)
			return
		}
	}

	// Mark edits as applied
	now := time.Now()
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET edits_applied_at = $2 WHERE id = $1`,
		activityID, now,
	)
	if err != nil {
		slog.Warn("editor: failed to set edits_applied_at", "id", activityID, "error", err)
	}

	updated, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || updated == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

// applySidecarEdits reads the activity streams, applies edits from the sidecar,
// writes modified streams back to DB, and recalculates affected metrics.
func (h *Handler) applySidecarEdits(ctx context.Context, activityID, userID string, sidecar *editor.EditSidecar) error {
	// Load current streams
	streams, err := h.DB.GetStreams(ctx, activityID)
	if err != nil {
		return fmt.Errorf("load streams: %w", err)
	}
	if len(streams) == 0 {
		return nil
	}

	// Convert to editor.StreamPoint for the apply functions
	editorPoints := make([]editor.StreamPoint, len(streams))
	for i, s := range streams {
		editorPoints[i] = editor.StreamPoint{
			Time:      s.Time,
			HeartRate: s.HeartRate,
			Lat:       s.Lat,
			Lng:       s.Lng,
		}
	}

	// Build a sidecar with only smoothing edits (not trim/crop which change
	// slice boundaries and are handled differently)
	hasGPS := false
	hasHR := false
	smoothEdits := make([]editor.Edit, 0, len(sidecar.Edits))
	for _, edit := range sidecar.Edits {
		switch edit.Type {
		case editor.EditTypeGPSOutlierSmooth:
			hasGPS = true
			smoothEdits = append(smoothEdits, edit)
		case editor.EditTypeHRSpikeSmooth:
			hasHR = true
			smoothEdits = append(smoothEdits, edit)
		}
	}

	if !hasGPS && !hasHR {
		return nil
	}

	// Apply only smoothing edits (preserves slice length for 1:1 index mapping)
	smoothSidecar := &editor.EditSidecar{Edits: smoothEdits}
	modified := editor.ApplyEdits(editorPoints, smoothSidecar)

	// Apply GPS outlier corrections to DB
	if hasGPS {
		if err := h.applyGPSEditsToStreams(ctx, activityID, editorPoints, modified); err != nil {
			return fmt.Errorf("apply GPS edits: %w", err)
		}
	}

	// Apply HR spike corrections to DB
	if hasHR {
		if err := h.applyHREditsToStreams(ctx, activityID, editorPoints, modified); err != nil {
			return fmt.Errorf("apply HR edits: %w", err)
		}
	}

	// Recalculate distance, speed, and other metrics from the modified streams
	if hasGPS || hasHR {
		if err := h.recalcActivityMetricsFromStreams(ctx, activityID, userID); err != nil {
			return fmt.Errorf("recalc metrics: %w", err)
		}
	}

	// Rebuild heatmap lines if GPS was modified (skip for heatmap-excluded activities)
	if hasGPS {
		var excludeHeatmap bool
		_ = h.DB.Pool.QueryRow(ctx, "SELECT exclude_from_heatmap FROM activities WHERE id = $1", activityID).Scan(&excludeHeatmap)
		if excludeHeatmap {
			_ = h.DB.DeleteHeatmapData(ctx, activityID)
		} else if err := h.rebuildHeatmapForActivity(ctx, activityID, userID); err != nil {
			slog.Error("editor: rebuild heatmap failed", "id", activityID, "error", err)
		}
	}

	return nil
}

// applyGPSEditsToStreams writes interpolated or nulled GPS coordinates back to the DB.
func (h *Handler) applyGPSEditsToStreams(ctx context.Context, activityID string, original, modified []editor.StreamPoint) error {
	for i := range modified {
		if i >= len(original) {
			break
		}
		// Only update points that were changed
		origLat := original[i].Lat
		modLat := modified[i].Lat
		origLng := original[i].Lng
		modLng := modified[i].Lng

		latChanged := (origLat == nil) != (modLat == nil) || (origLat != nil && modLat != nil && *origLat != *modLat)
		lngChanged := (origLng == nil) != (modLng == nil) || (origLng != nil && modLng != nil && *origLng != *modLng)

		if !latChanged && !lngChanged {
			continue
		}

		_, err := h.DB.Pool.Exec(ctx,
			`UPDATE activity_streams SET lat = $1, lng = $2 WHERE activity_id = $3 AND time = $4`,
			modLat, modLng, activityID, modified[i].Time,
		)
		if err != nil {
			return fmt.Errorf("update GPS at %v: %w", modified[i].Time, err)
		}
	}
	return nil
}

// applyHREditsToStreams writes smoothed or nulled HR values back to the DB.
func (h *Handler) applyHREditsToStreams(ctx context.Context, activityID string, original, modified []editor.StreamPoint) error {
	for i := range modified {
		if i >= len(original) {
			break
		}
		origHR := original[i].HeartRate
		modHR := modified[i].HeartRate

		hrChanged := (origHR == nil) != (modHR == nil) || (origHR != nil && modHR != nil && *origHR != *modHR)
		if !hrChanged {
			continue
		}

		_, err := h.DB.Pool.Exec(ctx,
			`UPDATE activity_streams SET heart_rate = $1 WHERE activity_id = $2 AND time = $3`,
			modHR, activityID, modified[i].Time,
		)
		if err != nil {
			return fmt.Errorf("update HR at %v: %w", modified[i].Time, err)
		}
	}
	return nil
}

// recalcActivityMetricsFromStreams reloads streams from DB and recalculates
// distance, speed, max_speed, avg_hr, max_hr for the activity.
func (h *Handler) recalcActivityMetricsFromStreams(ctx context.Context, activityID, userID string) error {
	points, err := h.DB.GetStreamPoints(ctx, activityID)
	if err != nil {
		return fmt.Errorf("get stream points: %w", err)
	}
	if len(points) == 0 {
		return nil
	}

	metrics := editor.MetricsFromStreams(points)

	// Build update fields
	fields := map[string]any{
		"distance_m":   metrics.DistanceM,
		"avg_speed_ms": metrics.AvgSpeedMS,
		"max_speed_ms": metrics.MaxSpeedMS,
	}
	if metrics.AvgHR != nil {
		fields["avg_heart_rate"] = int(*metrics.AvgHR)
	}
	if metrics.MaxHR != nil {
		fields["max_heart_rate"] = *metrics.MaxHR
	}

	return h.DB.UpdateEnrichmentFields(ctx, activityID, fields)
}

// rebuildHeatmapForActivity deletes and re-creates heatmap_lines and heatmap_points
// for an activity after GPS edits were applied.
func (h *Handler) rebuildHeatmapForActivity(ctx context.Context, activityID, userID string) error {
	activity, err := h.DB.GetActivity(ctx, activityID)
	if err != nil || activity == nil {
		return fmt.Errorf("load activity for heatmap rebuild: %w", err)
	}

	// Delete + re-insert heatmap data (lines + points)
	if err := h.DB.DeleteHeatmapData(ctx, activityID); err != nil {
		return fmt.Errorf("delete heatmap data: %w", err)
	}
	subTypePtr := activity.SubType
	interval, _ := h.DB.GetUserHeatmapInterval(ctx, userID)
	if err := h.DB.InsertHeatmapData(ctx, userID, activityID, activity.SportType, subTypePtr, activity.StartedAt, interval); err != nil {
		return fmt.Errorf("insert heatmap data: %w", err)
	}
	h.DB.InvalidateTerraIncognitaCache(ctx, userID)

	return nil
}
