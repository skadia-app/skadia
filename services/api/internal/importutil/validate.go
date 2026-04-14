package importutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateImportSourcePath checks that the given path is a clean, absolute path
// within os.TempDir(). Returns the cleaned path or an error if validation fails.
func ValidateImportSourcePath(sourcePath string) (string, error) {
	return ValidateImportSourcePathWithDirs(sourcePath, nil)
}

// ValidateImportSourcePathWithDirs checks that the given path is within os.TempDir()
// or one of the additional allowed directories. Returns the cleaned path or an error.
func ValidateImportSourcePathWithDirs(sourcePath string, allowedDirs []string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source_path is required")
	}

	cleaned := filepath.Clean(sourcePath)
	// Resolve symlinks so that symlink traversal cannot bypass directory checks.
	// If EvalSymlinks fails (path does not exist yet), fall back to the cleaned path.
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}

	// Resolve tmpDir through symlinks as well (e.g. macOS /var/folders → /private/var/folders).
	tmpDir := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}

	within := func(dir string) bool {
		return cleaned == dir || strings.HasPrefix(cleaned, dir+string(filepath.Separator))
	}

	if within(tmpDir) {
		return cleaned, nil
	}
	for _, dir := range allowedDirs {
		if dir == "" {
			continue
		}
		resolvedDir := filepath.Clean(dir)
		if r, err := filepath.EvalSymlinks(resolvedDir); err == nil {
			resolvedDir = r
		}
		if within(resolvedDir) {
			return cleaned, nil
		}
	}

	return "", fmt.Errorf("source_path is not in an allowed import directory")
}
