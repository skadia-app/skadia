package shadowfs

import (
	"mime"
	"net/http"
	"path/filepath"
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
