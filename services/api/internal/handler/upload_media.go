package handler

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const maxMediaUploadSize = 50 << 20 // 50 MB total

// mediaUploadResult is the per-file result returned to the caller.
type mediaUploadResult struct {
	Filename      string  `json:"filename"`
	Status        string  `json:"status"` // "assigned" | "queued" | "error"
	ActivityID    *string `json:"activity_id,omitempty"`
	ActivityTitle *string `json:"activity_title,omitempty"`
	QueueID       *string `json:"queue_id,omitempty"`
	QueueType     *string `json:"queue_type,omitempty"`
	Error         *string `json:"error,omitempty"`
}

// UploadMedia handles POST /api/v1/upload/media
// Accepts multipart/form-data with field "files" (multiple allowed).
// Images are EXIF-parsed and auto-assigned to activities when possible.
// Files that cannot be auto-assigned go into the import_queue.
func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMediaUploadSize)
	if err := r.ParseMultipartForm(maxMediaUploadSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "request too large or invalid multipart form", r.URL.Path)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no files in request (field name: files)", r.URL.Path)
		return
	}

	results := make([]mediaUploadResult, 0, len(files))

	for _, fileHeader := range files {
		f, err := fileHeader.Open()
		if err != nil {
			errMsg := "failed to open file"
			results = append(results, mediaUploadResult{
				Filename: fileHeader.Filename,
				Status:   "error",
				Error:    &errMsg,
			})
			continue
		}

		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			errMsg := "failed to read file"
			results = append(results, mediaUploadResult{
				Filename: fileHeader.Filename,
				Status:   "error",
				Error:    &errMsg,
			})
			continue
		}

		ct := fileHeader.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(data)
		}

		var res mediaUploadResult
		switch {
		case strings.HasPrefix(ct, "image/"):
			res = h.processMediaImage(r, userID, fileHeader.Filename, ct, data)
		case ct == "application/pdf":
			res = h.processMediaPDF(r, userID, fileHeader.Filename, ct, data)
		default:
			errMsg := "unsupported file type: " + ct
			res = mediaUploadResult{
				Filename: fileHeader.Filename,
				Status:   "error",
				Error:    &errMsg,
			}
		}

		results = append(results, res)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

// processMediaImage handles a single image file in the media upload flow.
func (h *Handler) processMediaImage(r *http.Request, userID, filename, ct string, data []byte) mediaUploadResult {
	takenAt, lat, lng, locationSource := extractExifData(data, ct)

	if takenAt == nil {
		// No EXIF timestamp — queue for manual review
		return h.queueMediaFile(r, userID, filename, ct, data, "image_no_exif", nil)
	}

	// Find activities whose time window covers this timestamp
	matches, err := h.DB.FindActivitiesCoveringTime(r.Context(), userID, *takenAt)
	if err != nil {
		slog.Error("upload media: find activities by time failed", "error", err)
		return h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", nil)
	}

	switch len(matches) {
	case 1:
		// Exactly one match — auto-assign
		return h.autoAssignPhoto(r, userID, filename, ct, data, matches[0], takenAt, lat, lng, locationSource)
	case 0:
		// No match — queue with no candidates
		return h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", nil)
	default:
		// Multiple matches — queue with candidate IDs
		candidateIDs := make([]string, len(matches))
		for i, m := range matches {
			candidateIDs[i] = m.ID
		}
		meta := map[string]any{"candidate_ids": candidateIDs}
		return h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", meta)
	}
}

// processMediaPDF handles a single PDF file in the media upload flow.
func (h *Handler) processMediaPDF(r *http.Request, userID, filename, ct string, data []byte) mediaUploadResult {
	return h.queueMediaFile(r, userID, filename, ct, data, "pdf_untyped", nil)
}

// autoAssignPhoto stores a photo directly into the activity_photos table.
func (h *Handler) autoAssignPhoto(
	r *http.Request,
	userID, filename, ct string,
	data []byte,
	match database.ActivityTimeMatch,
	takenAt *time.Time,
	lat, lng *float64,
	locationSource *string,
) mediaUploadResult {
	// Try track interpolation if no GPS in EXIF
	if lat == nil && takenAt != nil {
		lat, lng = h.interpolateFromTrack(r.Context(), match.ID, userID, *takenAt)
		if lat != nil {
			ls := "activity_track"
			locationSource = &ls
		}
	}

	thumbData, _ := imageutil.Resize(data, 200, 200, 80)

	ext := imageExtFromContentType(ct)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Duplikat-Prüfung: bereits vorhandene Fotos überspringen
	if exists, err := h.DB.ActivityPhotoExistsByHash(r.Context(), userID, hash); err == nil && exists {
		return mediaUploadResult{Filename: filename, Status: "duplicate"}
	}

	mediaBucket := mediaBucketName()
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, hash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, hash)

	var photoFileID string
	if h.ShadowFS != nil {
		actParent := "activity"
		actID := match.ID
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    filename,
			SizeBytes:   int64(len(data)),
			FileHash:    hash,
			ParentType:  &actParent,
			ParentID:    &actID,
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Error("upload media: ShadowFS.Put photo failed", "error", putErr)
			errMsg := "failed to store photo"
			return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
		}
		photoFileID = rec.ID
		// Thumbnail is stored after DB insert (see below) so we can use parent_type='photo'.
	} else if h.Storage != nil {
		if err := h.Storage.UploadToBucket(r.Context(), mediaBucket, s3Key, data, ct); err != nil {
			slog.Error("upload media: s3 upload photo failed", "error", err)
			errMsg := "failed to store photo"
			return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
		}
		if thumbData != nil {
			_ = h.Storage.UploadToBucket(r.Context(), mediaBucket, s3KeyThumb, thumbData, "image/jpeg")
		} else {
			s3KeyThumb = s3Key
		}
	}

	fileSize := int64(len(data))
	activityID := match.ID
	photo, err := h.DB.InsertActivityPhoto(r.Context(), model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &activityID,
		FileID:           &photoFileID,
		S3Bucket:         mediaBucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		OriginalFilename: filename,
		ContentType:      ct,
		FileSizeBytes:    fileSize,
		FileHash:         &hash,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		LocationSource:   locationSource,
	})
	if err != nil {
		slog.Error("upload media: insert photo failed", "error", err)
		// Clean up the file we already stored to avoid orphans.
		if photoFileID != "" && h.ShadowFS != nil {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, photoFileID); delErr != nil {
				slog.Warn("upload media: ShadowFS.Delete photo rollback failed", "file_id", photoFileID, "error", delErr)
			}
		} else if h.Storage != nil {
			_ = h.Storage.DeleteFromBucket(r.Context(), mediaBucket, s3Key)
			if s3KeyThumb != s3Key {
				_ = h.Storage.DeleteFromBucket(r.Context(), mediaBucket, s3KeyThumb)
			}
		}
		errMsg := "failed to save photo"
		return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
	}

	// Store thumbnail in ShadowFS with parent_type='photo' (after insert so we have the photo ID).
	if thumbData != nil && h.ShadowFS != nil && photo.ID != "" {
		thumbName := fmt.Sprintf("%s_thumb.jpg", hash)
		photoParent := "photo"
		photoID := photo.ID
		// FileHash uses a synthetic "hash_thumb" value (not a real SHA256 of the thumbnail
		// data) because thumbnails are regenerable derivatives. The "_thumb" suffix makes
		// the value deterministic and unique within the user_files registry.
		_, _ = h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: "image/jpeg",
			FileName:    thumbName,
			SizeBytes:   int64(len(thumbData)),
			FileHash:    hash + "_thumb",
			ParentType:  &photoParent,
			ParentID:    &photoID,
		}, bytes.NewReader(thumbData))
	}

	return mediaUploadResult{
		Filename:      filename,
		Status:        "assigned",
		ActivityID:    &activityID,
		ActivityTitle: match.Title,
	}
}

// queueMediaFile stores the file via ShadowFS (preferred) or temp S3 (fallback)
// and creates an import_queue entry for manual review.
func (h *Handler) queueMediaFile(
	r *http.Request,
	userID, filename, ct string,
	data []byte,
	queueType string,
	extraMeta map[string]any,
) mediaUploadResult {
	// Determine extension for legacy S3 key
	var ext string
	if ct == "application/pdf" {
		ext = ".pdf"
	} else {
		ext = imageExtFromContentType(ct)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Duplikat-Prüfung für Fotos (nicht für PDFs)
	if ct != "application/pdf" {
		if exists, err := h.DB.ActivityPhotoExistsByHash(r.Context(), userID, hash); err == nil && exists {
			return mediaUploadResult{Filename: filename, Status: "duplicate"}
		}
	}

	tempBucket := mediaBucketName()
	s3Key := fmt.Sprintf("media-temp/%s/%s%s", userID, hash, ext)

	// Upload via ShadowFS (preferred) or legacy S3 (fallback).
	var queueFileID string
	if h.ShadowFS != nil {
		rec, putErr := h.ShadowFS.Put(r.Context(), shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    filename,
			SizeBytes:   int64(len(data)),
			FileHash:    hash,
			Tags:        map[string]string{"queue_type": queueType},
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Error("upload media: ShadowFS.Put temp failed", "error", putErr)
			errMsg := "failed to store file"
			return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
		}
		queueFileID = rec.ID
	} else if h.Storage != nil {
		if err := h.Storage.UploadToBucket(r.Context(), tempBucket, s3Key, data, ct); err != nil {
			slog.Error("upload media: s3 temp upload failed", "error", err)
			errMsg := "failed to store file"
			return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
		}
	}

	meta := map[string]any{}
	if extraMeta != nil {
		maps.Copy(meta, extraMeta)
	}
	if queueFileID != "" {
		meta["file_store_id"] = queueFileID
	}

	fileSize := int64(len(data))
	qItem := database.ImportQueueItem{
		UserID:           userID,
		QueueType:        queueType,
		OriginalFilename: filename,
		ContentType:      ct,
		FileSizeBytes:    &fileSize,
		S3Bucket:         &tempBucket,
		S3Key:            &s3Key,
		Metadata:         meta,
	}

	queueID, err := h.DB.CreateImportQueueItem(r.Context(), qItem)
	if err != nil {
		slog.Error("upload media: create queue item failed", "error", err)
		// Clean up the file we already stored to avoid orphans.
		if queueFileID != "" && h.ShadowFS != nil {
			if delErr := h.ShadowFS.Delete(r.Context(), userID, queueFileID); delErr != nil {
				slog.Warn("upload media: ShadowFS.Delete queue rollback failed", "file_id", queueFileID, "error", delErr)
			}
		} else if h.Storage != nil {
			_ = h.Storage.DeleteFromBucket(r.Context(), tempBucket, s3Key)
		}
		errMsg := "failed to create queue entry"
		return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
	}

	return mediaUploadResult{
		Filename:  filename,
		Status:    "queued",
		QueueID:   &queueID,
		QueueType: &queueType,
	}
}

// imageExtFromContentType returns the file extension for an image content type.
// Exact matches are required to prevent ambiguous content-type substrings from
// matching unrelated types (e.g. "application/x-png" should not match "png").
func imageExtFromContentType(ct string) string {
	// Strip optional parameters (e.g. "image/jpeg; charset=...").
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	switch ct {
	case "image/png":
		return ".png"
	case "image/heic", "image/heif":
		return ".heic"
	case "image/avif":
		return ".avif"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/tiff":
		return ".tiff"
	case "image/bmp", "image/x-bmp", "image/x-ms-bmp":
		return ".bmp"
	default:
		return ".jpg"
	}
}

// mediaBucketName returns the configured S3 media bucket name.
func mediaBucketName() string {
	if b := os.Getenv("GARAGE_MEDIA_BUCKET"); b != "" {
		return b
	}
	return "skadia-media"
}
