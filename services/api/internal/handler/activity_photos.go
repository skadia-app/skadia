package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bep/imagemeta"
	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const maxPhotoSize = 20 << 20 // 20 MB

func (h *Handler) UploadActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before accepting upload.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("upload photo: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	if err := r.ParseMultipartForm(maxPhotoSize); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "file too large (max 20 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "no photo in request", r.URL.Path)
		return
	}
	defer file.Close()

	// Content-Type pruefen
	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	if !strings.HasPrefix(ct, "image/") {
		writeError(w, http.StatusBadRequest, "Bad Request", "only images allowed", r.URL.Path)
		return
	}

	// Datei vollstaendig lesen
	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read photo data failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Hash
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// EXIF-Daten extrahieren (GPS + Zeitstempel)
	takenAt, lat, lng, locationSource := extractExifData(data, ct)

	// Falls kein EXIF-GPS: Track-Interpolation versuchen
	if lat == nil && takenAt != nil && activityID != "" {
		lat, lng = h.interpolateFromTrack(r.Context(), activityID, userID, *takenAt)
		if lat != nil {
			ls := "activity_track"
			locationSource = &ls
		}
	}

	// Thumbnail (200×200) generieren
	thumbData, _ := imageutil.Resize(data, 200, 200, 80)

	// Preview-Thumbnail generieren (Größe aus User-Einstellungen)
	previewData, s3KeyPreview := []byte(nil), ""
	if settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID); err == nil {
		if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
			previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
		}
	}

	// S3-Pfade
	ext := ".jpg"
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "heic"), strings.Contains(ct, "heif"):
		ext = ".heic"
	case strings.Contains(ct, "avif"):
		ext = ".avif"
	}
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, hash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, hash)
	bucket := "skadia-media"

	// Upload Original
	if err := h.Storage.UploadToBucket(r.Context(), bucket, s3Key, data, ct); err != nil {
		slog.Error("s3 upload photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to store photo", r.URL.Path)
		return
	}

	// Upload Thumbnail (200px)
	if thumbData != nil {
		if err := h.Storage.UploadToBucket(r.Context(), bucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
			slog.Warn("s3 upload thumb failed", "error", err)
			s3KeyThumb = s3Key // Fallback: Original
		}
	} else {
		s3KeyThumb = s3Key // Original ist bereits klein genug
	}

	// Upload Preview (2048/4K)
	if previewData != nil {
		s3KeyPreview = fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, hash)
		if err := h.Storage.UploadToBucket(r.Context(), bucket, s3KeyPreview, previewData, "image/jpeg"); err != nil {
			slog.Warn("s3 upload preview failed", "error", err)
			s3KeyPreview = ""
		}
	}

	photo := model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &activityID,
		S3Bucket:         bucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		S3KeyPreview:     s3KeyPreview,
		OriginalFilename: header.Filename,
		ContentType:      ct,
		FileSizeBytes:    int64(len(data)),
		FileHash:         &hash,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		LocationSource:   locationSource,
	}

	saved, err := h.DB.InsertActivityPhoto(r.Context(), photo)
	if err != nil {
		slog.Error("insert activity photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save photo", r.URL.Path)
		return
	}

	setPhotoURLs(&saved)
	writeJSON(w, http.StatusCreated, saved)
}

func (h *Handler) ListActivityPhotos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before listing photos.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("list activity photos: get activity failed", "id", activityID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	photos, err := h.DB.ListActivityPhotos(r.Context(), activityID, userID)
	if err != nil {
		slog.Error("list activity photos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}
	for i := range photos {
		setPhotoURLs(&photos[i])
	}
	writeJSON(w, http.StatusOK, photos)
}

func (h *Handler) DeleteActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	bucket, key, keyThumb, err := h.DB.DeleteActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		slog.Error("delete activity photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete photo", r.URL.Path)
		return
	}
	_ = h.Storage.DeleteFromBucket(r.Context(), bucket, key)
	if keyThumb != key {
		_ = h.Storage.DeleteFromBucket(r.Context(), bucket, keyThumb)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StreamActivityPhotoThumb(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}
	data, _, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3KeyThumb)
	if err != nil {
		slog.Error("s3 download thumb failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load thumbnail", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

func (h *Handler) StreamActivityPhotoFull(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}
	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3Key)
	if err != nil {
		slog.Error("s3 download photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load photo", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// patchPhotoLocationRequest is the body for PATCH /activities/{id}/photos/{photoID}.
type patchPhotoLocationRequest struct {
	Lat            *float64   `json:"lat"`
	Lng            *float64   `json:"lng"`
	TakenAt        *time.Time `json:"taken_at"`
	LocationSource string     `json:"location_source"`
}

// PatchActivityPhotoLocation handles PATCH /api/v1/activities/{id}/photos/{photoID}.
// Updates the GPS position and/or timestamp of a photo. When location_source is
// 'manual' a meta-sidecar JSON is written to S3 so the original file remains untouched.
func (h *Handler) PatchActivityPhotoLocation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	photoID := chi.URLParam(r, "photoID")

	var req patchPhotoLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	// Default location_source to 'manual' when not provided.
	if req.LocationSource == "" {
		req.LocationSource = "manual"
	}

	photo, err := h.DB.UpdateActivityPhotoLocation(
		r.Context(), photoID, userID,
		req.Lat, req.Lng, req.TakenAt, req.LocationSource,
	)
	if err != nil {
		slog.Error("patch photo location: update failed", "photo_id", photoID, "error", err)
		writeError(w, http.StatusNotFound, "Not Found", "photo not found or not owned by user", r.URL.Path)
		return
	}

	// ShadowFS: write meta-sidecar when position was manually set.
	if req.LocationSource == "manual" && photo.FileHash != nil {
		h.writePhotoMetaSidecar(r.Context(), userID, *photo.FileHash, req)
	}

	setPhotoURLs(&photo)
	writeJSON(w, http.StatusOK, photo)
}

// writePhotoMetaSidecar writes a .meta.json sidecar to S3 (skadia-media bucket).
// Path: activity-photos/{userID}/{hash}.meta.json
// This follows the Shadow FS principle: originals stay untouched; annotations go in sidecars.
func (h *Handler) writePhotoMetaSidecar(ctx context.Context, userID, fileHash string, req patchPhotoLocationRequest) {
	type metaSidecar struct {
		Lat            *float64   `json:"lat"`
		Lng            *float64   `json:"lng"`
		TakenAt        *time.Time `json:"taken_at,omitempty"`
		LocationSource string     `json:"location_source"`
		UpdatedAt      time.Time  `json:"updated_at"`
	}

	meta := metaSidecar{
		Lat:            req.Lat,
		Lng:            req.Lng,
		TakenAt:        req.TakenAt,
		LocationSource: req.LocationSource,
		UpdatedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		slog.Error("photo meta sidecar: marshal failed", "error", err)
		return
	}

	bucket := mediaBucketName()
	key := fmt.Sprintf("activity-photos/%s/%s.meta.json", userID, fileHash)

	if err := h.Storage.UploadToBucket(ctx, bucket, key, data, "application/json"); err != nil {
		slog.Error("photo meta sidecar: upload failed", "bucket", bucket, "key", key, "error", err)
		return
	}

	slog.Info("photo meta sidecar written", "bucket", bucket, "key", key)
}

func (h *Handler) StreamActivityPhotoPreview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}
	// Use preview key if available, otherwise fall back to original.
	key := photo.S3KeyPreview
	if key == "" {
		key = photo.S3Key
	}
	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, key)
	if err != nil && key != photo.S3Key {
		// Preview key didn't exist (e.g., generated before this feature) — fall back.
		data, ct, err = h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3Key)
	}
	if err != nil {
		slog.Error("s3 download preview failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load preview", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// RecomputePhotoPreviews regenerates preview thumbnails for all user photos
// using the current photo_preview_size setting. Runs as a background job.
func (h *Handler) RecomputePhotoPreviews(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load settings", r.URL.Path)
		return
	}
	maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize)

	photos, err := h.DB.ListAllUserPhotos(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_preview_recompute", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("recompute previews: failed to create background job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	go func() {
		ctx := context.Background()
		processed := int64(0)
		anyFailed := false

		for _, photo := range photos {
			data, _, err := h.Storage.DownloadFromBucket(ctx, photo.S3Bucket, photo.S3Key)
			if err != nil {
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			if maxW == 0 {
				if photo.S3KeyPreview != "" {
					_ = h.DB.UpdateActivityPhotoPreviewKey(ctx, photo.ID, userID, "")
				}
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			previewData, err := imageutil.Resize(data, maxW, maxH, 85)
			if err != nil || previewData == nil {
				if photo.S3KeyPreview != "" {
					_ = h.DB.UpdateActivityPhotoPreviewKey(ctx, photo.ID, userID, "")
				}
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			if photo.FileHash == nil {
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			s3KeyPreview := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, *photo.FileHash)
			if err := h.Storage.UploadToBucket(ctx, photo.S3Bucket, s3KeyPreview, previewData, "image/jpeg"); err != nil {
				slog.Warn("recompute preview: s3 upload failed", "photo_id", photo.ID, "error", err)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}
			if err := h.DB.UpdateActivityPhotoPreviewKey(ctx, photo.ID, userID, s3KeyPreview); err != nil {
				anyFailed = true
			}
			processed++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (h *Handler) ListMapPhotos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	entries, err := h.DB.ListMapPhotos(r.Context(), userID)
	if err != nil {
		slog.Error("list map photos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list map photos", r.URL.Path)
		return
	}
	for i := range entries {
		entries[i].ThumbURL = "/photos/" + entries[i].ID + "/thumb"
	}
	writeJSON(w, http.StatusOK, entries)
}

// setPhotoURLs sets the computed URL fields on an ActivityPhoto.
// NOTE: paths are relative to /api/v1 — the frontend's apiClient/apiDownload
// automatically prepends API_BASE (/api/v1), so we must NOT include it here.
func setPhotoURLs(p *model.ActivityPhoto) {
	base := "/photos/" + p.ID
	p.ThumbURL = base + "/thumb"
	p.FullURL = base + "/full"
	if p.S3KeyPreview != "" {
		p.PreviewURL = base + "/preview"
	} else {
		p.PreviewURL = base + "/full" // fallback: original
	}
}


// extractExifData versucht GPS-Koordinaten und Aufnahmezeit aus EXIF-Metadaten zu lesen.
// Unterstuetzt JPEG, PNG, TIFF und WebP. Gibt nil-Werte zurueck wenn kein EXIF vorhanden.
func extractExifData(data []byte, contentType string) (takenAt *time.Time, lat *float64, lng *float64, locationSource *string) {
	format := imagemeta.JPEG
	switch {
	case strings.Contains(contentType, "image/png"):
		format = imagemeta.PNG
	case strings.Contains(contentType, "image/tiff"):
		format = imagemeta.TIFF
	case strings.Contains(contentType, "image/webp"):
		format = imagemeta.WebP
	case strings.Contains(contentType, "image/heic"),
		strings.Contains(contentType, "image/heif"):
		format = imagemeta.HEIF
	case strings.Contains(contentType, "image/avif"):
		format = imagemeta.AVIF
	}

	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: format,
		Sources:     imagemeta.EXIF,
		HandleTag: func(info imagemeta.TagInfo) error {
			tags.Add(info)
			return nil
		},
	})
	if err != nil {
		return nil, nil, nil, nil
	}

	// Aufnahmezeit aus DateTimeOriginal / DateTime
	if tm, err := tags.GetDateTime(); err == nil && !tm.IsZero() {
		takenAt = &tm
	}

	// GPS-Koordinaten
	if la, lo, err := tags.GetLatLong(); err == nil && (la != 0 || lo != 0) {
		ls := "exif"
		lat = &la
		lng = &lo
		locationSource = &ls
	}

	return takenAt, lat, lng, locationSource
}

// interpolateFromTrack sucht den GPS-Punkt der Aktivitaet zum Zeitpunkt takenAt.
// TODO: Query activity_streams fuer naechsten Zeitpunkt zu takenAt
func (h *Handler) interpolateFromTrack(ctx context.Context, activityID, userID string, takenAt time.Time) (lat *float64, lng *float64) {
	sp, err := h.DB.GetStreamPointNearTime(ctx, activityID, takenAt)
	if err != nil || sp == nil {
		return nil, nil
	}
	return sp.Lat, sp.Lng
}

