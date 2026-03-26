package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bep/imagemeta"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
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
		fileHash := fmt.Sprintf("%x", sha256.Sum256(data))
		actIDPtr := &activityID
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    header.Filename,
			SizeBytes:   int64(len(data)),
			FileHash:    fileHash,
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

	// Batch-fetch thumbnail file_ids from user_files in a single query.
	// photoIDs come from a user_id-scoped DB query above — safe to use as batch parameter.
	photoIDs := make([]string, len(photos))
	for i, p := range photos {
		photoIDs[i] = p.ID
	}
	thumbs := h.batchFetchThumbs(r.Context(), photoIDs, userID)

	for i := range photos {
		setPhotoURLs(&photos[i])
		if thumbs != nil {
			photos[i].ThumbFileID = thumbs[photos[i].ID]
		}
	}
	httputil.WriteJSON(w, http.StatusOK, photos)
}

func (h *Handler) DeleteActivityPhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	photoID := chi.URLParam(r, "photoID")
	fileID, err := h.DB.DeleteActivityPhoto(r.Context(), photoID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not found", r.URL.Path)
			return
		}
		slog.Error("delete activity photo failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete photo", r.URL.Path)
		return
	}

	// ShadowFS: delete original + any registered sidecar for this photo.
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

	// ShadowFS path: look up a dedicated thumb file registered against this photo.
	if h.ShadowFS != nil {
		var thumbFileID string
		_ = h.DB.Pool.QueryRow(r.Context(),
			`SELECT id::text FROM user_files
			 WHERE user_id = $1 AND file_type = 'thumb'
			   AND parent_type = 'photo' AND parent_id = $2
			   AND status = 'active'
			 ORDER BY created_at DESC LIMIT 1`,
			userID, photoID,
		).Scan(&thumbFileID)
		if thumbFileID != "" {
			resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, thumbFileID)
			if resolveErr == nil {
				serveResolvedFile(w, r, resolved)
				return
			}
			slog.Warn("stream photo thumb: ShadowFS.Resolve thumb failed",
				"photo_id", photoID, "thumb_file_id", thumbFileID, "error", resolveErr)
		}
		// Fallback: serve original via ShadowFS.
		if photo.FileID != nil && *photo.FileID != "" {
			resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, *photo.FileID)
			if resolveErr == nil {
				serveResolvedFile(w, r, resolved)
				return
			}
			slog.Warn("stream photo thumb: ShadowFS.Resolve original failed",
				"photo_id", photoID, "file_id", *photo.FileID, "error", resolveErr)
		}
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

	httputil.WriteError(w, http.StatusNotFound, "Not Found", "photo not available", r.URL.Path)
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

	// ShadowFS path: look up a dedicated preview file registered against this photo.
	if h.ShadowFS != nil {
		var previewFileID string
		_ = h.DB.Pool.QueryRow(r.Context(),
			`SELECT id::text FROM user_files
			 WHERE user_id = $1 AND file_type = 'preview'
			   AND parent_type = 'photo' AND parent_id = $2
			   AND status = 'active'
			 ORDER BY created_at DESC LIMIT 1`,
			userID, photoID,
		).Scan(&previewFileID)
		if previewFileID != "" {
			resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, previewFileID)
			if resolveErr == nil {
				serveResolvedFile(w, r, resolved)
				return
			}
			slog.Warn("stream photo preview: ShadowFS.Resolve preview failed",
				"photo_id", photoID, "preview_file_id", previewFileID, "error", resolveErr)
		}
		// Fallback: serve original via ShadowFS.
		if photo.FileID != nil && *photo.FileID != "" {
			resolved, resolveErr := h.ShadowFS.Resolve(r.Context(), userID, *photo.FileID)
			if resolveErr == nil {
				serveResolvedFile(w, r, resolved)
				return
			}
			slog.Warn("stream photo preview: ShadowFS.Resolve original failed",
				"photo_id", photoID, "file_id", *photo.FileID, "error", resolveErr)
		}
	}

	httputil.WriteError(w, http.StatusNotFound, "Not Found", "preview not available", r.URL.Path)
}

// RecomputePhotoPreviews regenerates preview images for all user photos via ShadowFS.
// For each photo without a preview in user_files, the original is downloaded, resized,
// and stored as file_type='preview' with parent_type='photo'.
func (h *Handler) RecomputePhotoPreviews(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "fixed": 0, "message": "ShadowFS not configured"})
		return
	}

	photos, err := h.DB.ListAllUserPhotos(r.Context(), userID)
	if err != nil {
		slog.Error("recompute previews: list photos failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_preview_recompute", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("recompute previews: create job failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	shutdown.Go("photo-previews-recompute", func(ctx context.Context) {
		fixed := int64(0)
		for i, photo := range photos {
			if ctx.Err() != nil {
				break
			}
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1))

			// Skip if preview already exists in ShadowFS.
			var exists bool
			_ = h.DB.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM user_files
				 WHERE user_id = $1 AND file_type = 'preview'
				   AND parent_type = 'photo' AND parent_id = $2
				   AND status = 'active')`,
				userID, photo.ID,
			).Scan(&exists)
			if exists {
				continue
			}

			if photo.FileID == nil || *photo.FileID == "" {
				continue
			}

			rc, getErr := h.ShadowFS.Get(ctx, userID, *photo.FileID)
			if getErr != nil {
				slog.Warn("recompute previews: get original failed", "photo_id", photo.ID, "err", getErr)
				continue
			}
			origData, readErr := io.ReadAll(rc)
			rc.Close()
			if readErr != nil {
				continue
			}

			previewData := h.generatePreview(origData, photo.ContentType, photo.OriginalFilename, userID)
			if previewData == nil {
				continue
			}

			hash := ""
			if photo.FileHash != nil {
				hash = *photo.FileHash
			} else {
				hash = fmt.Sprintf("%x", sha256.Sum256(origData))
			}

			_, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "preview",
				ContentType: "image/jpeg",
				FileName:    hash + "_preview.jpg",
				SizeBytes:   int64(len(previewData)),
				FileHash:    hash + "_preview",
				ParentType:  shadowfs.Ptr("photo"),
				ParentID:    shadowfs.Ptr(photo.ID),
			}, bytes.NewReader(previewData))
			if putErr != nil {
				slog.Warn("recompute previews: put preview failed", "photo_id", photo.ID, "err", putErr)
				continue
			}
			fixed++
		}
		_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
		slog.Info("recompute previews done", "fixed", fixed, "total", len(photos))
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "total": len(photos)})
}

// RecomputePhotoThumbnails regenerates thumbnail images for all user photos via ShadowFS.
// For each photo without a thumb in user_files, the original is downloaded, resized to 200px,
// and stored as file_type='thumb' with parent_type='photo'.
func (h *Handler) RecomputePhotoThumbnails(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "fixed": 0, "message": "ShadowFS not configured"})
		return
	}

	photos, err := h.DB.ListAllUserPhotos(r.Context(), userID)
	if err != nil {
		slog.Error("recompute thumbs: list photos failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list photos", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "photo_preview_recompute", nil, nil, int64(len(photos)))
	if err != nil {
		slog.Error("recompute thumbs: create job failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	shutdown.Go("photo-thumbs-recompute", func(ctx context.Context) {
		fixed := int64(0)
		for i, photo := range photos {
			if ctx.Err() != nil {
				break
			}
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1))

			// Skip if thumb already exists in ShadowFS.
			var exists bool
			_ = h.DB.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM user_files
				 WHERE user_id = $1 AND file_type = 'thumb'
				   AND parent_type = 'photo' AND parent_id = $2
				   AND status = 'active')`,
				userID, photo.ID,
			).Scan(&exists)
			if exists {
				continue
			}

			if photo.FileID == nil || *photo.FileID == "" {
				continue
			}

			rc, getErr := h.ShadowFS.Get(ctx, userID, *photo.FileID)
			if getErr != nil {
				slog.Warn("recompute thumbs: get original failed", "photo_id", photo.ID, "err", getErr)
				continue
			}
			origData, readErr := io.ReadAll(rc)
			rc.Close()
			if readErr != nil {
				continue
			}

			thumbData := h.generateThumb(origData, photo.ContentType, photo.OriginalFilename)
			if thumbData == nil {
				continue
			}

			hash := ""
			if photo.FileHash != nil {
				hash = *photo.FileHash
			} else {
				hash = fmt.Sprintf("%x", sha256.Sum256(origData))
			}

			_, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "thumb",
				ContentType: "image/jpeg",
				FileName:    hash + "_thumb.jpg",
				SizeBytes:   int64(len(thumbData)),
				FileHash:    hash + "_thumb",
				ParentType:  shadowfs.Ptr("photo"),
				ParentID:    shadowfs.Ptr(photo.ID),
			}, bytes.NewReader(thumbData))
			if putErr != nil {
				slog.Warn("recompute thumbs: put thumb failed", "photo_id", photo.ID, "err", putErr)
				continue
			}
			fixed++
		}
		_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
		slog.Info("recompute thumbs done", "fixed", fixed, "total", len(photos))
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "total": len(photos)})
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

	shutdown.Go("photo-embeddings-recompute", func(ctx context.Context) {
		published := int64(0)
		anyFailed := false

		for _, photo := range photos {
			if h.Events != nil {
				var embedFileID string
				if photo.FileID != nil {
					embedFileID = *photo.FileID
				}
				ev := event.PhotoEmbedEvent{
					PhotoID:         photo.ID,
					UserID:          userID,
					FileID:          embedFileID,
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
	})

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

// batchFetchThumbs performs a single batch query to fetch thumbnail file_ids for a list
// of photo IDs. The photoIDs MUST originate from a prior user_id-scoped query — never
// pass user-supplied IDs directly. Returns a map of photoID → thumb user_files.id.
func (h *Handler) batchFetchThumbs(ctx context.Context, photoIDs []string, userID string) map[string]string {
	if h.ShadowFS == nil || len(photoIDs) == 0 {
		return nil
	}
	thumbs := make(map[string]string, len(photoIDs))
	rows, err := h.DB.Pool.Query(ctx,
		`SELECT parent_id::text, id::text
		 FROM user_files
		 WHERE parent_type = 'photo' AND parent_id = ANY($1::uuid[])
		   AND file_type = 'thumb' AND user_id = $2
		   AND status = 'active'`,
		photoIDs, userID)
	if err != nil {
		slog.Warn("batchFetchThumbs: query failed", "error", err)
		return thumbs
	}
	defer rows.Close()
	for rows.Next() {
		var parentID, thumbFileID string
		if scanErr := rows.Scan(&parentID, &thumbFileID); scanErr == nil {
			thumbs[parentID] = thumbFileID
		}
	}
	return thumbs
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
// For local backend: streams the file directly via http.ServeContent (Range/ETag).
// For S3 backend: redirects to the pre-signed URL.
//
// NOTE: headers (Cache-Control, Content-Disposition) are set before the switch.
// For local files http.ServeContent sets Content-Type itself; for S3 it is unused
// because http.Redirect sends no body — both are intentional.
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
	default:
		// "s3" and any future types: proxy through the API server because
		// the S3 endpoint (e.g. garage:3900) is only reachable inside the
		// container network — browsers cannot resolve it.
		resp, fetchErr := http.Get(resolved.URL)
		if fetchErr != nil {
			slog.Error("serveResolvedFile: fetch S3 presigned URL failed", "url_prefix", resolved.URL[:min(80, len(resolved.URL))], "error", fetchErr)
			http.Error(w, "upstream fetch failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			slog.Error("serveResolvedFile: S3 returned non-200", "status", resp.StatusCode)
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", ct)
		if resolved.SizeBytes > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resolved.SizeBytes, 10))
		}
		io.Copy(w, resp.Body) //nolint:errcheck
	}
}

// generateThumb creates a 200px thumbnail from image/video data.
func (h *Handler) generateThumb(data []byte, ct, filename string) []byte {
	ext := fileExtForContentType(ct, filename)
	switch {
	case strings.HasPrefix(ct, "video/"):
		if td, err := imageutil.GenerateThumbnailFFmpeg(data, ext, 200, 0); err == nil {
			return td
		}
	case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
		if td, err := imageutil.GenerateThumbnailFFmpeg(data, ext, 200, 0); err == nil {
			return td
		}
	default:
		if td, err := imageutil.Resize(data, 200, 200, 80); err == nil {
			return td
		}
	}
	return nil
}

// generatePreview creates a preview image (default 2048px) from image/video data.
func (h *Handler) generatePreview(data []byte, ct, filename, userID string) []byte {
	ext := fileExtForContentType(ct, filename)
	maxW, maxH := 2048, 2048

	if strings.HasPrefix(ct, "video/") {
		if pd, err := imageutil.GenerateThumbnailFFmpeg(data, ext, 2560, 0); err == nil {
			return pd
		}
		return nil
	}

	if ct == "image/heic" || ct == "image/heif" || ct == "image/jxl" {
		pd, _ := imageutil.GenerateThumbnailFFmpeg(data, ext, maxW, 0)
		return pd
	}
	pd, _ := imageutil.Resize(data, maxW, maxH, 85)
	return pd
}
