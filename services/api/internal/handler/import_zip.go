package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// maxZIPUploadBytes is the maximum allowed size for an uploaded export ZIP (2 GB).
const maxZIPUploadBytes = 2 << 30

// maxTotalExtractedBytes caps cumulative decompressed bytes across all entries
// in a single ZIP archive to mitigate multi-file zip bombs.
const maxTotalExtractedBytes int64 = 4 << 30

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

	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("open extract root: %w", err)
	}
	defer root.Close()

	var totalExtracted int64
	for _, f := range zr.File {
		remaining := maxTotalExtractedBytes - totalExtracted
		if remaining <= 0 {
			break
		}
		written, err := extractZipEntry(root, f, remaining)
		totalExtracted += written
		if err != nil {
			// Non-fatal: skip unsafe or unreadable entries and continue with remaining entries.
			continue
		}
	}

	return tmpDir, nil
}

// cleanZipEntryPath validates a ZIP entry name and returns a clean relative
// slash-separated path guaranteed not to escape the extraction root.
func cleanZipEntryPath(name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("absolute path not allowed: %s", name)
	}
	clean := path.Clean(normalized)
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid path: %s", name)
	}
	return clean, nil
}

// extractZipEntry extracts a single zip.File into root with kernel-enforced
// path confinement (os.Root) and symlink/size-budget protection.
func extractZipEntry(root *os.Root, f *zip.File, remainingBudget int64) (int64, error) {
	if f.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("symlinks not allowed in archive: %s", f.Name)
	}

	relPath, err := cleanZipEntryPath(f.Name)
	if err != nil {
		return 0, err
	}

	if f.FileInfo().IsDir() {
		return 0, root.MkdirAll(relPath, 0o755)
	}

	if dir := path.Dir(relPath); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return 0, err
		}
	}

	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	out, err := root.OpenFile(relPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	// Limit per-entry and cumulative extraction size to defend against zip bomb attacks.
	const maxEntrySize int64 = 500 << 20 // 500 MB
	limit := min(maxEntrySize, remainingBudget)
	n, err := io.Copy(out, io.LimitReader(rc, limit))
	return n, err
}

// parseUploadedZIP is a helper used by the upload-zip handlers.
// It parses the multipart form, extracts the ZIP to a temp dir and returns the dir path.
func parseUploadedZIP(w http.ResponseWriter, r *http.Request, path string) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxZIPUploadBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "cannot parse form (file too large?)", path)
		return "", false
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing file field", path)
		return "", false
	}
	defer file.Close()

	tmpDir, err := extractZipToTempDir(file)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid ZIP: "+err.Error(), path)
		return "", false
	}
	return tmpDir, true
}
