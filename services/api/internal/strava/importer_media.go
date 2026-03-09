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
	"strconv"
	"strings"
	"time"

	"github.com/bep/imagemeta"
	"golang.org/x/image/draw"

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

		// EXIF: extract taken_at and GPS coords
		takenAt, lat, lng := extractMediaEXIF(data)

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
