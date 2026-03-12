package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// ListImportQueue handles GET /api/v1/import-queue
// Query param: status=pending|resolved|dismissed|all (default: pending)
func (h *Handler) ListImportQueue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}

	items, err := h.DB.ListImportQueue(r.Context(), userID, status)
	if err != nil {
		slog.Error("list import queue failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list import queue", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CountImportQueue handles GET /api/v1/import-queue/count
func (h *Handler) CountImportQueue(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	count, err := h.DB.CountPendingImportQueue(r.Context(), userID)
	if err != nil {
		slog.Error("count import queue failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to count import queue", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// PatchImportQueueItem handles PATCH /api/v1/import-queue/{id}
// Body: {"status": "resolved"|"dismissed"|"pending"}
func (h *Handler) PatchImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.Status != "resolved" && req.Status != "dismissed" && req.Status != "pending" {
		writeError(w, http.StatusBadRequest, "Bad Request", "status must be resolved, dismissed, or pending", r.URL.Path)
		return
	}

	var resolvedAt *time.Time
	if req.Status == "resolved" || req.Status == "dismissed" {
		now := time.Now()
		resolvedAt = &now
	}

	if err := h.DB.UpdateImportQueueStatus(r.Context(), id, userID, req.Status, resolvedAt); err != nil {
		slog.Error("update import queue status failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update queue item", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteImportQueueItem handles DELETE /api/v1/import-queue/{id}
func (h *Handler) DeleteImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteImportQueueItem(r.Context(), id, userID); err != nil {
		slog.Error("delete import queue item failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete queue item", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AssignImportQueueItem handles POST /api/v1/import-queue/{id}/assign
// Assigns a queued image_no_match item to an activity via the ImportService pipeline.
// Body: {"activity_id": "uuid"}
func (h *Handler) AssignImportQueueItem(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		ActivityID string `json:"activity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ActivityID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "activity_id is required", r.URL.Path)
		return
	}

	// Verify activity ownership before processing
	activity, err := h.DB.GetActivity(r.Context(), req.ActivityID)
	if err != nil || activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	result, err := h.ImportService.ResolveQueuedPhoto(r.Context(), userID, id, req.ActivityID)
	if err != nil {
		slog.Error("assign queue item: ResolveQueuedPhoto failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// GetImportQueueItemThumb handles GET /api/v1/import-queue/{id}/thumb
// Returns the thumbnail image for a queued item stored in S3.
func (h *Handler) GetImportQueueItemThumb(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	item, err := h.DB.GetImportQueueItem(r.Context(), id, userID)
	if err != nil || item == nil {
		writeError(w, http.StatusNotFound, "Not Found", "queue item not found", r.URL.Path)
		return
	}

	thumbKey, ok := item.Metadata["s3_key_thumb"].(string)
	if !ok || thumbKey == "" {
		writeError(w, http.StatusNotFound, "Not Found", "no thumbnail available for this queue item", r.URL.Path)
		return
	}

	bucket := "skadia-media"
	if item.S3Bucket != nil {
		bucket = *item.S3Bucket
	}

	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), bucket, thumbKey)
	if err != nil {
		slog.Error("get queue item thumb: s3 download failed", "id", id, "key", thumbKey, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve thumbnail", r.URL.Path)
		return
	}

	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

