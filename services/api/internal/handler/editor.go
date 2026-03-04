package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/editor"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	key := editorKey(userID, activityID)
	data, _, err := h.Storage.DownloadFromBucket(r.Context(), editorBucket(), key)
	if err != nil {
		// No sidecar yet — return empty sidecar
		empty := &editor.EditSidecar{
			Version:    1,
			ActivityID: activityID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Edits:      []editor.Edit{},
		}
		writeJSON(w, http.StatusOK, empty)
		return
	}

	var sidecar editor.EditSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		slog.Error("editor: unmarshal sidecar failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to parse edit sidecar", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, &sidecar)
}

// PutActivityEdits saves the edit sidecar for an activity to S3.
//
// PUT /api/v1/activities/{id}/edits
func (h *Handler) PutActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	var sidecar editor.EditSidecar
	if err := json.NewDecoder(r.Body).Decode(&sidecar); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
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
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to encode sidecar", r.URL.Path)
		return
	}

	key := editorKey(userID, activityID)
	if err := h.Storage.UploadToBucket(r.Context(), editorBucket(), key, raw, "application/json"); err != nil {
		slog.Error("editor: upload sidecar failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to store edit sidecar", r.URL.Path)
		return
	}

	// Mark activity as having edits
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET has_edits = true, updated_at = NOW() WHERE id = $1`,
		activityID,
	)
	if err != nil {
		slog.Warn("editor: failed to set has_edits flag", "id", activityID, "error", err)
	}

	writeJSON(w, http.StatusOK, &sidecar)
}

// DeleteActivityEdits removes the edit sidecar and clears the has_edits flag.
//
// DELETE /api/v1/activities/{id}/edits
func (h *Handler) DeleteActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	key := editorKey(userID, activityID)
	// Best-effort delete from S3
	if err := h.Storage.DeleteFromBucket(r.Context(), editorBucket(), key); err != nil {
		slog.Warn("editor: s3 delete sidecar failed", "id", activityID, "error", err)
	}

	// Clear has_edits flag
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET has_edits = false, edits_applied_at = NULL, updated_at = NOW() WHERE id = $1`,
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
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
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load streams", r.URL.Path)
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

	writeJSON(w, http.StatusOK, map[string]any{
		"count":      len(timestamps),
		"timestamps": timestamps,
		"threshold":  req.Threshold,
	})
}

// ApplyActivityEdits applies the saved edits and triggers a recompute.
//
// POST /api/v1/activities/{id}/edits/apply
func (h *Handler) ApplyActivityEdits(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("editor: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Trigger a full recompute — reprocessActivity already handles all recalculation
	if err := h.reprocessActivity(r.Context(), activityID, userID, nil); err != nil {
		slog.Error("editor: apply recompute failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Recompute Error", err.Error(), r.URL.Path)
		return
	}

	// Mark edits as applied
	now := time.Now()
	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE activities SET edits_applied_at = $2, updated_at = NOW() WHERE id = $1`,
		activityID, now,
	)
	if err != nil {
		slog.Warn("editor: failed to set edits_applied_at", "id", activityID, "error", err)
	}

	updated, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}
