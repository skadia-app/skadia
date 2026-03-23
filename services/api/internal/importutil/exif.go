package importutil

import (
	"bytes"
	"strings"
	"time"

	"github.com/bep/imagemeta"
)

// ImageFormatFromContentType maps a MIME content type to the imagemeta.ImageFormat
// constant required for EXIF decoding.
func ImageFormatFromContentType(ct string) imagemeta.ImageFormat {
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

// ExtractEXIF reads taken_at, lat, lng from image EXIF data.
// The contentType parameter selects the correct imagemeta decoder.
func ExtractEXIF(data []byte, contentType string) (takenAt *time.Time, lat *float64, lng *float64) {
	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: ImageFormatFromContentType(contentType),
		Sources:     imagemeta.EXIF,
		HandleTag: func(info imagemeta.TagInfo) error {
			tags.Add(info)
			return nil
		},
	})
	if err != nil {
		return nil, nil, nil
	}

	if tm, err := tags.GetDateTime(); err == nil && !tm.IsZero() {
		takenAt = &tm
	}

	if la, lo, err := tags.GetLatLong(); err == nil && (la != 0 || lo != 0) {
		lat = &la
		lng = &lo
	}

	return takenAt, lat, lng
}
