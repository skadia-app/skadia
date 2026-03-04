package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// maxZIPUploadBytes is the maximum allowed size for an uploaded export ZIP (2 GB).
const maxZIPUploadBytes = 2 << 30

// extractZipToTempDir reads a ZIP from r (limited to maxBytes), extracts it to a
// newly created temp directory and returns that directory path.
// The caller is responsible for removing the directory (os.RemoveAll) when done.
func extractZipToTempDir(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxZIPUploadBytes))
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "skadia-import-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("open zip: %w", err)
	}

	for _, f := range zr.File {
		if err := extractZipEntry(f, tmpDir); err != nil {
			// Non-fatal: log and continue with remaining entries.
			continue
		}
	}

	return tmpDir, nil
}

// extractZipEntry extracts a single zip.File to destDir with path-traversal protection.
func extractZipEntry(f *zip.File, destDir string) error {
	// Sanitise path: strip leading slashes / ".." components.
	clean := filepath.Clean(filepath.Join(destDir, f.Name))
	if !strings.HasPrefix(clean, destDir+string(os.PathSeparator)) && clean != destDir {
		return fmt.Errorf("invalid path: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(clean, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(clean)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// parseUploadedZIP is a helper used by the upload-zip handlers.
// It parses the multipart form, extracts the ZIP to a temp dir and returns the dir path.
func parseUploadedZIP(w http.ResponseWriter, r *http.Request, path string) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxZIPUploadBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "cannot parse form (file too large?)", path)
		return "", false
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing file field", path)
		return "", false
	}
	defer file.Close()

	tmpDir, err := extractZipToTempDir(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid ZIP: "+err.Error(), path)
		return "", false
	}
	return tmpDir, true
}
