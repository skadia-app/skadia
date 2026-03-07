package strava

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/bep/imagemeta"
	"golang.org/x/image/draw"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// MediaImporter imports photos from the Strava export's media/ directory.
// It reads media.csv (Medien-Dateiname, Medienunterschrift), reads EXIF data from each
// photo, matches photos to activities by timestamp, and stores them in activity_photos.
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

	hdr := csvHeader(rows[0])
	total := len(rows) - 1
	slog.Info("strava media: found photos", "count", total)

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

		// EXIF: extract taken_at and GPS coords
		takenAt, lat, lng := extractMediaEXIF(data)

		// Match photo to activity by taken_at timestamp
		var activityIDPtr *string
		if takenAt != nil && params.DB != nil {
			if actID, err := params.DB.FindActivityIDAtTime(ctx, params.UserID, *takenAt); err == nil && actID != "" {
				activityIDPtr = &actID
			}
		}

		// Hash for dedup / S3 key
		hashBytes := sha256.Sum256(data)
		hash := fmt.Sprintf("%x", hashBytes)

		// Generate thumbnail
		thumbData, thumbErr := mediaGenerateThumbnail(data, 200, 200)

		// S3 paths
		ext := filepath.Ext(relPath)
		if ext == "" {
			ext = ".jpg"
		}
		s3Key := fmt.Sprintf("activity-photos/%s/%s%s", params.UserID, hash, ext)
		s3KeyThumb := s3Key // fallback: original as thumb
		bucket := "skadia-media"

		if params.Storage != nil {
			if err := params.Storage.UploadToBucket(ctx, bucket, s3Key, data, "image/jpeg"); err != nil {
				slog.Warn("strava media: s3 upload failed", "file", relPath, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("s3 upload %s: %s", relPath, err))
				continue
			}

			if thumbErr == nil && thumbData != nil {
				s3KeyThumb = fmt.Sprintf("activity-photos/%s/%s_thumb.jpg", params.UserID, hash)
				if err := params.Storage.UploadToBucket(ctx, bucket, s3KeyThumb, thumbData, "image/jpeg"); err != nil {
					slog.Warn("strava media: s3 thumb upload failed", "file", relPath, "error", err)
					s3KeyThumb = s3Key
				}
			}
		}

		photo := model.ActivityPhoto{
			UserID:           params.UserID,
			ActivityID:       activityIDPtr,
			S3Bucket:         bucket,
			S3Key:            s3Key,
			S3KeyThumb:       s3KeyThumb,
			OriginalFilename: filepath.Base(relPath),
			ContentType:      "image/jpeg",
			FileSizeBytes:    int64(len(data)),
			FileHash:         &hash,
			TakenAt:          takenAt,
			Lat:              lat,
			Lng:              lng,
		}
		if lat != nil {
			ls := "exif"
			photo.LocationSource = &ls
		}

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

// extractMediaEXIF reads taken_at, lat, lng from JPEG EXIF data.
func extractMediaEXIF(data []byte) (takenAt *time.Time, lat *float64, lng *float64) {
	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: imagemeta.JPEG,
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

// mediaGenerateThumbnail creates a max-200x200 JPEG thumbnail.
func mediaGenerateThumbnail(data []byte, maxW, maxH int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	scaleW := float64(maxW) / float64(w)
	scaleH := float64(maxH) / float64(h)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	if scale >= 1 {
		scale = 1
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
