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

	"github.com/bep/imagemeta"

	"github.com/skadia-app/skadia/services/api/internal/imageutil"
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
			takenAt, lat, lng = extractMediaEXIF(data, contentType)
		}

		// Primary: look up activity via activities.csv Medien mapping
		var activityIDPtr *string
		uuid := mediaUUID(relPath)
		if actID, ok := uuidToActivityID[uuid]; ok {
			activityIDPtr = &actID
		}

		// Fallback: match by EXIF timestamp if no direct mapping found
		if activityIDPtr == nil && takenAt != nil && params.DB != nil {
			if actID, err := params.DB.FindActivityIDAtTime(ctx, params.UserID, *takenAt); err == nil && actID != "" {
				activityIDPtr = &actID
			}
		}

		// Hash for dedup / S3 key
		hashBytes := sha256.Sum256(data)
		hash := fmt.Sprintf("%x", hashBytes)

		// Skip if photo with same hash already exists for this user
		if params.DB != nil {
			if exists, err := params.DB.ActivityPhotoExistsByHash(ctx, params.UserID, hash); err == nil && exists {
				result.Skipped++
				continue
			}
		}

		// S3 paths
		ext := filepath.Ext(relPath)
		if ext == "" {
			ext = ".jpg"
		}
		s3Key := fmt.Sprintf("activity-photos/%s/%s%s", params.UserID, hash, ext)
		bucket := "skadia-media"

		// For videos: skip thumbnail/preview generation entirely.
		// Thumbnails for videos will be generated later by the ML service via CLIP.
		// For images: generate thumbnail (200×200) and preview.
		s3KeyThumb := s3Key // fallback: original as thumb
		s3KeyPreview := ""

		var thumbData []byte
		var thumbErr error
		var previewData []byte

		if !isVideo {
			thumbData, thumbErr = imageutil.Resize(data, 200, 200, 80)

			if params.DB != nil {
				if settings, err := params.DB.GetEnrichmentSettings(ctx, params.UserID); err == nil {
					if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
						previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
					}
				}
			}
		}

		if params.Storage != nil {
			if err := params.Storage.UploadToBucket(ctx, bucket, s3Key, data, contentType); err != nil {
				slog.Warn("strava media: s3 upload failed", "file", relPath, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("s3 upload %s: %s", relPath, err))
				continue
			}

			if !isVideo {
				if thumbErr == nil && thumbData != nil {
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
			} else {
				// Videos: no thumbnails at import time — ML service generates them later.
				s3KeyThumb = ""
				s3KeyPreview = ""
			}
		}

		photo := model.ActivityPhoto{
			UserID:           params.UserID,
			ActivityID:       activityIDPtr,
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
		}

		if lat != nil {
			ls := "exif"
			photo.LocationSource = &ls
		} else if activityIDPtr != nil && (lat == nil) {
			// For videos and images without EXIF GPS: location comes from activity track.
			ls := "activity_track"
			photo.LocationSource = &ls
		}

		// TODO(agent1/migration-000138): Once ActivityPhoto.IsVideo field is available,
		// set photo.IsVideo = isVideo here to distinguish videos from images in the DB.
		// Also set photo.DurationS if video duration can be extracted.

		if params.DB != nil {
			if _, err := params.DB.InsertActivityPhoto(ctx, photo); err != nil {
				slog.Warn("strava media: insert photo failed", "file", relPath, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("insert %s: %s", relPath, err))
				continue
			}
		}

		result.Imported++
	}

	slog.Info("strava media import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
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

		for _, path := range strings.Split(mediaCell, "|") {
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

// imageFormatFromContentType maps a MIME content type to the imagemeta.ImageFormat
// constant required for EXIF decoding.
func imageFormatFromContentType(ct string) imagemeta.ImageFormat {
	switch {
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return imagemeta.JPEG
	case strings.Contains(ct, "png"):
		return imagemeta.PNG
	case strings.Contains(ct, "tiff"):
		return imagemeta.TIFF
	case strings.Contains(ct, "webp"):
		return imagemeta.WebP
	case strings.Contains(ct, "heic"), strings.Contains(ct, "heif"):
		return imagemeta.HEIF
	case strings.Contains(ct, "avif"):
		return imagemeta.AVIF
	default:
		return imagemeta.JPEG // safe fallback
	}
}

// extractMediaEXIF reads taken_at, lat, lng from image EXIF data.
// The contentType parameter selects the correct imagemeta decoder.
func extractMediaEXIF(data []byte, contentType string) (takenAt *time.Time, lat *float64, lng *float64) {
	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: imageFormatFromContentType(contentType),
		Sources:     imagemeta.EXIF,
		HandleTag: func(info imagemeta.TagInfo) error {
			tags.Add(info)
			return nil
		},
	})
	if err != nil {
		return nil, nil, nil
	}

	var t *time.Time
	if tm, err := tags.GetDateTime(); err == nil && !tm.IsZero() {
		t = &tm
	}

	if la, lo, err := tags.GetLatLong(); err == nil && (la != 0 || lo != 0) {
		lat = &la
		lng = &lo
	}

	return t, lat, lng
}
