package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/phash"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

const mediaBucket = "skadia-media"
const importQueuePrefix = "import-queue"

// ProcessPhoto centralizes all photo import logic: SHA256 exact dedup, pHash near-duplicate
// detection, thumbnail/preview generation, S3 upload, and DB insertion.
// It mirrors the logic of handler.UploadActivityPhoto but accepts raw bytes + PhotoMetadata
// instead of an HTTP request.
func (s *Service) ProcessPhoto(ctx context.Context, userID, activityID string, photoBytes []byte, meta PhotoMetadata) (*PhotoResult, error) {
	// Load enrichment settings once — used for pHash threshold, preview size, and clip frames.
	enrichSettings, err := s.DB.GetEnrichmentSettings(ctx, userID)
	if err != nil {
		slog.Warn("ProcessPhoto: get enrichment settings failed", "error", err)
	}

	// 1. SHA256 exact dedup
	hashArr := sha256.Sum256(photoBytes)
	fileHash := hex.EncodeToString(hashArr[:])

	if exists, err := s.DB.ActivityPhotoExistsByHashAndActivity(ctx, userID, fileHash, activityID); err == nil && exists {
		return &PhotoResult{Status: "duplicate_exact"}, nil
	}

	// 2. Determine content type: trust meta.ContentType if provided; otherwise sniff bytes.
	ct := meta.ContentType
	if ct == "" {
		sniff := photoBytes
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		ct = http.DetectContentType(sniff)
		// Fallback for HEIC/JXL/video that returns application/octet-stream
		if ct == "application/octet-stream" || ct == "application/zip" {
			ct = contentTypeFromFilename(meta.OriginalFilename)
		}
	}

	isVideo := strings.HasPrefix(ct, "video/")

	// 3. Compute pHash
	var pHashVal int64
	var pHashOk bool
	if isVideo {
		durationS := 30.0
		if meta.DurationS != nil {
			durationS = *meta.DurationS
		}
		if h, err := phash.ComputeVideoHash(photoBytes, durationS); err == nil {
			pHashVal = int64(h)
			pHashOk = true
		} else {
			slog.Warn("importer: video phash failed", "err", err)
		}
	} else {
		if h, err := phash.ComputeFromBytes(photoBytes); err == nil {
			pHashVal = int64(h)
			pHashOk = true
		} else {
			// Fallback for HEIC/JXL via ffmpeg
			if h2, err2 := phash.ComputeViaFFmpeg(photoBytes); err2 == nil {
				pHashVal = int64(h2)
				pHashOk = true
			} else {
				slog.Warn("importer: phash computation failed, importing with phash=NULL", "err", err)
			}
		}
	}

	// 4. pHash duplicate check against existing photos of this activity
	if pHashOk && activityID != "" {
		existing, err := s.DB.GetActivityPhotoHashes(ctx, activityID)
		if err == nil && len(existing) > 0 {
			pHashThreshold := 3
			if enrichSettings != nil && enrichSettings.PhashThresholdBits > 0 {
				pHashThreshold = enrichSettings.PhashThresholdBits
			}
			hashes := make([]uint64, len(existing))
			for i, e := range existing {
				if e.PHash != nil {
					hashes[i] = uint64(*e.PHash)
				}
			}
			bestIdx, _ := phash.FindBestMatch(uint64(pHashVal), hashes, pHashThreshold)
			if bestIdx >= 0 {
				matchedPhoto := existing[bestIdx]
				// Resolution check: replace if new image is strictly larger
				var newW, newH int
				if img, _, err := image.Decode(bytes.NewReader(photoBytes)); err == nil {
					newW = img.Bounds().Dx()
					newH = img.Bounds().Dy()
				}
				existingPixels := 0
				if matchedPhoto.Width != nil && matchedPhoto.Height != nil {
					existingPixels = *matchedPhoto.Width * *matchedPhoto.Height
				}
				newPixels := newW * newH

				// Sanitize location_source to values allowed by the DB constraint.
				locSrc := meta.LocationSource
				switch locSrc {
				case "exif", "activity_track", "manual":
				default:
					locSrc = ""
				}

				if newPixels > existingPixels && existingPixels > 0 {
					slog.Info("importer: phash duplicate with higher resolution, replacing",
						"photo", matchedPhoto.ID, "old_pixels", existingPixels, "new_pixels", newPixels)

					// Update file metadata; fill EXIF fields only when existing lacks them (COALESCE: existing wins).
					if _, execErr := s.DB.Pool.Exec(ctx,
						`UPDATE activity_photos
						 SET file_size_bytes = $1, file_hash = $2, width = $3, height = $4, phash = $5,
						     taken_at        = COALESCE(taken_at, $6::timestamptz),
						     lat             = COALESCE(lat, $7::double precision),
						     lng             = COALESCE(lng, $8::double precision),
						     location_source = COALESCE(location_source, NULLIF($9, ''))
						 WHERE id = $10`,
						int64(len(photoBytes)), fileHash, newW, newH, pHashVal,
						meta.TakenAt, meta.Lat, meta.Lng, locSrc,
						matchedPhoto.ID,
					); execErr != nil {
						slog.Error("importer: replace photo metadata failed", "photo", matchedPhoto.ID, "err", execErr)
						return nil, fmt.Errorf("replace photo metadata in DB: %w", execErr)
					}
					return &PhotoResult{PhotoID: matchedPhoto.ID, Status: "replaced", PHash: pHashVal}, nil
				}
				// EXIF enrichment: new photo has timestamp or GPS that the existing photo lacks.
				// Returns "duplicate_phash" so callers do not treat this as a new file import.
				if (meta.TakenAt != nil && matchedPhoto.TakenAt == nil) || (meta.Lat != nil && matchedPhoto.Lat == nil) {
					slog.Info("importer: phash duplicate, enriching with EXIF metadata", "photo", matchedPhoto.ID)
					if _, execErr := s.DB.Pool.Exec(ctx,
						`UPDATE activity_photos
						 SET taken_at        = COALESCE(taken_at, $1::timestamptz),
						     lat             = COALESCE(lat, $2::double precision),
						     lng             = COALESCE(lng, $3::double precision),
						     location_source = COALESCE(location_source, NULLIF($4, ''))
						 WHERE id = $5`,
						meta.TakenAt, meta.Lat, meta.Lng, locSrc,
						matchedPhoto.ID,
					); execErr != nil {
						slog.Error("importer: enrich photo EXIF failed", "photo", matchedPhoto.ID, "err", execErr)
						return nil, fmt.Errorf("enrich photo EXIF in DB: %w", execErr)
					}
					return &PhotoResult{PhotoID: matchedPhoto.ID, Status: "duplicate_phash", PHash: pHashVal}, nil
				}
				return &PhotoResult{PhotoID: matchedPhoto.ID, Status: "duplicate_phash", PHash: pHashVal}, nil
			}
		}
	}

	// 5. No duplicate — insert new photo.

	// Determine file extension for S3 key
	ext := fileExtFromContentType(ct, meta.OriginalFilename)

	// Video metadata via ffprobe
	var videoMeta *imageutil.VideoMeta
	if isVideo {
		if vm, err := imageutil.ProbeVideo(photoBytes, ext); err == nil {
			videoMeta = &vm
		} else {
			slog.Warn("importer: ffprobe failed", "filename", meta.OriginalFilename, "error", err)
		}
	}

	// Thumbnail (200px) — first frame for video/HEIC/JXL, otherwise imageutil.Resize
	var thumbData []byte
	switch {
	case isVideo:
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		} else {
			slog.Warn("importer: ffmpeg video thumbnail failed", "filename", meta.OriginalFilename, "error", err)
		}
	case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		} else {
			slog.Warn("importer: ffmpeg image thumbnail failed", "ct", ct, "error", err)
		}
	default:
		thumbData, _ = imageutil.Resize(photoBytes, 200, 200, 80)
	}

	// Preview thumbnail (2048px for images; mid-frame for video)
	var previewData []byte
	s3KeyPreview := ""
	if isVideo {
		var midOffset float64
		if videoMeta != nil && videoMeta.DurationS > 2 {
			midOffset = videoMeta.DurationS / 2
		}
		if pd, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 2560, midOffset); err == nil {
			previewData = pd
		} else {
			slog.Warn("importer: ffmpeg video preview failed", "filename", meta.OriginalFilename, "error", err)
		}
	} else {
		// Load user preview size setting; default to 2048px on error.
		maxW, maxH := 2048, 2048
		if enrichSettings != nil {
			if w, h := imageutil.PreviewSizePixels(enrichSettings.PhotoPreviewSize); w > 0 {
				maxW, maxH = w, h
			}
		}
		if ct == "image/heic" || ct == "image/heif" || ct == "image/jxl" {
			previewData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, maxW, 0)
		} else {
			previewData, _ = imageutil.Resize(photoBytes, maxW, maxH, 85)
		}
	}

	// Decode image dimensions for width/height columns
	var imgWidth, imgHeight int
	if !isVideo {
		if img, _, err := image.Decode(bytes.NewReader(photoBytes)); err == nil {
			imgWidth = img.Bounds().Dx()
			imgHeight = img.Bounds().Dy()
		}
	} else if videoMeta != nil {
		imgWidth = videoMeta.Width
		imgHeight = videoMeta.Height
	}

	// S3 paths
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, fileHash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, fileHash)

	// ShadowFS path (preferred): store original via ShadowFS.
	var shadowFileID string
	if s.ShadowFS != nil {
		parentType := "activity"
		rec, putErr := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    meta.OriginalFilename,
			SizeBytes:   int64(len(photoBytes)),
			FileHash:    fileHash,
			ParentType:  &parentType,
			ParentID:    shadowfs.Ptr(activityID),
		}, bytes.NewReader(photoBytes))
		if putErr != nil {
			slog.Warn("importer: ShadowFS.Put photo failed, falling back to S3", "err", putErr)
		} else {
			shadowFileID = rec.ID
		}
	}

	if shadowFileID == "" {
		return nil, fmt.Errorf("no storage available for photo upload")
	}

	// Thumb/preview are stored via ShadowFS after DB insert (see below)
	// so we can use parent_type='photo' with the photo ID. Legacy S3 paths
	// are still computed for the DB model but no longer uploaded.
	if thumbData == nil {
		s3KeyThumb = s3Key
	}
	if previewData != nil {
		s3KeyPreview = fmt.Sprintf("activity-photos/%s/%s_preview.jpg", userID, fileHash)
	}

	// Build DB model
	var actIDPtr *string
	if activityID != "" {
		actIDPtr = &activityID
	}
	hashCopy := fileHash
	photo := model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       actIDPtr,
		S3Bucket:         mediaBucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		S3KeyPreview:     s3KeyPreview,
		OriginalFilename: meta.OriginalFilename,
		ContentType:      ct,
		FileSizeBytes:    int64(len(photoBytes)),
		FileHash:         &hashCopy,
		TakenAt:          meta.TakenAt,
		IsVideo:          isVideo,
	}
	if shadowFileID != "" {
		photo.FileID = &shadowFileID
	}

	// Location fields from metadata
	if meta.Lat != nil {
		photo.Lat = meta.Lat
		photo.Lng = meta.Lng
		if meta.LocationSource != "" {
			ls := meta.LocationSource
			photo.LocationSource = &ls
		}
	}

	// pHash
	if pHashOk {
		photo.PHash = &pHashVal
	}

	// Image dimensions
	if imgWidth > 0 {
		photo.Width = &imgWidth
		photo.Height = &imgHeight
	}

	// Video metadata
	if videoMeta != nil {
		photo.DurationS = &videoMeta.DurationS
		photo.VideoWidth = &videoMeta.Width
		photo.VideoHeight = &videoMeta.Height
	} else if meta.DurationS != nil {
		// Use caller-supplied video metadata as fallback
		photo.DurationS = meta.DurationS
		photo.VideoWidth = meta.VideoWidth
		photo.VideoHeight = meta.VideoHeight
	}

	saved, err := s.DB.InsertActivityPhoto(ctx, photo)
	if err != nil {
		return nil, fmt.Errorf("insert activity photo: %w", err)
	}

	// Store thumb/preview via ShadowFS with parent_type='photo' (needs photo ID from insert).
	if s.ShadowFS != nil && saved.ID != "" {
		if thumbData != nil {
			_, thumbErr := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "thumb",
				ContentType: "image/jpeg",
				FileName:    fileHash + "_thumb.jpg",
				SizeBytes:   int64(len(thumbData)),
				FileHash:    fileHash + "_thumb",
				ParentType:  shadowfs.Ptr("photo"),
				ParentID:    shadowfs.Ptr(saved.ID),
			}, bytes.NewReader(thumbData))
			if thumbErr != nil {
				slog.Warn("importer: ShadowFS.Put thumb failed", "photo_id", saved.ID, "err", thumbErr)
			}
		}
		if previewData != nil {
			_, previewErr := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "preview",
				ContentType: "image/jpeg",
				FileName:    fileHash + "_preview.jpg",
				SizeBytes:   int64(len(previewData)),
				FileHash:    fileHash + "_preview",
				ParentType:  shadowfs.Ptr("photo"),
				ParentID:    shadowfs.Ptr(saved.ID),
			}, bytes.NewReader(previewData))
			if previewErr != nil {
				slog.Warn("importer: ShadowFS.Put preview failed", "photo_id", saved.ID, "err", previewErr)
			}
		}
	}

	// Publish CLIP embed event (best-effort)
	if s.Events != nil {
		clipFrames := 4
		if enrichSettings != nil && enrichSettings.VideoClipFrames > 0 {
			clipFrames = enrichSettings.VideoClipFrames
		}
		var photoFileID string
		if saved.FileID != nil {
			photoFileID = *saved.FileID
		}
		ev := event.PhotoEmbedEvent{
			PhotoID:         saved.ID,
			UserID:          userID,
			FileID:          photoFileID,
			ContentType:     ct,
			VideoClipFrames: clipFrames,
		}
		if err := s.Events.PublishPhotoEmbed(ev); err != nil {
			slog.Warn("importer: failed to publish photo embed event", "photo_id", saved.ID, "err", err)
		}
	}

	result := &PhotoResult{
		PhotoID: saved.ID,
		Status:  "created",
	}
	if pHashOk {
		result.PHash = pHashVal
	}
	return result, nil
}

// ProcessOrphanPhoto handles a photo without a known activity.
// If the photo's TakenAt timestamp falls within a known activity's window, it is
// immediately processed via ProcessPhoto (status: "matched").
// Otherwise the photo is stored in S3 under import-queue/{userID}/{hash} and a
// corresponding import_queue record (queue_type="image_no_match") is created
// (status: "queued").
func (s *Service) ProcessOrphanPhoto(ctx context.Context, userID string, photoBytes []byte, meta PhotoMetadata) (*OrphanResult, error) {
	// 1. Time-based activity lookup
	if meta.TakenAt != nil {
		activityID, err := s.DB.FindActivityIDAtTime(ctx, userID, *meta.TakenAt)
		if err != nil {
			slog.Warn("importer: FindActivityIDAtTime failed", "err", err)
		}
		if activityID != "" {
			result, err := s.ProcessPhoto(ctx, userID, activityID, photoBytes, meta)
			if err != nil {
				return nil, fmt.Errorf("process orphan photo (matched): %w", err)
			}
			return &OrphanResult{Status: "matched", ActivityID: activityID, QueueEntryID: result.PhotoID}, nil
		}
	}

	// 2. No match — store and enqueue for manual review
	hashArr := sha256.Sum256(photoBytes)
	fileHash := hex.EncodeToString(hashArr[:])

	// Determine content type
	ct := meta.ContentType
	if ct == "" {
		sniff := photoBytes
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		ct = http.DetectContentType(sniff)
		if ct == "application/octet-stream" || ct == "application/zip" {
			ct = contentTypeFromFilename(meta.OriginalFilename)
		}
	}

	// Generate thumbnail (200px) for the queue preview
	isVideo := strings.HasPrefix(ct, "video/")
	var thumbData []byte
	ext := fileExtFromContentType(ct, meta.OriginalFilename)
	switch {
	case isVideo:
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		}
	case ct == "image/heic" || ct == "image/heif" || ct == "image/jxl":
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		}
	default:
		thumbData, _ = imageutil.Resize(photoBytes, 200, 200, 80)
	}

	fileSize := int64(len(photoBytes))
	s3Key := fmt.Sprintf("%s/%s/%s", importQueuePrefix, userID, fileHash)
	queueMeta := map[string]any{
		"filename": meta.OriginalFilename,
	}

	if s.ShadowFS != nil {
		// ShadowFS path: store original via ShadowFS (assigned to _temp internally).
		rec, putErr := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "media",
			ContentType: ct,
			FileName:    meta.OriginalFilename,
			SizeBytes:   fileSize,
			FileHash:    fileHash,
			Tags:        map[string]string{"queue_type": "image_no_match"},
		}, bytes.NewReader(photoBytes))
		if putErr != nil {
			return nil, fmt.Errorf("shadowfs put orphan photo: %w", putErr)
		}
		queueMeta["file_store_id"] = rec.ID

		// Store thumbnail via ShadowFS (best-effort).
		if thumbData != nil {
			thumbName := fileHash + "_thumb.jpg"
			thumbRec, thumbErr := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "thumb",
				ContentType: "image/jpeg",
				FileName:    thumbName,
				SizeBytes:   int64(len(thumbData)),
				FileHash:    fileHash + "_thumb",
				Tags:        map[string]string{"queue_type": "image_no_match_thumb"},
			}, bytes.NewReader(thumbData))
			if thumbErr == nil {
				queueMeta["file_store_id_thumb"] = thumbRec.ID
			} else {
				slog.Warn("importer: orphan thumb ShadowFS.Put failed", "err", thumbErr)
			}
		}
	} else {
		return nil, fmt.Errorf("no storage backend configured")
	}

	s3BE := "s3"
	queueItem := database.ImportQueueItem{
		UserID:           userID,
		QueueType:        "image_no_match",
		OriginalFilename: meta.OriginalFilename,
		ContentType:      ct,
		FileSizeBytes:    &fileSize,
		StorageBackend:   &s3BE,
		StoragePath:      &s3Key,
		Metadata:         queueMeta,
	}
	entryID, err := s.DB.CreateImportQueueItem(ctx, queueItem)
	if err != nil {
		return nil, fmt.Errorf("create import queue item: %w", err)
	}

	return &OrphanResult{Status: "queued", QueueEntryID: entryID}, nil
}

// ResolveQueuedPhoto assigns a queued image_no_match item to a specific activity.
// It loads the file from ShadowFS or S3, runs ProcessPhoto, marks the queue item
// as resolved, and cleans up the temporary files.
func (s *Service) ResolveQueuedPhoto(ctx context.Context, userID, queueEntryID, targetActivityID string) (*PhotoResult, error) {
	// 1. Load and validate queue item
	item, err := s.DB.GetImportQueueItem(ctx, queueEntryID, userID)
	if err != nil || item == nil {
		return nil, fmt.Errorf("queue item not found: %w", err)
	}
	if item.QueueType != "image_no_match" {
		return nil, fmt.Errorf("queue item is not an image_no_match entry (got %q)", item.QueueType)
	}
	// Ownership already enforced by GetImportQueueItem (WHERE user_id = $2)

	// 2. Load photo bytes — ShadowFS preferred, S3 as fallback.
	var photoBytes []byte
	ct := item.ContentType

	shadowFSID, hasShadowFSID := item.Metadata["file_store_id"].(string)
	if hasShadowFSID && shadowFSID != "" && s.ShadowFS != nil {
		rc, getErr := s.ShadowFS.Get(ctx, userID, shadowFSID)
		if getErr != nil {
			return nil, fmt.Errorf("shadowfs get queued photo: %w", getErr)
		}
		photoBytes, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("shadowfs read queued photo: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no storage backend configured")
	}

	// 3. Process as a normal activity photo
	meta := PhotoMetadata{
		OriginalFilename: item.OriginalFilename,
		ContentType:      ct,
	}
	result, err := s.ProcessPhoto(ctx, userID, targetActivityID, photoBytes, meta)
	if err != nil {
		return nil, fmt.Errorf("process queued photo: %w", err)
	}

	if result.Status == "error" {
		return result, nil
	}

	// 4. Cleanup: mark resolved + delete temp files
	if resolveErr := s.DB.ResolveImportQueueItem(ctx, queueEntryID, userID); resolveErr != nil {
		slog.Warn("importer: ResolveImportQueueItem failed", "id", queueEntryID, "err", resolveErr)
	}

	if hasShadowFSID && shadowFSID != "" && s.ShadowFS != nil {
		if delErr := s.ShadowFS.Delete(ctx, userID, shadowFSID); delErr != nil {
			slog.Warn("importer: ShadowFS.Delete orphan photo failed", "id", shadowFSID, "err", delErr)
		}
		if thumbFileID, ok := item.Metadata["file_store_id_thumb"].(string); ok && thumbFileID != "" {
			if delErr := s.ShadowFS.Delete(ctx, userID, thumbFileID); delErr != nil {
				slog.Warn("importer: ShadowFS.Delete orphan thumb failed", "id", thumbFileID, "err", delErr)
			}
		}
	}

	return result, nil
}

// contentTypeFromFilename derives a MIME type from a filename's extension.
// Used as fallback when http.DetectContentType returns application/octet-stream.
func contentTypeFromFilename(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return "application/octet-stream"
	}
	switch strings.ToLower(filename[dot:]) {
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

// fileExtFromContentType returns the canonical file extension (with dot) for a content type.
// Prefers the extension from the original filename when available.
func fileExtFromContentType(ct, originalFilename string) string {
	if dot := strings.LastIndex(originalFilename, "."); dot >= 0 {
		origExt := strings.ToLower(originalFilename[dot:])
		switch origExt {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".tiff", ".tif",
			".heic", ".heif", ".avif", ".jxl",
			".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm":
			return origExt
		}
	}
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
