package handler

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// UploadRoutePhoto handles POST /api/v1/routes/{id}/photos
// Accepts a multipart form with a "photo" field, generates thumbnails, uploads to S3.
func (h *Handler) UploadRoutePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")

	// Verify ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("upload route photo: get route failed", "id", routeID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load route", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	if err := r.ParseMultipartForm(maxPhotoSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large (max 20 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no photo in request", r.URL.Path)
		return
	}
	defer file.Close()

	// Quick pre-check on client-supplied Content-Type.
	ctFromHeader := header.Header.Get("Content-Type")
	if ctFromHeader != "" && !strings.HasPrefix(ctFromHeader, "image/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read route photo data failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Magic-byte detection for actual content type.
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)
	if !strings.HasPrefix(ct, "image/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Generate thumbnail (200x200).
	thumbData, _ := imageutil.Resize(data, 200, 200, 80)

	// Generate preview thumbnail.
	var previewData []byte
	if settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID); err == nil {
		if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
			previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
		}
	}

	// S3 paths.
	ext := ".jpg"
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "heic"), strings.Contains(ct, "heif"):
		ext = ".heic"
	case strings.Contains(ct, "avif"):
		ext = ".avif"
	}

	bucket := mediaBucketName()
	s3Key := fmt.Sprintf("route-photos/%s/%s/%s%s", userID, routeID, hash, ext)
	s3KeyThumb := fmt.Sprintf("route-photos/%s/%s/%s_thumb.jpg", userID, routeID, hash)

	// Track uploaded S3 keys for cleanup on failure.
	var uploadedKeys []string
	cleanupS3 := func() {
		for _, key := range uploadedKeys {
			if delErr := h.Storage.DeleteFromBucket(r.Context(), bucket, key); delErr != nil {
				slog.Warn("s3 orphan cleanup failed", "key", key, "error", delErr)
			}
		}
	}

	// Upload original.
	if err := h.Storage.UploadToBucket(r.Context(), bucket, s3Key, data, ct); err != nil {
		slog.Error("s3 upload route photo failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to store photo", r.URL.Path)
		return
	}
	uploadedKeys = append(uploadedKeys, s3Key)

	// Upload thumbnail.
	if thumbData != nil {
		if err := h.Storage.UploadToBucket(r.Context(), bucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
			slog.Warn("s3 upload route thumb failed", "error", err)
			s3KeyThumb = s3Key
		} else {
			uploadedKeys = append(uploadedKeys, s3KeyThumb)
		}
	} else {
		s3KeyThumb = s3Key
	}

	// Upload preview.
	var s3KeyPreview *string
	if previewData != nil {
		key := fmt.Sprintf("route-photos/%s/%s/%s_preview.jpg", userID, routeID, hash)
		if err := h.Storage.UploadToBucket(r.Context(), bucket, key, previewData, "image/jpeg"); err != nil {
			slog.Warn("s3 upload route preview failed", "error", err)
		} else {
			s3KeyPreview = &key
			uploadedKeys = append(uploadedKeys, key)
		}
	}

	photo := model.RoutePhoto{
		RouteID:      routeID,
		UserID:       userID,
		FileHash:     hash,
		S3Key:        s3Key,
		S3KeyThumb:   s3KeyThumb,
		S3KeyPreview: s3KeyPreview,
		OriginalName: &header.Filename,
	}

	saved, err := h.DB.InsertRoutePhoto(r.Context(), photo)
	if err != nil {
		slog.Error("insert route photo failed", "error", err)
		cleanupS3()
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save photo", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, saved)
}

// DeleteRoutePhoto handles DELETE /api/v1/routes/{id}/photos/{photoId}
func (h *Handler) DeleteRoutePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")
	photoID := chi.URLParam(r, "photoId")

	// Verify route ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("delete route photo: get route failed", "id", routeID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load route", r.URL.Path)
		return
	}

	bucket := mediaBucketName()
	s3Key, s3KeyThumb, s3KeyPreview, err := h.DB.DeleteRoutePhoto(r.Context(), photoID, userID, routeID)
	if err != nil {
		slog.Error("delete route photo failed", "error", err)
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}

	_ = h.Storage.DeleteFromBucket(r.Context(), bucket, s3Key)
	if s3KeyThumb != s3Key {
		_ = h.Storage.DeleteFromBucket(r.Context(), bucket, s3KeyThumb)
	}
	if s3KeyPreview != nil && *s3KeyPreview != "" && *s3KeyPreview != s3Key {
		_ = h.Storage.DeleteFromBucket(r.Context(), bucket, *s3KeyPreview)
	}

	w.WriteHeader(http.StatusNoContent)
}

// reorderRoutePhotosRequest is the JSON body for PUT /routes/{id}/photos/reorder.
type reorderRoutePhotosRequest struct {
	PhotoIDs []string `json:"photo_ids"`
}

// ReorderRoutePhotos handles PUT /api/v1/routes/{id}/photos/reorder
func (h *Handler) ReorderRoutePhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")

	// Verify route ownership.
	_, err := h.DB.GetRoute(r.Context(), routeID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("reorder route photos: get route failed", "id", routeID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load route", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[reorderRoutePhotosRequest](w, r)
	if !ok {
		return
	}
	if len(req.PhotoIDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "photo_ids required", r.URL.Path)
		return
	}

	if err := h.DB.ReorderRoutePhotos(r.Context(), routeID, req.PhotoIDs); err != nil {
		slog.Error("reorder route photos failed", "error", err, "route_id", routeID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to reorder photos", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
