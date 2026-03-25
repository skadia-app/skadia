package strava

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// MediaImporter imports photos from the Strava export's media/ directory.
// Primary mapping: activities.csv "Medien" column (pipe-separated UUIDs per activity).
// Fallback: EXIF timestamp → nearest activity lookup.
type MediaImporter struct{}

func (m *MediaImporter) Name() string        { return "media" }
func (m *MediaImporter) Description() string { return "Fotos aus dem media/-Verzeichnis" }
func (m *MediaImporter) Priority() int       { return 50 }

func (m *MediaImporter) Detect(src ExportSource) (bool, int, error) {
	rows, err := src.ReadCSV("media.csv")
	if err != nil || len(rows) < 2 {
		return false, 0, nil
	}
	return true, len(rows) - 1, nil
}

func (m *MediaImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	rows, err := params.Source.ReadCSV("media.csv")
	if err != nil {
		slog.Warn("strava media: cannot read media.csv", "error", err)
		return result, nil
	}
	if len(rows) < 2 {
		return result, nil
	}

	// Build mapping: media UUID (filename without ext) → Skadia activity ID
	// by reading activities.csv "Medien" column (pipe-separated UUID paths).
	uuidToActivityID := buildMediaActivityMap(ctx, params)

	hdr := csvHeader(rows[0])
	total := len(rows) - 1
	slog.Info("strava media: found photos", "count", total, "mapped", len(uuidToActivityID))

	for i, row := range rows[1:] {
		if ctx.Err() != nil {
			break
		}

		relPath := colVal(row, hdr["Medien-Dateiname"])
		if relPath == "" {
			result.Skipped++
			continue
		}

		if params.OnProgress != nil {
			params.OnProgress(i+1, total, fmt.Sprintf("media: %d/%d %s", i+1, total, filepath.Base(relPath)))
		}

		data, err := params.Source.ReadFile(relPath)
		if err != nil {
			slog.Warn("strava media: cannot read photo", "file", relPath, "error", err)
			result.Skipped++
			continue
		}

		// Detect content type from file bytes + extension fallback.
		contentType := detectMediaContentType(data, relPath)
		isVideo := isVideoContentType(contentType)

		// EXIF: extract taken_at and GPS coords (only for images, not videos).
		var takenAt *time.Time
		var lat, lng *float64
		if !isVideo {
			takenAt, lat, lng = importutil.ExtractEXIF(data, contentType)
		}

		// Primary: look up activity via activities.csv Medien mapping
		var activityID string
		uuid := mediaUUID(relPath)
		if actID, ok := uuidToActivityID[uuid]; ok {
			activityID = actID
		}

		// Fallback: match by EXIF timestamp if no direct mapping found
		if activityID == "" && takenAt != nil && params.DB != nil {
			if actID, err := params.DB.FindActivityIDAtTime(ctx, params.UserID, *takenAt); err == nil && actID != "" {
				activityID = actID
			}
		}

		// Determine location source
		locationSource := ""
		if lat != nil {
			locationSource = "exif"
		} else if activityID != "" {
			locationSource = "activity_track"
		}

		photoMeta := importer.PhotoMetadata{
			OriginalFilename: filepath.Base(relPath),
			ContentType:      contentType,
			TakenAt:          takenAt,
			Lat:              lat,
			Lng:              lng,
			LocationSource:   locationSource,
			SourcePlatform:   "strava",
		}

		// --- Use ImportService pipeline when available ---
		if params.ImportService != nil {
			if activityID != "" {
				photoResult, processErr := params.ImportService.ProcessPhoto(ctx, params.UserID, activityID, data, photoMeta)
				if processErr != nil {
					slog.Warn("strava media: ProcessPhoto failed", "file", relPath, "error", processErr)
					result.Errors = append(result.Errors, fmt.Sprintf("process %s: %s", relPath, processErr))
					continue
				}
				switch photoResult.Status {
				case "duplicate_exact", "duplicate_phash":
					result.Skipped++
				default:
					result.Imported++
				}
			} else {
				orphanResult, orphanErr := params.ImportService.ProcessOrphanPhoto(ctx, params.UserID, data, photoMeta)
				if orphanErr != nil {
					slog.Warn("strava media: ProcessOrphanPhoto failed", "file", relPath, "error", orphanErr)
					result.Errors = append(result.Errors, fmt.Sprintf("orphan %s: %s", relPath, orphanErr))
					continue
				}
				switch orphanResult.Status {
				case "queued", "matched":
					result.Imported++
				default:
					result.Skipped++
				}
			}
			continue
		}

		// --- Legacy path (no ImportService): inline processing ---
		imported := m.legacyImportPhoto(ctx, params, relPath, data, contentType, isVideo, takenAt, lat, lng, activityID)
		if imported {
			result.Imported++
		} else {
			result.Skipped++
		}
	}

	slog.Info("strava media import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// legacyImportPhoto handles a single photo when ImportService is not available.
// Returns true if the photo was imported, false if it was skipped or an error occurred.
func (m *MediaImporter) legacyImportPhoto(
	ctx context.Context,
	params ImportParams,
	relPath string,
	data []byte,
	contentType string,
	isVideo bool,
	takenAt *time.Time,
	lat, lng *float64,
	activityID string,
) bool {
	ext := filepath.Ext(relPath)
	if ext == "" {
		ext = ".jpg"
	}

	// Guard: if content-type detection returned image/* but the file extension is a
	// known video container, trust the extension.
	if !isVideo && isVideoExtension(ext) {
		isVideo = true
		contentType = detectMediaContentType(data, relPath)
		if !isVideoContentType(contentType) {
			contentType = videoContentTypeFromExt(ext)
		}
	}

	// Hash for dedup / S3 key
	hashBytes := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", hashBytes)

	// Skip if photo with same hash already exists for this user
	if params.DB != nil {
		if exists, err := params.DB.ActivityPhotoExistsByHash(ctx, params.UserID, hash); err == nil && exists {
			return false
		}
	}

	// S3 paths
	bucket := "skadia-media"
	s3Key := fmt.Sprintf("activity-photos/%s/%s%s", params.UserID, hash, ext)

	// Thumbnail (200px) + Preview generation
	s3KeyThumb := s3Key
	s3KeyPreview := ""
	var thumbData []byte
	var previewData []byte

	useFFmpeg := isVideo || contentType == "image/heic" || contentType == "image/heif" || contentType == "image/jxl"

	if isVideo {
		var midOffset float64
		if meta, err := imageutil.ProbeVideo(data, ext); err == nil && meta.DurationS > 2 {
			midOffset = meta.DurationS / 2
		}
		thumbData, _ = imageutil.GenerateThumbnailFFmpeg(data, ext, 200, 0)
		previewData, _ = imageutil.GenerateThumbnailFFmpeg(data, ext, 2560, midOffset)
	} else if useFFmpeg {
		thumbData, _ = imageutil.GenerateThumbnailFFmpeg(data, ext, 200, 0)
		if params.DB != nil {
			if settings, err := params.DB.GetEnrichmentSettings(ctx, params.UserID); err == nil {
				if maxW, _ := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
					previewData, _ = imageutil.GenerateThumbnailFFmpeg(data, ext, maxW, 0)
				}
			}
		}
	} else {
		thumbData, _ = imageutil.Resize(data, 200, 200, 80)
		if params.DB != nil {
			if settings, err := params.DB.GetEnrichmentSettings(ctx, params.UserID); err == nil {
				if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
					previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
				}
			}
		}
	}

	// --- Upload original photo ---
	// FileStore (preferred): registers in user_files and sets file_id.
	// Legacy S3 fallback: direct bucket upload, no registry entry.
	var fileID *string
	if params.FileStore != nil {
		parentType := "activity"
		rec, putErr := params.FileStore.Put(ctx, filestore.FileEntry{
			UserID:      params.UserID,
			FileType:    "media",
			ContentType: contentType,
			FileName:    filepath.Base(relPath),
			SizeBytes:   int64(len(data)),
			FileHash:    hash,
			ParentType:  &parentType,
			ParentID:    func() *string {
				if activityID != "" {
					return &activityID
				}
				return nil
			}(),
			Tags: map[string]string{"source": "strava"},
		}, bytes.NewReader(data))
		if putErr != nil {
			slog.Warn("strava media: FileStore.Put failed", "file", relPath, "error", putErr)
			return false
		}
		fileID = &rec.ID
	} else if params.Storage != nil {
		if err := params.Storage.UploadToBucket(ctx, bucket, s3Key, data, contentType); err != nil {
			slog.Warn("strava media: s3 upload failed", "file", relPath, "error", err)
			return false
		}
	} else {
		// Neither FileStore nor Storage configured: cannot persist the photo.
		slog.Warn("strava media: no storage backend configured, skipping", "file", relPath)
		return false
	}

	// --- Upload thumb/preview (regenerable cache, S3 only, not in FileStore registry) ---
	// If Storage is nil but FileStore exists, skip generation — they'll be regenerated on demand.
	if params.Storage != nil {
		if thumbData != nil {
			s3KeyThumb = fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", params.UserID, hash)
			if err := params.Storage.UploadToBucket(ctx, bucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
				slog.Warn("strava media: s3 thumb upload failed", "file", relPath, "error", err)
				s3KeyThumb = s3Key
			}
		}

		if previewData != nil {
			s3KeyPreview = fmt.Sprintf("activity-photos/%s/%s_preview.jpg", params.UserID, hash)
			if err := params.Storage.UploadToBucket(ctx, bucket, s3KeyPreview, previewData, "image/jpeg"); err != nil {
				slog.Warn("strava media: s3 preview upload failed", "file", relPath, "error", err)
				s3KeyPreview = ""
			}
		}
	}

	var activityIDPtr *string
	if activityID != "" {
		activityIDPtr = &activityID
	}

	photo := model.ActivityPhoto{
		UserID:           params.UserID,
		ActivityID:       activityIDPtr,
		FileID:           fileID,
		S3Bucket:         bucket,
		S3Key:            s3Key,
		S3KeyThumb:       s3KeyThumb,
		S3KeyPreview:     s3KeyPreview,
		OriginalFilename: filepath.Base(relPath),
		ContentType:      contentType,
		FileSizeBytes:    int64(len(data)),
		FileHash:         &hash,
		TakenAt:          takenAt,
		Lat:              lat,
		Lng:              lng,
		IsVideo:          isVideo,
	}

	if isVideo {
		if meta, err := imageutil.ProbeVideo(data, ext); err == nil {
			photo.DurationS = &meta.DurationS
			photo.VideoWidth = &meta.Width
			photo.VideoHeight = &meta.Height
		}
	}

	if lat != nil {
		ls := "exif"
		photo.LocationSource = &ls
	} else if activityIDPtr != nil {
		ls := "activity_track"
		photo.LocationSource = &ls
	}

	if params.DB != nil {
		if _, err := params.DB.InsertActivityPhoto(ctx, photo); err != nil {
			slog.Warn("strava media: insert photo failed", "file", relPath, "error", err)
			return false
		}
	}

	return true
}

// buildMediaActivityMap reads activities.csv and returns a map from media UUID
// (filename stem, e.g. "eb811093-44e9-437c-8127-abcd02647414") to Skadia activity ID.
func buildMediaActivityMap(ctx context.Context, params ImportParams) map[string]string {
	m := make(map[string]string)
	if params.DB == nil {
		return m
	}

	rows, err := params.Source.ReadCSV("activities.csv")
	if err != nil || len(rows) < 2 {
		return m
	}

	hdr := csvHeader(rows[0])
	medienCol, hasMediaCol := hdr["Medien"]
	stravaIDCol, hasIDCol := hdr["Aktivitäts-ID"]
	if !hasMediaCol || !hasIDCol {
		return m
	}

	for _, row := range rows[1:] {
		if ctx.Err() != nil {
			break
		}
		stravaIDStr := colVal(row, stravaIDCol)
		mediaCell := colVal(row, medienCol)
		if stravaIDStr == "" || mediaCell == "" {
			continue
		}

		stravaID, err := strconv.ParseInt(stravaIDStr, 10, 64)
		if err != nil {
			continue
		}

		actID, err := params.DB.GetActivityIDByStravaID(ctx, params.UserID, stravaID)
		if err != nil || actID == "" {
			continue
		}

		for path := range strings.SplitSeq(mediaCell, "|") {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			m[mediaUUID(path)] = actID
		}
	}
	return m
}

// mediaUUID returns the filename stem (UUID without extension) from a media path.
func mediaUUID(relPath string) string {
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// detectMediaContentType determines the MIME type from file bytes, falling back to
// extension-based detection for formats that http.DetectContentType cannot identify
// (HEIC, JXL, some video containers).
func detectMediaContentType(data []byte, filename string) string {
	// Use at most 512 bytes for sniffing (http.DetectContentType requirement).
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)

	if ct == "application/octet-stream" || ct == "text/plain; charset=utf-8" {
		switch strings.ToLower(filepath.Ext(filename)) {
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
		}
	}
	return ct
}

// isVideoContentType reports whether the given MIME type represents a video.
func isVideoContentType(ct string) bool {
	return strings.HasPrefix(ct, "video/")
}


// isVideoExtension reports whether the file extension (including the dot) is a known
// video container format. Used to catch mis-detected content types where http.DetectContentType
// returns image/* for a video file.
func isVideoExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm":
		return true
	}
	return false
}

// videoContentTypeFromExt returns the most specific MIME type for a video extension
// when content-type sniffing failed.
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
