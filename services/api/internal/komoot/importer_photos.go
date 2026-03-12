package komoot

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bep/imagemeta"

	"github.com/skadia-app/skadia/services/api/internal/importer"
)

// PhotosImporter imports photos from tour cover_images (own photos uploaded directly to a tour).
// Must run AFTER ActivitiesImporter to have TourActivityMap populated.
// Note: Komoot does NOT use type "photo" in the timeline — own photos come via cover_images.
type PhotosImporter struct {
	Client          *Client
	TourActivityMap map[int64]string // komoot_tour_id → skadia activity.ID (from ActivitiesImporter)
}

func (i *PhotosImporter) Name() string        { return "photos" }
func (i *PhotosImporter) Description() string { return "Tour-Fotos" }
func (i *PhotosImporter) Priority() int       { return 40 }

// Import downloads and stores own photos from each tour's cover_images (directly uploaded tour photos).
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

		// cover_images = photos uploaded directly to the tour by the owner.
		// Komoot does NOT use type "photo" in the timeline; own photos come via cover_images.
		items := detail.Embedded.CoverImages.Embedded.Items
		for _, item := range items {
			if ctx.Err() != nil {
				break
			}

			// cover_images contains only the tour owner's own photos.
			// highlight_id > 0 means the photo was associated with a Highlight by the user — still their own photo, import it.
			imageURL := item.Src
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

			// Determine content type for EXIF decoding
			contentType := komootPhotoContentType(data, imageURL)

			// EXIF: extract taken_at and GPS coords
			var takenAt *time.Time
			var lat, lng *float64
			takenAt, lat, lng = extractKomootEXIF(data, contentType)

			// Fallback GPS from cover_image location field
			var locationSource string
			if lat != nil {
				locationSource = "exif"
			} else if item.Location != nil &&
				(item.Location.Lat != 0 || item.Location.Lng != 0) {
				la := item.Location.Lat
				lo := item.Location.Lng
				lat = &la
				lng = &lo
				locationSource = "activity_track"
			}
			// else: no coordinates at all — locationSource stays "" and lat/lng stay nil

			photoMeta := importer.PhotoMetadata{
				OriginalFilename: fmt.Sprintf("komoot-tour-%d-%d.jpg", tourID, item.ID),
				ContentType:      contentType,
				TakenAt:          takenAt,
				Lat:              lat,
				Lng:              lng,
				LocationSource:   locationSource,
				SourcePlatform:   "komoot",
			}

			photoResult, photoErr := p.ImportService.ProcessPhoto(ctx, p.UserID, activityID, data, photoMeta)
			if photoErr != nil {
				slog.Warn("komoot photos: process failed", "url", imageURL, "error", photoErr)
				result.Errors = append(result.Errors, fmt.Sprintf("process photo (tour %d): %s", tourID, photoErr))
				continue
			}

			switch photoResult.Status {
			case "created":
				result.Imported++
			case "duplicate_exact", "duplicate_phash":
				result.Skipped++
			case "replaced":
				result.Imported++
			}
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
