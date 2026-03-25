package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bep/imagemeta"
	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const maxPhotoSize = 200 << 20 // 200 MB (Videos können größer sein)

func (h *Handler) UploadActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before accepting upload.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("upload photo: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	if err := r.ParseMultipartForm(maxPhotoSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file too large (max 200 MB)", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no photo in request", r.URL.Path)
		return
	}
	defer file.Close()

	// Quick pre-check on client-supplied Content-Type (before reading body).
	ctFromHeader := header.Header.Get("Content-Type")
	if ctFromHeader != "" && !strings.HasPrefix(ctFromHeader, "image/") && !strings.HasPrefix(ctFromHeader, "video/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images and videos are allowed", r.URL.Path)
		return
	}

	// Datei vollstaendig lesen
	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("read photo data failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Magic-byte detection: determine actual content type from file bytes,
	// ignoring the client-supplied header to prevent content-type spoofing.
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)

	// Fallback auf Datei-Extension wenn http.DetectContentType() kein Bild/Video erkennt.
	// Das ist nötig für HEIC/JXL (geben application/octet-stream zurück) und mancher Videos.
	if ct == "application/octet-stream" || ct == "application/zip" {
		ct = contentTypeFromExtension(strings.ToLower(header.Filename))
	}

	if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only images and videos are allowed", r.URL.Path)
		return
	}

	// Delegate all remaining processing (dedup, pHash, thumbnails, S3 upload, DB insert,
	// CLIP embed event) to the shared ImportService.ProcessPhoto pipeline.
	photoMeta := importer.PhotoMetadata{
		OriginalFilename: header.Filename,
		ContentType:      ct,
	}

	result, err := h.ImportService.ProcessPhoto(r.Context(), userID, activityID, data, photoMeta)
	if err != nil {
		slog.Error("upload photo: ProcessPhoto failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to process photo", r.URL.Path)
		return
	}

	// registerInShadowFS stores the photo in ShadowFS and links file_id on the DB row.
	registerInShadowFS := func(photoID string) {
		if h.ShadowFS == nil {
			return
		}
		actIDPtr := &activityID
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			ParentType:  func() *string { s := "activity"; return &s }(),
			ParentID:    actIDPtr,
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Warn("upload photo: ShadowFS.Put failed (non-fatal)", "photo_id", photoID, "error", putErr)
			return
		}
		// Link file_id on the activity_photos row.
		if _, execErr := h.DB.Pool.Exec(r.Context(),
			`UPDATE activity_photos SET file_id = $1 WHERE id = $2 AND user_id = $3`,
			rec.ID, photoID, userID,
		); execErr != nil {
			slog.Warn("upload photo: link file_id failed", "photo_id", photoID, "file_id", rec.ID, "error", execErr)
		}
	}

	switch result.Status {
	case "duplicate_exact", "duplicate_phash":
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
	case "replaced":
		// Higher-resolution variant replaced an existing photo — also register in ShadowFS.
		registerInShadowFS(result.PhotoID)
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "replaced", "photo_id": result.PhotoID})
	default:
		// "created" — register in ShadowFS (preferred) and return the full photo record.
		registerInShadowFS(result.PhotoID)

		saved, err := h.DB.GetActivityPhoto(r.Context(), result.PhotoID, userID)
		if err != nil {
			slog.Error("upload photo: fetch saved photo failed", "photo_id", result.PhotoID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load saved photo", r.URL.Path)
			return
		}
		setPhotoURLs(&saved)
		httputil.WriteJSON(w, http.StatusCreated, saved)
	}
}

func (h *Handler) ListActivityPhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	// Verify ownership before listing photos.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("list activity photos: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	photos, err := h.DB.ListActivityPhotos(r.Context(), activityID, userID)
	if err != nil {
		slog.Error("list activity photos failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}
	for i := range photos {
		setPhotoURLs(&photos[i])
	}
	httputil.WriteJSON(w, http.StatusOK, photos)
}

func (h *Handler) DeleteActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")
	bucket, key, keyThumb, keyPreview, fileID, err := h.DB.DeleteActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		slog.Error("delete activity photo failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete photo", r.URL.Path)
		return
	}

	// ShadowFS path (preferred): delete original + any registered sidecar for this photo.
	if h.ShadowFS != nil && fileID != "" {
		if delErr := h.ShadowFS.Delete(r.Context(), userID, fileID); delErr != nil {
			slog.Warn("delete activity photo: ShadowFS.Delete failed", "file_id", fileID, "error", delErr)
		}
		// Delete any sidecar registered with parent_type='photo', parent_id=photoID.
		if sidecarRows, listErr := h.DB.Pool.Query(r.Context(),
			`SELECT id::text FROM user_files WHERE user_id = $1 AND parent_type = 'photo' AND parent_id = $2`,
			userID, photoID,
		); listErr == nil {
			defer sidecarRows.Close()
			for sidecarRows.Next() {
				var sidecarID string
				if scanErr := sidecarRows.Scan(&sidecarID); scanErr == nil && sidecarID != "" {
					if delErr := h.ShadowFS.Delete(r.Context(), userID, sidecarID); delErr != nil {
						slog.Warn("delete activity photo: ShadowFS.Delete sidecar failed", "sidecar_id", sidecarID, "error", delErr)
					}
				}
			}
		}
	}

	// Legacy S3 fallback: runs when ShadowFS is nil or this photo has no file_id yet.
	if h.Storage != nil && (h.ShadowFS == nil || fileID == "") {
		_ = h.Storage.DeleteFromBucket(r.Context(), bucket, key)
		if keyThumb != key {
			_ = h.Storage.DeleteFromBucket(r.Context(), bucket, keyThumb)
		}
		if keyPreview != "" && keyPreview != key {
			_ = h.Storage.DeleteFromBucket(r.Context(), bucket, keyPreview)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StreamActivityPhotoThumb(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}

	// Try S3 first (thumbs are generated artifacts stored in S3).
	if h.Storage != nil && photo.S3KeyThumb != "" {
		data, _, dlErr := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3KeyThumb)
		if dlErr == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "private, max-age=86400")
			w.Write(data)
			return
		}
		slog.Warn("stream photo thumb: S3 download failed, trying ShadowFS fallback",
			"photo_id", photoID, "error", dlErr)
	}

	// ShadowFS fallback: serve original via Resolve (redirect/serve).
	if h.ShadowFS != nil && photo.FileID != nil && *photo.FileID != "" {
		resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, *photo.FileID)
		if resolveErr == nil {
			serveResolvedFile(w, r, resolved)
			return
		}
		slog.Warn("stream photo thumb: ShadowFS.Resolve failed",
			"photo_id", photoID, "file_id", *photo.FileID, "error", resolveErr)
	}

	httputil.WriteError(w, http.StatusNotFound, "Not Found", "thumbnail not available", r.URL.Path)
}

func (h *Handler) StreamActivityPhotoFull(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}

	// ShadowFS path (preferred): resolve via file_id when available.
	if h.ShadowFS != nil && photo.FileID != nil && *photo.FileID != "" {
		resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, *photo.FileID)
		if resolveErr == nil {
			serveResolvedFile(w, r, resolved)
			return
		}
		slog.Warn("stream photo full: ShadowFS.Resolve failed, falling back to S3", "file_id", *photo.FileID, "error", resolveErr)
	}

	// Legacy S3 fallback.
	if h.Storage == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "storage not configured", r.URL.Path)
		return
	}
	data, ct, err := h.Storage.DownloadFromBucket(r.Context(), photo.S3Bucket, photo.S3Key)
	if err != nil {
		slog.Error("s3 download photo failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load photo", r.URL.Path)
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")

	req, ok := httputil.DecodeJSON[patchPhotoLocationRequest](w, r)
	if !ok {
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
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found or not owned by user", r.URL.Path)
		return
	}

	// ShadowFS: write meta-sidecar when position was manually set.
	if req.LocationSource == "manual" && photo.FileHash != nil {
		h.writePhotoMetaSidecar(r.Context(), userID, photoID, *photo.FileHash, req)
	}

	setPhotoURLs(&photo)
	httputil.WriteJSON(w, http.StatusOK, photo)
}

// writePhotoMetaSidecar writes a .meta.json sidecar for a photo.
// ShadowFS path (preferred): registers the sidecar as a user_files entry with parent_type='photo'.
// Legacy S3 fallback: writes to activity-photos/{userID}/{hash}.meta.json in the media bucket.
// In both cases the original file remains untouched (Shadow FS principle).
func (h *Handler) writePhotoMetaSidecar(ctx context.Context, userID, photoID, fileHash string, req patchPhotoLocationRequest) {
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

	sidecarBytes, err := json.Marshal(meta)
	if err != nil {
		slog.Error("photo meta sidecar: marshal failed", "error", err)
		return
	}

	sidecarName := fileHash + ".meta.json"

	// ShadowFS path (preferred).
	if h.ShadowFS != nil {
		_, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: "application/json",
			FileName:    sidecarName,
			SizeBytes:   int64(len(sidecarBytes)),
			ParentType:  func() *string { s := "photo"; return &s }(),
			ParentID:    func() *string { s := photoID; return &s }(),
			HasSidecar:  true,
		}, bytes.NewReader(sidecarBytes))
		if putErr != nil {
			slog.Error("photo meta sidecar: ShadowFS.Put failed", "photo_id", photoID, "error", putErr)
			// Fall through to S3 fallback below.
		} else {
			slog.Info("photo meta sidecar written via ShadowFS", "photo_id", photoID)
			return
		}
	}

	// Legacy S3 fallback.
	if h.Storage == nil {
		return
	}
	bucket := mediaBucketName()
	key := fmt.Sprintf("activity-photos/%s/%s.meta.json", userID, fileHash)
	if err := h.Storage.UploadToBucket(ctx, bucket, key, sidecarBytes, "application/json"); err != nil {
		slog.Error("photo meta sidecar: S3 upload failed", "bucket", bucket, "key", key, "error", err)
		return
	}
	slog.Info("photo meta sidecar written via S3", "bucket", bucket, "key", key)
}

func (h *Handler) StreamActivityPhotoPreview(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")
	photo, err := h.DB.GetActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
		return
	}

	// Previews/thumbnails are regenerable cache stored in S3; serve them from S3 when available.
	// When no S3 preview exists and ShadowFS has the original, serve that instead.
	if photo.S3KeyPreview == "" && h.ShadowFS != nil && photo.FileID != nil && *photo.FileID != "" {
		resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, *photo.FileID)
		if resolveErr == nil {
			serveResolvedFile(w, r, resolved)
			return
		}
		slog.Warn("stream photo preview: ShadowFS.Resolve failed, falling back to S3", "file_id", *photo.FileID, "error", resolveErr)
	}

	// Use S3 preview key if available, otherwise fall back to original S3 key.
	if h.Storage == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "storage not configured", r.URL.Path)
		return
	}
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load preview", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// RecomputePhotoPreviews regenerates preview thumbnails for all user photos
// using the current photo_preview_size setting. Runs as a background job.
func (h *Handler) RecomputePhotoPreviews(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load settings", r.URL.Path)
		return
	}
	maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize)

	photos, err := h.DB.ListAllUserPhotos(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_preview_recompute", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("recompute previews: failed to create background job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	if h.Storage == nil && h.ShadowFS == nil {
		_ = h.DB.CompleteBackgroundJob(r.Context(), jobID, false, "")
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"job_id": jobID})
		return
	}

	videoExts := map[string]bool{".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true}

	go func() {
		ctx := context.Background()
		processed := int64(0)
		anyFailed := false

		for _, photo := range photos {
			// ShadowFS path (preferred): read via file_id when available.
			var data []byte
			if h.ShadowFS != nil && photo.FileID != nil && *photo.FileID != "" {
				rc, getErr := h.ShadowFS.Get(ctx, userID, *photo.FileID)
				if getErr == nil {
					data, _ = io.ReadAll(rc)
					rc.Close()
				} else {
					slog.Warn("recompute previews: ShadowFS.Get failed, falling back to S3",
						"photo_id", photo.ID, "file_id", *photo.FileID, "error", getErr)
				}
			}
			// Legacy S3 fallback.
			if data == nil && h.Storage != nil && photo.S3Key != "" {
				var dlErr error
				data, _, dlErr = h.Storage.DownloadFromBucket(ctx, photo.S3Bucket, photo.S3Key)
				if dlErr != nil {
					slog.Warn("recompute previews: S3 download failed",
						"photo_id", photo.ID, "error", dlErr)
				}
			}
			if data == nil {
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			if photo.FileHash == nil {
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			// Detect videos by flag or by S3 key extension.
			isVideo := photo.IsVideo
			if !isVideo {
				dot := strings.LastIndex(photo.S3Key, ".")
				if dot >= 0 {
					isVideo = videoExts[strings.ToLower(photo.S3Key[dot:])]
				}
			}

			if isVideo {
				// For videos: generate a preview JPEG from the midpoint frame.
				ext := ".mp4"
				if dot := strings.LastIndex(photo.S3Key, "."); dot >= 0 {
					ext = photo.S3Key[dot:]
				}
				var midOffset float64
				if meta, err := imageutil.ProbeVideo(data, ext); err == nil && meta.DurationS > 2 {
					midOffset = meta.DurationS / 2
				}
				previewData, err := imageutil.GenerateThumbnailFFmpeg(data, ext, maxW, midOffset)
				if err != nil || previewData == nil {
					processed++
					_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
					continue
				}
				previewKey := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, *photo.FileHash)
				if h.Storage != nil {
					if err := h.Storage.UploadToBucket(ctx, photo.S3Bucket, previewKey, previewData, "image/jpeg"); err != nil {
						slog.Warn("recompute preview: video s3 upload failed", "photo_id", photo.ID, "error", err)
						anyFailed = true
					} else if err := h.DB.UpdateActivityPhotoPreviewKey(ctx, photo.ID, userID, previewKey); err != nil {
						anyFailed = true
					}
				}
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			// Regular images.
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

			s3KeyPreview := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, *photo.FileHash)
			if h.Storage != nil {
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
			}
			processed++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed, "")
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// RecomputePhotoThumbnails regenerates s3_key_thumb for photos where the thumb path
// currently points to a non-image file (e.g. a raw .mp4 path stored during a buggy
// Strava import). It also corrects is_video and content_type for such rows.
// Runs as a background job; returns 202 Accepted immediately.
func (h *Handler) RecomputePhotoThumbnails(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	photos, err := h.DB.ListAllUserPhotos(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	// Filter to photos that need repair:
	// 1. Thumb key ends in a video extension — broken rows from the Strava importer bug.
	// 2. is_video=true but duration_s is NULL — videos missing metadata (e.g. repaired
	//    by an older version of this handler that didn't persist video dimensions).
	var badPhotos []model.ActivityPhoto
	videoExts := map[string]bool{".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true}
	for _, p := range photos {
		dot := strings.LastIndex(p.S3KeyThumb, ".")
		if dot >= 0 {
			ext := strings.ToLower(p.S3KeyThumb[dot:])
			if videoExts[ext] {
				badPhotos = append(badPhotos, p)
				continue
			}
		}
		// Also pick up videos that are already repaired (JPEG thumb) but still lack metadata.
		if p.IsVideo && p.DurationS == nil {
			badPhotos = append(badPhotos, p)
		}
	}

	if len(badPhotos) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "fixed": 0})
		return
	}

	if h.Storage == nil && h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "fixed": 0})
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_thumb_recompute", nil, nil, int64(len(badPhotos)))
	if err != nil {
		slog.Error("recompute thumbnails: failed to create background job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	go func() {
		ctx := context.Background()
		processed := int64(0)
		anyFailed := false

		for _, photo := range badPhotos {
			// Download the original file: ShadowFS (preferred), then S3 fallback.
			var data []byte
			if h.ShadowFS != nil && photo.FileID != nil && *photo.FileID != "" {
				rc, getErr := h.ShadowFS.Get(ctx, userID, *photo.FileID)
				if getErr == nil {
					data, _ = io.ReadAll(rc)
					rc.Close()
				} else {
					slog.Warn("recompute thumbnails: ShadowFS.Get failed, falling back to S3",
						"photo_id", photo.ID, "file_id", *photo.FileID, "error", getErr)
				}
			}
			if data == nil && h.Storage != nil {
				var dlErr error
				data, _, dlErr = h.Storage.DownloadFromBucket(ctx, photo.S3Bucket, photo.S3Key)
				if dlErr != nil {
					slog.Warn("recompute thumbnails: download failed", "photo_id", photo.ID, "key", photo.S3Key, "error", dlErr)
				}
			}
			if data == nil {
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			extDot := strings.LastIndex(photo.S3Key, ".")
			ext := ".mp4" // safe default so ffmpeg always has a hint
			if extDot >= 0 {
				ext = photo.S3Key[extDot:]
			}

			// Generate 200px thumbnail via ffmpeg (first frame).
			thumbData, err := imageutil.GenerateThumbnailFFmpeg(data, ext, 200, 0)
			if err != nil || thumbData == nil {
				slog.Warn("recompute thumbnails: ffmpeg failed", "photo_id", photo.ID, "error", err)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			if photo.FileHash == nil {
				slog.Warn("recompute thumbnails: photo has no file_hash, skipping", "photo_id", photo.ID)
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			// Generate preview (mid-frame) too so both thumbs are fixed in one pass.
			// Capture video metadata to persist duration/dimensions.
			var midOffset float64
			var durationS *float64
			var videoWidth, videoHeight *int
			if meta, err := imageutil.ProbeVideo(data, ext); err == nil {
				if meta.DurationS > 0 {
					d := meta.DurationS
					durationS = &d
					if meta.DurationS > 2 {
						midOffset = meta.DurationS / 2
					}
				}
				if meta.Width > 0 {
					w := meta.Width
					videoWidth = &w
				}
				if meta.Height > 0 {
					h2 := meta.Height
					videoHeight = &h2
				}
			}
			previewData, _ := imageutil.GenerateThumbnailFFmpeg(data, ext, 2560, midOffset)

			thumbKey := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, *photo.FileHash)
			if err := h.Storage.UploadToBucket(ctx, photo.S3Bucket, thumbKey, thumbData, "image/jpeg"); err != nil {
				slog.Warn("recompute thumbnails: s3 thumb upload failed", "photo_id", photo.ID, "error", err)
				anyFailed = true
				processed++
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
				continue
			}

			previewKey := ""
			if previewData != nil {
				pk := fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, *photo.FileHash)
				if err := h.Storage.UploadToBucket(ctx, photo.S3Bucket, pk, previewData, "image/jpeg"); err == nil {
					previewKey = pk
				}
			}

			// Determine corrected content type from the S3 key extension.
			ct := photo.ContentType
			if !strings.HasPrefix(ct, "video/") {
				ct = videoContentTypeFromExt(ext)
			}

			if err := h.DB.UpdateActivityPhotoThumbAndMeta(ctx, photo.ID, userID, thumbKey, previewKey, ct, true, durationS, videoWidth, videoHeight); err != nil {
				slog.Warn("recompute thumbnails: db update failed", "photo_id", photo.ID, "error", err)
				anyFailed = true
			}

			processed++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, processed)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed, "")
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "queued": len(badPhotos)})
}

// videoContentTypeFromExt returns the MIME type for common video extensions.
func videoContentTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	default:
		return "video/mp4"
	}
}

// RecomputePhotoEmbeddings re-queues CLIP embedding computation for all user
// photos that currently have no embedding stored (clip_embedding IS NULL).
// Runs as a background job; returns 202 Accepted immediately.
func (h *Handler) RecomputePhotoEmbeddings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	photos, err := h.DB.GetPhotosWithoutEmbedding(r.Context(), userID)
	if err != nil {
		slog.Error("recompute embeddings: failed to query photos without embedding", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_embed_backfill", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("recompute embeddings: failed to create background job", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	clipFrames := 4
	if s, err := h.DB.GetEnrichmentSettings(r.Context(), userID); err == nil && s.VideoClipFrames > 0 {
		clipFrames = s.VideoClipFrames
	}

	go func() {
		ctx := context.Background()
		published := int64(0)
		anyFailed := false

		for _, photo := range photos {
			if h.Events != nil {
				ev := event.PhotoEmbedEvent{
					PhotoID:         photo.ID,
					UserID:          userID,
					S3Bucket:        photo.S3Bucket,
					S3Key:           photo.S3Key,
					S3KeyPreview:    photo.S3KeyPreview,
					VideoClipFrames: clipFrames,
				}
				if err := h.Events.PublishPhotoEmbed(ev); err != nil {
					slog.Warn("recompute embeddings: publish failed", "photo_id", photo.ID, "err", err)
					anyFailed = true
				}
			}
			published++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, published)
		}

		_ = h.DB.CompleteBackgroundJob(ctx, jobID, anyFailed, "")
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"queued": len(photos),
	})
}

func (h *Handler) ListMapPhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	entries, err := h.DB.ListMapPhotos(r.Context(), userID)
	if err != nil {
		slog.Error("list map photos failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list map photos", r.URL.Path)
		return
	}
	for i := range entries {
		entries[i].ThumbURL = "/photos/" + entries[i].ID + "/thumb"
	}
	httputil.WriteJSON(w, http.StatusOK, entries)
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

// contentTypeFromExtension gibt den MIME-Type anhand der Dateiendung zurück.
// Wird als Fallback genutzt wenn http.DetectContentType() application/octet-stream liefert
// (z.B. HEIC, JXL, einige Video-Formate).
func contentTypeFromExtension(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return "application/octet-stream"
	}
	switch filename[dot:] {
	case ".heic", ".heif":
		return "image/heic"
	case ".jxl":
		return "image/jxl"
	case ".avif":
		return "image/avif"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// fileExtForContentType gibt die passende Dateiendung (inkl. Punkt) für einen Content-Type zurück.
// Wenn der Original-Dateiname eine bekannte Endung hat, wird diese bevorzugt.
func fileExtForContentType(ct, originalFilename string) string {
	// Endung aus Originaldateinamen extrahieren wenn vorhanden
	if dot := strings.LastIndex(originalFilename, "."); dot >= 0 {
		origExt := strings.ToLower(originalFilename[dot:])
		switch origExt {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".tiff", ".tif",
			".heic", ".heif", ".avif", ".jxl",
			".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm":
			return origExt
		}
	}
	// Fallback: aus Content-Type ableiten
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "heic") || strings.Contains(ct, "heif"):
		return ".heic"
	case strings.Contains(ct, "avif"):
		return ".avif"
	case strings.Contains(ct, "jxl"):
		return ".jxl"
	case strings.Contains(ct, "video/mp4"):
		return ".mp4"
	case strings.Contains(ct, "quicktime"):
		return ".mov"
	case strings.Contains(ct, "x-msvideo"):
		return ".avi"
	case strings.Contains(ct, "x-matroska"):
		return ".mkv"
	case strings.Contains(ct, "webm"):
		return ".webm"
	default:
		return ".jpg"
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

// serveResolvedFile writes a ResolvedFile to the HTTP response.
// For local backend: streams the file directly.
// For S3 backend: redirects to the pre-signed URL (media) or proxies (non-media).
func serveResolvedFile(w http.ResponseWriter, r *http.Request, resolved shadowfs.ResolvedFile) {
	ct := resolved.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disposition := "inline"
	if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": resolved.FileName}))

	switch resolved.Type {
	case "local":
		w.Header().Set("Content-Type", ct)
		f, openErr := os.Open(resolved.Path)
		if openErr != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		modTime := time.Time{}
		if fi != nil {
			modTime = fi.ModTime()
		}
		http.ServeContent(w, r, resolved.FileName, modTime, f)
	case "s3":
		if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") {
			http.Redirect(w, r, resolved.URL, http.StatusTemporaryRedirect)
		} else {
			w.Header().Set("Content-Type", ct)
			http.Redirect(w, r, resolved.URL, http.StatusTemporaryRedirect)
		}
	default:
		w.Header().Set("Content-Type", ct)
		http.Redirect(w, r, resolved.URL, http.StatusTemporaryRedirect)
	}
}
