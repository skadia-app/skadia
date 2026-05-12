package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
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
	"github.com/jackc/pgx/v5/pgconn"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
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

	locSrc := sanitizePhotoLocationSource(meta.LocationSource)
	if existing, err := s.DB.GetActivityPhotoByHash(ctx, userID, fileHash); err == nil && existing != nil {
		if err := s.enrichActivityPhotoMetadata(ctx, existing.ID, userID, meta, locSrc); err != nil {
			return nil, err
		}
		return &PhotoResult{PhotoID: existing.ID, ActivityID: existing.ActivityID, Status: "duplicate_exact"}, nil
	} else if err != nil {
		return nil, fmt.Errorf("lookup exact activity photo duplicate: %w", err)
	}

	// 2. Determine content type: trust meta.ContentType if provided; otherwise sniff bytes.
	ct := meta.ContentType
	if ct == "" {
		sniff := photoBytes
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		ct = http.DetectContentType(sniff)
		// Fallback for HEIC/JXL/AVIF/video that returns application/octet-stream.
		if ct == "application/octet-stream" || ct == "application/zip" {
			ct = contentTypeFromFilename(meta.OriginalFilename)
		}
	}
	ext := fileExtFromContentType(ct, meta.OriginalFilename)

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
			if h2, err2 := phash.ComputeViaFFmpegWithExt(photoBytes, ext); err2 == nil {
				pHashVal = int64(h2)
				pHashOk = true
			} else {
				slog.Warn("importer: phash computation failed, importing with phash=NULL", "err", err)
			}
		}
	}

	unlockDedup, err := s.lockUserPhotoDedup(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer unlockDedup()

	if existing, err := s.DB.GetActivityPhotoByHash(ctx, userID, fileHash); err == nil && existing != nil {
		if err := s.enrichActivityPhotoMetadata(ctx, existing.ID, userID, meta, locSrc); err != nil {
			return nil, err
		}
		return &PhotoResult{PhotoID: existing.ID, ActivityID: existing.ActivityID, Status: "duplicate_exact"}, nil
	} else if err != nil {
		return nil, fmt.Errorf("recheck exact activity photo duplicate: %w", err)
	}

	// 4. pHash duplicate check against existing photos for this user.
	if pHashOk {
		existing, err := s.DB.GetUserActivityPhotoHashes(ctx, userID)
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
				newW, newH := decodePhotoDimensions(photoBytes, ext)
				existingPixels := 0
				if matchedPhoto.Width != nil && matchedPhoto.Height != nil {
					existingPixels = *matchedPhoto.Width * *matchedPhoto.Height
				}
				newPixels := newW * newH

				if newPixels > existingPixels && existingPixels > 0 {
					slog.Info("importer: phash duplicate with higher resolution, replacing",
						"photo", matchedPhoto.ID, "old_pixels", existingPixels, "new_pixels", newPixels)

					return s.replaceActivityPhotoOriginal(ctx, replaceActivityPhotoOriginalRequest{
						userID:       userID,
						activityID:   activityIDForStoredPhoto(matchedPhoto.ActivityID, activityID),
						photoID:      matchedPhoto.ID,
						photoBytes:   photoBytes,
						meta:         meta,
						contentType:  ct,
						fileHash:     fileHash,
						pHash:        pHashVal,
						width:        newW,
						height:       newH,
						locationSrc:  locSrc,
						previousFile: matchedPhoto.FileID,
					})
				}
				// EXIF enrichment: new photo has timestamp or GPS that the existing photo lacks.
				// Returns "duplicate_phash" so callers do not treat this as a new file import.
				if (meta.TakenAt != nil && matchedPhoto.TakenAt == nil) || (meta.Lat != nil && matchedPhoto.Lat == nil) {
					slog.Info("importer: phash duplicate, enriching with EXIF metadata", "photo", matchedPhoto.ID)
					if err := s.enrichActivityPhotoMetadata(ctx, matchedPhoto.ID, userID, meta, locSrc); err != nil {
						return nil, err
					}
					return &PhotoResult{PhotoID: matchedPhoto.ID, ActivityID: matchedPhoto.ActivityID, Status: "duplicate_phash", PHash: pHashVal}, nil
				}
				return &PhotoResult{PhotoID: matchedPhoto.ID, ActivityID: matchedPhoto.ActivityID, Status: "duplicate_phash", PHash: pHashVal}, nil
			}
		}
	}

	// 5. No duplicate — insert new photo.

	// Video metadata via ffprobe
	var videoMeta *imageutil.VideoMeta
	if isVideo {
		if vm, err := imageutil.ProbeVideo(photoBytes, ext); err == nil {
			videoMeta = &vm
		} else {
			slog.Warn("importer: ffprobe failed", "filename", meta.OriginalFilename, "error", err)
		}
	}

	// Thumbnail (200px) — first frame for video or ffmpeg-only images, otherwise imageutil.Resize
	var thumbData []byte
	switch {
	case isVideo:
		if td, err := imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, 200, 0); err == nil {
			thumbData = td
		} else {
			slog.Warn("importer: ffmpeg video thumbnail failed", "filename", meta.OriginalFilename, "error", err)
		}
	case usesFFmpegStillPipeline(ct):
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
		if usesFFmpegStillPipeline(ct) {
			previewData, _ = imageutil.GenerateThumbnailFFmpeg(photoBytes, ext, maxW, 0)
		} else {
			previewData, _ = imageutil.Resize(photoBytes, maxW, maxH, 85)
		}
	}

	// Decode image dimensions for width/height columns
	var imgWidth, imgHeight int
	if !isVideo {
		imgWidth, imgHeight = decodePhotoDimensions(photoBytes, ext)
	} else if videoMeta != nil {
		imgWidth = videoMeta.Width
		imgHeight = videoMeta.Height
	}

	// S3 paths
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", userID, fileHash, ext)
	s3KeyThumb := fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", userID, fileHash)

	shadowFileID, storeErr := s.storeActivityPhotoOriginal(ctx, userID, activityID, photoBytes, meta, ct, fileHash)
	if storeErr != nil {
		slog.Warn("importer: ShadowFS.Put photo failed", "err", storeErr)
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
		s.deleteStoredPhotoOriginal(ctx, userID, shadowFileID, "insert activity photo rollback")
		if isActivityPhotoUniqueHashViolation(err) {
			existing, lookupErr := s.DB.GetActivityPhotoByHash(ctx, userID, fileHash)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if existing != nil {
				if enrichErr := s.enrichActivityPhotoMetadata(ctx, existing.ID, userID, meta, locSrc); enrichErr != nil {
					return nil, enrichErr
				}
				return &PhotoResult{PhotoID: existing.ID, ActivityID: existing.ActivityID, Status: "duplicate_exact"}, nil
			}
		}
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
				ParentType:  new("photo"),
				ParentID:    new(saved.ID),
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
				ParentType:  new("photo"),
				ParentID:    new(saved.ID),
			}, bytes.NewReader(previewData))
			if previewErr != nil {
				slog.Warn("importer: ShadowFS.Put preview failed", "photo_id", saved.ID, "err", previewErr)
			}
		}
	}

	// Publish CLIP embed event (best-effort)
	if s.PhotoEmbedPublisher != nil {
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
		if err := s.PhotoEmbedPublisher.PublishPhotoEmbed(ev); err != nil {
			slog.Warn("importer: failed to publish photo embed event", "photo_id", saved.ID, "err", err)
		}
	}

	result := &PhotoResult{
		PhotoID:    saved.ID,
		ActivityID: activityID,
		Status:     "created",
	}
	if pHashOk {
		result.PHash = pHashVal
	}
	return result, nil
}

func activityIDForStoredPhoto(existingActivityID, fallbackActivityID string) string {
	if existingActivityID != "" {
		return existingActivityID
	}
	return fallbackActivityID
}

func sanitizePhotoLocationSource(locSrc string) string {
	switch locSrc {
	case "exif", "activity_track", "manual":
		return locSrc
	default:
		return ""
	}
}

func (s *Service) enrichActivityPhotoMetadata(ctx context.Context, photoID, userID string, meta PhotoMetadata, locSrc string) error {
	if meta.TakenAt == nil && meta.Lat == nil && meta.Lng == nil && locSrc == "" {
		return nil
	}
	if _, err := s.DB.Pool.Exec(ctx,
		`UPDATE activity_photos
		 SET taken_at        = COALESCE(taken_at, $1::timestamptz),
		     lat             = COALESCE(lat, $2::double precision),
		     lng             = COALESCE(lng, $3::double precision),
		     location_source = COALESCE(location_source, NULLIF($4, ''))
		 WHERE id = $5 AND user_id = $6`,
		meta.TakenAt, meta.Lat, meta.Lng, locSrc,
		photoID, userID,
	); err != nil {
		slog.Error("importer: enrich photo metadata failed", "photo", photoID, "err", err)
		return fmt.Errorf("enrich photo metadata in DB: %w", err)
	}
	return nil
}

func (s *Service) lockUserPhotoDedup(ctx context.Context, userID string) (func(), error) {
	conn, err := s.DB.Pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("photo dedup lock acquire connection: %w", err)
	}
	key := "activity_photo_dedup:" + userID
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		conn.Release()
		return nil, fmt.Errorf("photo dedup advisory lock: %w", err)
	}
	return func() {
		if _, err := conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key); err != nil {
			slog.Warn("photo dedup advisory unlock failed", "user_id", userID, "error", err)
		}
		conn.Release()
	}, nil
}

func usesFFmpegStillPipeline(ct string) bool {
	switch ct {
	case "image/heic", "image/heif", "image/jxl", "image/avif":
		return true
	default:
		return false
	}
}

func decodePhotoDimensions(data []byte, ext string) (int, int) {
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return img.Bounds().Dx(), img.Bounds().Dy()
	}
	if meta, err := imageutil.ProbeMedia(data, ext); err == nil && meta.Width > 0 && meta.Height > 0 {
		return meta.Width, meta.Height
	}
	return 0, 0
}

func isActivityPhotoUniqueHashViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "activity_photos_user_hash_unique"
}

type replaceActivityPhotoOriginalRequest struct {
	userID       string
	activityID   string
	photoID      string
	photoBytes   []byte
	meta         PhotoMetadata
	contentType  string
	fileHash     string
	pHash        int64
	width        int
	height       int
	locationSrc  string
	previousFile string
}

func (s *Service) replaceActivityPhotoOriginal(ctx context.Context, req replaceActivityPhotoOriginalRequest) (*PhotoResult, error) {
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("replace photo begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "activity_photo:"+req.photoID); err != nil {
		return nil, fmt.Errorf("replace photo advisory lock: %w", err)
	}

	var currentFileID string
	var currentWidth, currentHeight *int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(file_id::text, ''), width, height
		 FROM activity_photos
		 WHERE id = $1 AND user_id = $2
		 FOR UPDATE`,
		req.photoID, req.userID,
	).Scan(&currentFileID, &currentWidth, &currentHeight); err != nil {
		return nil, fmt.Errorf("replace photo reload: %w", err)
	}

	currentPixels := 0
	if currentWidth != nil && currentHeight != nil {
		currentPixels = *currentWidth * *currentHeight
	}
	newPixels := req.width * req.height
	if currentPixels > 0 && newPixels <= currentPixels {
		if _, err := tx.Exec(ctx,
			`UPDATE activity_photos
			 SET taken_at        = COALESCE(taken_at, $1::timestamptz),
			     lat             = COALESCE(lat, $2::double precision),
			     lng             = COALESCE(lng, $3::double precision),
			     location_source = COALESCE(location_source, NULLIF($4, ''))
			 WHERE id = $5 AND user_id = $6`,
			req.meta.TakenAt, req.meta.Lat, req.meta.Lng, req.locationSrc,
			req.photoID, req.userID,
		); err != nil {
			return nil, fmt.Errorf("replace photo fallback EXIF update: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("replace photo fallback commit: %w", err)
		}
		return &PhotoResult{PhotoID: req.photoID, ActivityID: req.activityID, Status: "duplicate_phash", PHash: req.pHash}, nil
	}

	newFileID, err := s.storeActivityPhotoOriginal(ctx, req.userID, req.activityID, req.photoBytes, req.meta, req.contentType, req.fileHash)
	if err != nil {
		return nil, fmt.Errorf("replace photo store original: %w", err)
	}
	cleanupNew := true
	defer func() {
		if cleanupNew {
			s.deleteStoredPhotoOriginal(ctx, req.userID, newFileID, "replace photo rollback")
		}
	}()

	if _, err := tx.Exec(ctx,
		`UPDATE activity_photos
		 SET file_id = $1,
		     file_size_bytes = $2,
		     file_hash = $3,
		     width = $4,
		     height = $5,
		     phash = $6,
		     taken_at        = COALESCE(taken_at, $7::timestamptz),
		     lat             = COALESCE(lat, $8::double precision),
		     lng             = COALESCE(lng, $9::double precision),
		     location_source = COALESCE(location_source, NULLIF($10, ''))
		 WHERE id = $11 AND user_id = $12`,
		newFileID, int64(len(req.photoBytes)), req.fileHash, req.width, req.height, req.pHash,
		req.meta.TakenAt, req.meta.Lat, req.meta.Lng, req.locationSrc,
		req.photoID, req.userID,
	); err != nil {
		return nil, fmt.Errorf("replace photo metadata in DB: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("replace photo commit: %w", err)
	}
	cleanupNew = false

	oldFileID := currentFileID
	if oldFileID == "" {
		oldFileID = req.previousFile
	}
	if oldFileID != "" && oldFileID != newFileID && s.ShadowFS != nil {
		if err := s.ShadowFS.Archive(context.WithoutCancel(ctx), req.userID, oldFileID, newFileID); err != nil {
			slog.Warn("importer: archive replaced photo original failed",
				"photo_id", req.photoID, "old_file_id", oldFileID, "new_file_id", newFileID, "err", err)
		}
	}

	return &PhotoResult{PhotoID: req.photoID, ActivityID: req.activityID, Status: "replaced", PHash: req.pHash}, nil
}

func (s *Service) storeActivityPhotoOriginal(ctx context.Context, userID, activityID string, photoBytes []byte, meta PhotoMetadata, contentType, fileHash string) (string, error) {
	if s.ShadowFS == nil {
		return "", fmt.Errorf("shadowfs not configured")
	}
	parentType := "activity"
	parentID := activityID
	rec, err := s.ShadowFS.Put(ctx, shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "media",
		ContentType: contentType,
		FileName:    meta.OriginalFilename,
		SizeBytes:   int64(len(photoBytes)),
		FileHash:    fileHash,
		ParentType:  &parentType,
		ParentID:    &parentID,
	}, bytes.NewReader(photoBytes))
	if err != nil {
		return "", err
	}
	return rec.ID, nil
}

func (s *Service) deleteStoredPhotoOriginal(ctx context.Context, userID, fileID, reason string) {
	if s.ShadowFS == nil || fileID == "" {
		return
	}
	if err := s.ShadowFS.Delete(context.WithoutCancel(ctx), userID, fileID); err != nil {
		slog.Warn("importer: ShadowFS.Delete stored photo failed", "file_id", fileID, "reason", reason, "err", err)
	}
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
	if resolveErr := s.DB.ResolveImportQueueItem(ctx, queueEntryID, userID, targetActivityID); resolveErr != nil {
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
