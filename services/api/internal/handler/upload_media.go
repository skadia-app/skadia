package handler

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

const maxMediaUploadSize = 50 << 20 // 50 MB total

// mediaUploadResult is the per-file result returned to the caller.
type mediaUploadResult struct {
	Filename      string  `json:"filename"`
	Status        string  `json:"status"` // "assigned" | "queued" | "duplicate" | "error"
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

		ct := detectUploadedMediaContentType(fileHeader.Filename, data)

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
// Tracker is created at start (status=pending) and finalised based on outcome.
// Synchronous flow — no Processing() transition needed.
func (h *Handler) processMediaImage(r *http.Request, userID, filename, ct string, data []byte) mediaUploadResult {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	sz := int64(len(data))
	tracker := h.DB.NewImportTracker(r.Context(), userID, "upload", filename, &sz, nil, model.ImportTypePhoto)
	tracker.SetFileHash(r.Context(), hash)

	finaliseQueued := func(res mediaUploadResult) mediaUploadResult {
		switch {
		case res.QueueID != nil:
			tracker.Queued(r.Context(), *res.QueueID)
		case res.Error != nil:
			tracker.Error(r.Context(), fmt.Errorf("%s", *res.Error))
		default:
			// Defensive: queueMediaFile should always return either QueueID or Error.
			// Without this branch, the tracker would stay 'pending' forever and keep
			// being polled by the queue indicator.
			tracker.Error(r.Context(), fmt.Errorf("queueMediaFile returned without QueueID or Error"))
		}
		return res
	}

	takenAt, lat, lng, locationSource := extractExifData(data, ct)

	if takenAt == nil {
		return finaliseQueued(h.queueMediaFile(r, userID, filename, ct, data, "image_no_exif", nil))
	}

	matches, err := h.DB.FindActivitiesCoveringTime(r.Context(), userID, *takenAt)
	if err != nil {
		slog.Error("upload media: find activities by time failed", "error", err)
		return finaliseQueued(h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", nil))
	}

	switch len(matches) {
	case 1:
		result := h.autoAssignPhoto(r, userID, filename, ct, data, matches[0], takenAt, lat, lng, locationSource)
		switch result.Status {
		case "assigned":
			if result.ActivityID != nil {
				tracker.Completed(r.Context(), *result.ActivityID)
			}
		case "duplicate":
			tracker.Skipped(r.Context(), "duplicate", result.ActivityID)
		case "error":
			if result.Error != nil {
				tracker.Error(r.Context(), fmt.Errorf("%s", *result.Error))
			}
		}
		return result
	case 0:
		return finaliseQueued(h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", nil))
	default:
		candidateIDs := make([]string, len(matches))
		for i, m := range matches {
			candidateIDs[i] = m.ID
		}
		meta := map[string]any{"candidate_ids": candidateIDs}
		return finaliseQueued(h.queueMediaFile(r, userID, filename, ct, data, "image_ambiguous", meta))
	}
}

// processMediaPDF handles a single PDF file in the media upload flow.
// PDFs always queue (no auto-classification yet); tracker is finalised to 'queued'.
func (h *Handler) processMediaPDF(r *http.Request, userID, filename, ct string, data []byte) mediaUploadResult {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	sz := int64(len(data))
	tracker := h.DB.NewImportTracker(r.Context(), userID, "upload", filename, &sz, nil, model.ImportTypeDocument)
	tracker.SetFileHash(r.Context(), hash)

	res := h.queueMediaFile(r, userID, filename, ct, data, "pdf_untyped", nil)
	if res.QueueID != nil {
		tracker.Queued(r.Context(), *res.QueueID)
	} else if res.Error != nil {
		tracker.Error(r.Context(), fmt.Errorf("%s", *res.Error))
	} else {
		// Defensive: queueMediaFile should always return either QueueID or Error.
		tracker.Error(r.Context(), fmt.Errorf("queueMediaFile returned without QueueID or Error"))
	}
	return res
}

// autoAssignPhoto assigns a photo through the shared activity-photo import pipeline.
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

	if h.ImportService == nil {
		errMsg := "photo import service unavailable"
		return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
	}

	locSrc := ""
	if locationSource != nil {
		locSrc = *locationSource
	}
	result, err := h.ImportService.ProcessPhoto(r.Context(), userID, match.ID, data, importer.PhotoMetadata{
		OriginalFilename: filename,
		ContentType:      ct,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		LocationSource:   locSrc,
	})
	if err != nil {
		slog.Error("upload media: ProcessPhoto failed", "error", err)
		errMsg := "failed to save photo"
		return mediaUploadResult{Filename: filename, Status: "error", Error: &errMsg}
	}

	if result.Status == "duplicate_exact" || result.Status == "duplicate_phash" || result.Status == "replaced" {
		activityID := result.ActivityID
		if activityID == "" {
			activityID = match.ID
		}
		return mediaUploadResult{Filename: filename, Status: "duplicate", ActivityID: &activityID}
	}

	activityID := match.ID
	return mediaUploadResult{
		Filename:      filename,
		Status:        "assigned",
		ActivityID:    &activityID,
		ActivityTitle: match.Title,
	}
}

func detectUploadedMediaContentType(filename string, data []byte) string {
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)
	if ct == "application/octet-stream" || ct == "application/zip" {
		ct = contentTypeFromExtension(strings.ToLower(filename))
	}
	return ct
}

// queueMediaFile stores the file via ShadowFS and creates an import_queue entry for manual review.
func (h *Handler) queueMediaFile(
	r *http.Request,
	userID, filename, ct string,
	data []byte,
	queueType string,
	extraMeta map[string]any,
) mediaUploadResult {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

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
	queueFileID := rec.ID

	meta := map[string]any{}
	if extraMeta != nil {
		maps.Copy(meta, extraMeta)
	}
	meta["file_store_id"] = queueFileID

	fileSize := int64(len(data))
	// ShadowFS path — storage is tracked via metadata file_store_id.
	localBE := "local"
	storageBackend := &localBE
	qItem := database.ImportQueueItem{
		UserID:           userID,
		QueueType:        queueType,
		OriginalFilename: filename,
		ContentType:      ct,
		FileSizeBytes:    &fileSize,
		StorageBackend:   storageBackend,
		Metadata:         meta,
	}

	queueID, err := h.DB.CreateImportQueueItem(r.Context(), qItem)
	if err != nil {
		slog.Error("upload media: create queue item failed", "error", err)
		// Clean up the file we already stored to avoid orphans.
		if delErr := h.ShadowFS.Delete(r.Context(), userID, queueFileID); delErr != nil {
			slog.Warn("upload media: ShadowFS.Delete queue rollback failed", "file_id", queueFileID, "error", delErr)
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
