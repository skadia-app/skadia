package handler

import (
	"fmt"
	"net/http"
)

// allowedImageTypes lists the MIME types accepted for image uploads.
// SVG is intentionally excluded because it can embed arbitrary JavaScript.
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// validateImageBytes inspects the first 512 bytes of data using
// http.DetectContentType to determine the real MIME type and compares it
// against the allowlist. The client-supplied Content-Type header is NOT
// trusted. Returns an error message suitable for a 400 response, or an
// empty string when the data is valid.
func validateImageBytes(data []byte) (detectedType string, errMsg string) {
	if len(data) == 0 {
		return "", "uploaded file is empty"
	}

	// DetectContentType inspects at most the first 512 bytes.
	probe := data
	if len(probe) > 512 {
		probe = probe[:512]
	}
	detected := http.DetectContentType(probe)

	// Strip optional parameters (e.g. "image/jpeg; charset=...").
	mimeType := detected
	if i := len(detected); i > 0 {
		for j := 0; j < len(detected); j++ {
			if detected[j] == ';' {
				mimeType = detected[:j]
				break
			}
		}
	}

	if !allowedImageTypes[mimeType] {
		return mimeType, fmt.Sprintf("unsupported image type %q — allowed: jpeg, png, webp, gif", mimeType)
	}
	return mimeType, ""
}
