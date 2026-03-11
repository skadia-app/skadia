package komoot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bep/imagemeta"

	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// PhotosImporter imports photos from tour timelines (type=="photo" only — own photos).
// Must run AFTER ActivitiesImporter to have TourActivityMap populated.
type PhotosImporter struct {
	Client          *Client
	TourActivityMap map[int64]string // komoot_tour_id → skadia activity.ID (from ActivitiesImporter)
}

func (i *PhotosImporter) Name() string        { return "photos" }
func (i *PhotosImporter) Description() string { return "Tour-Fotos" }
func (i *PhotosImporter) Priority() int       { return 40 }

// Import downloads and stores own photos (type=="photo") from each tour's timeline.
func (i *PhotosImporter) Import(ctx context.Context, p KomootImportParams) (*KomootImportResult, error) {
	result := &KomootImportResult{}

	if len(i.TourActivityMap) == 0 {
		slog.Info("komoot photos: no tours in map, skipping")
		return result, nil
	}

	total := len(i.TourActivityMap)
	idx := 0

	for tourID, activityID := range i.TourActivityMap {
		if ctx.Err() != nil {
			break
		}

		idx++
		if p.OnProgress != nil {
			p.OnProgress(idx, total, fmt.Sprintf("photos: %d/%d (tour %d)", idx, total, tourID))
		}

		detail, err := i.Client.GetTourDetail(ctx, tourID)
		if err != nil {
			slog.Warn("komoot photos: get tour detail failed", "tour_id", tourID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("tour detail %d: %s", tourID, err))
			continue
		}

		items := detail.Embedded.Timeline.Embedded.Items
		for _, item := range items {
			if ctx.Err() != nil {
				break
			}

			// Only own photos (type=="photo")
			if item.Type != "photo" {
				continue
			}

			imageURL := item.Embedded.Reference.Src
			if imageURL == "" {
				result.Skipped++
				continue
			}

			data, err := DownloadPhoto(ctx, imageURL)
			if err != nil {
				slog.Warn("komoot photos: download failed", "url", imageURL, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("download photo (tour %d): %s", tourID, err))
				continue
			}

			// SHA256 hash for dedup
			hashBytes := sha256.Sum256(data)
			hash := fmt.Sprintf("%x", hashBytes)

			// Skip if already exists
			if p.DB != nil {
				if exists, err := p.DB.ActivityPhotoExistsByHash(ctx, p.UserID, hash); err == nil && exists {
					result.Skipped++
					continue
				}
			}

			// Determine content type and extension
			contentType := komootPhotoContentType(data, imageURL)
			ext := filepath.Ext(imageURL)
			if idx2 := strings.Index(ext, "?"); idx2 > 0 {
				ext = ext[:idx2]
			}
			if ext == "" {
				ext = ".jpg"
			}

			// EXIF: extract taken_at and GPS coords
			var takenAt *time.Time
			var lat, lng *float64
			takenAt, lat, lng = extractKomootEXIF(data, contentType)

			// Fallback GPS from timeline location
			var locationSource string
			if lat != nil {
				locationSource = "exif"
			} else if item.Embedded.Reference.Location != nil &&
				(item.Embedded.Reference.Location.Lat != 0 || item.Embedded.Reference.Location.Lng != 0) {
				la := item.Embedded.Reference.Location.Lat
				lo := item.Embedded.Reference.Location.Lng
				lat = &la
				lng = &lo
				locationSource = "manual"
			} else {
				locationSource = "activity_track"
			}

			// S3 paths
			s3Key := fmt.Sprintf("activity-photos/%s/%s%s", p.UserID, hash, ext)
			bucket := "skadia-media"

			// Thumbnail (200px) + Preview
			s3KeyThumb := s3Key
			s3KeyPreview := ""

			thumbData, _ := imageutil.Resize(data, 200, 200, 80)
			var previewData []byte
			if p.DB != nil {
				if settings, err := p.DB.GetEnrichmentSettings(ctx, p.UserID); err == nil {
					if maxW, maxH := imageutil.PreviewSizePixels(settings.PhotoPreviewSize); maxW > 0 {
						previewData, _ = imageutil.Resize(data, maxW, maxH, 85)
					}
				}
			}

			if p.Storage != nil {
				if err := p.Storage.UploadToBucket(ctx, bucket, s3Key, data, contentType); err != nil {
					slog.Warn("komoot photos: s3 upload failed", "url", imageURL, "error", err)
					result.Errors = append(result.Errors, fmt.Sprintf("s3 upload (tour %d): %s", tourID, err))
					continue
				}

				if thumbData != nil {
					s3KeyThumb = fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", p.UserID, hash)
					if err := p.Storage.UploadToBucket(ctx, bucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
						slog.Warn("komoot photos: s3 thumb upload failed", "url", imageURL, "error", err)
						s3KeyThumb = s3Key
					}
				}

				if previewData != nil {
					s3KeyPreview = fmt.Sprintf("activity-photos/%s/%s_preview.jpg", p.UserID, hash)
					if err := p.Storage.UploadToBucket(ctx, bucket, s3KeyPreview, previewData, "image/jpeg"); err != nil {
						slog.Warn("komoot photos: s3 preview upload failed", "url", imageURL, "error", err)
						s3KeyPreview = ""
					}
				}
			}

			actIDPtr := &activityID

			photo := model.ActivityPhoto{
				UserID:           p.UserID,
				ActivityID:       actIDPtr,
				S3Bucket:         bucket,
				S3Key:            s3Key,
				S3KeyThumb:       s3KeyThumb,
				S3KeyPreview:     s3KeyPreview,
				OriginalFilename: filepath.Base(imageURL),
				ContentType:      contentType,
				FileSizeBytes:    int64(len(data)),
				FileHash:         &hash,
				TakenAt:          takenAt,
				Lat:              lat,
				Lng:              lng,
				LocationSource:   &locationSource,
			}

			if p.DB != nil {
				if _, err := p.DB.InsertActivityPhoto(ctx, photo); err != nil {
					slog.Warn("komoot photos: insert failed", "url", imageURL, "error", err)
					result.Errors = append(result.Errors, fmt.Sprintf("insert photo (tour %d): %s", tourID, err))
					continue
				}
			}

			result.Imported++
		}
	}

	slog.Info("komoot photos import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// komootPhotoContentType determines the MIME type from photo bytes + URL fallback.
// Komoot photos are almost exclusively JPEG; this handles edge cases.
func komootPhotoContentType(data []byte, imageURL string) string {
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)

	if ct == "application/octet-stream" || ct == "text/plain; charset=utf-8" {
		// Strip query params from URL for extension detection
		u := imageURL
		if idx := strings.Index(u, "?"); idx > 0 {
			u = u[:idx]
		}
		switch strings.ToLower(filepath.Ext(u)) {
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".webp":
			return "image/webp"
		case ".heic", ".heif":
			return "image/heic"
		}
	}
	return ct
}

// komootImageFormatFromContentType maps a MIME type to the imagemeta.ImageFormat for EXIF decoding.
func komootImageFormatFromContentType(ct string) imagemeta.ImageFormat {
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
		return imagemeta.JPEG
	}
}

// extractKomootEXIF reads taken_at, lat, lng from image EXIF data.
// Reuses the imagemeta library consistent with the Strava importer.
func extractKomootEXIF(data []byte, contentType string) (takenAt *time.Time, lat *float64, lng *float64) {
	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: komootImageFormatFromContentType(contentType),
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
