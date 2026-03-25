package filestore

import (
	"mime"
	"net/http"
	"path/filepath"
)

// DetectContentType determines the MIME type for a file.
// Priority: explicit override > extension > magic bytes.
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
