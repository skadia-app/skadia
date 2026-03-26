package shadowfs

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
)

// DetectContentType determines the MIME type for a file.
// Priority: explicit override > extension > magic bytes > application/octet-stream fallback.
// explicit: caller-supplied MIME type (e.g. stored Content-Type from S3); overrides all other detection.
// first512: up to 512 bytes of file content used for magic-byte sniffing when extension lookup fails.
func DetectContentType(fileName string, explicit string, first512 []byte) string {
	if explicit != "" {
		return explicit
	}
	ext := filepath.Ext(fileName)
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	if len(first512) > 0 {
		return http.DetectContentType(first512)
	}
	return "application/octet-stream"
}

// sanitizeExt normalises a file extension to lowercase alphanumeric chars
// preceded by a dot. Returns "" if ext is empty or contains no valid chars.
func sanitizeExt(ext string) string {
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	// Keep only the dot and lowercase alphanumeric chars.
	var b strings.Builder
	b.WriteByte('.')
	for _, r := range ext[1:] {
		lower := unicode.ToLower(r)
		if (lower >= 'a' && lower <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(lower)
		}
	}
	if b.Len() == 1 {
		return "" // Only had a dot, no valid chars.
	}
	return b.String()
}
