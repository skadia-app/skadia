package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
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
// Assigns a queued image to an activity: moves file from temp S3 to activity_photos.
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

	// Load queue item
	item, err := h.DB.GetImportQueueItem(r.Context(), id, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "queue item not found", r.URL.Path)
		return
	}
	if item.S3Bucket == nil || item.S3Key == nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "queue item has no stored file", r.URL.Path)
		return
	}

	// Verify activity ownership
	activity, err := h.DB.GetActivity(r.Context(), req.ActivityID)
	if err != nil || activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Download from temp S3
	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), *item.S3Bucket, *item.S3Key)
	if err != nil {
		slog.Error("assign queue item: s3 download failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to retrieve file", r.URL.Path)
		return
	}
	if ct == "" {
		ct = item.ContentType
	}

	// Re-extract EXIF
	takenAt, lat, lng, locationSource := extractExifData(data, ct)

	// Generate thumbnail
	thumbData, _ := imageutil.Resize(data, 200, 200, 80)

	// Determine extension and hash
	ext := imageExtFromContentType(ct)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	mediaBucket := mediaBucketName()
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, hash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, hash)

	// Upload to permanent location
	if err := h.Storage.UploadToBucket(r.Context(), mediaBucket, s3Key, data, ct); err != nil {
		slog.Error("assign queue item: s3 upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to store photo", r.URL.Path)
		return
	}
	if thumbData != nil {
		_ = h.Storage.UploadToBucket(r.Context(), mediaBucket, s3KeyThumb, thumbData, "image/jpeg")
	} else {
		s3KeyThumb = s3Key
	}

	fileSize := int64(len(data))
	// Save to activity_photos
	photo, err := h.DB.InsertActivityPhoto(r.Context(), model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &req.ActivityID,
		S3Bucket:         mediaBucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		OriginalFilename: item.OriginalFilename,
		ContentType:      ct,
		FileSizeBytes:    fileSize,
		FileHash:         &hash,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		LocationSource:   locationSource,
	})
	if err != nil {
		slog.Error("assign queue item: insert photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save photo", r.URL.Path)
		return
	}

	// Delete temp file from S3
	_ = h.Storage.DeleteFromBucket(r.Context(), *item.S3Bucket, *item.S3Key)

	// Mark queue item resolved
	now := time.Now()
	_ = h.DB.UpdateImportQueueStatus(r.Context(), id, userID, "resolved", &now)

	setPhotoURLs(&photo)
	writeJSON(w, http.StatusCreated, photo)
}
