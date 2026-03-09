// Package imageutil provides shared image processing utilities for Skadia.
package imageutil

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder

	"golang.org/x/image/draw"
)

// Resize scales src to fit within maxW × maxH while preserving aspect ratio.
// Returns (nil, nil) when the image is already ≤ the target dimensions (no upscaling).
// The result is always JPEG-encoded with the given quality (1-100).
func Resize(data []byte, maxW, maxH, quality int) ([]byte, error) {
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
		// Original is already smaller or equal to target — no resize needed.
		return nil, nil
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PreviewSizePixels converts a preview size setting string to max pixel dimensions.
// "2048" → 2048×2048, "4k" → 3840×3840, "original" → (0, 0) meaning no resize.
func PreviewSizePixels(size string) (maxW, maxH int) {
	switch size {
	case "4k":
		return 3840, 3840
	case "original":
		return 0, 0
	default: // "2048" and any unknown value
		return 2048, 2048
	}
}
