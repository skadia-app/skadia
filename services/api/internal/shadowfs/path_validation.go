package shadowfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateRelPath checks that relPath stays within basePath and contains no
// symlink escapes. It is the package-level equivalent of validateLocalPath and
// is used by handler code that operates outside of a specific ShadowFS instance.
func ValidateRelPath(basePath, relPath string) error {
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return fmt.Errorf("shadowfs: path traversal blocked: %q", relPath)
	}
	fullPath := filepath.Join(basePath, cleaned)
	if !strings.HasPrefix(fullPath, basePath+string(filepath.Separator)) && fullPath != basePath {
		return fmt.Errorf("shadowfs: path escapes basePath: %q", relPath)
	}
	// Check symlinks on existing paths.
	if _, err := os.Lstat(fullPath); err == nil {
		real, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			return fmt.Errorf("shadowfs: eval symlinks: %w", err)
		}
		if !strings.HasPrefix(real, basePath) {
			return fmt.Errorf("shadowfs: symlink escapes basePath: %q -> %q", fullPath, real)
		}
	} else {
		parentDir := filepath.Dir(fullPath)
		if _, err := os.Lstat(parentDir); err == nil {
			real, err := filepath.EvalSymlinks(parentDir)
			if err != nil {
				return fmt.Errorf("shadowfs: eval parent symlinks: %w", err)
			}
			if !strings.HasPrefix(real, basePath) {
				return fmt.Errorf("shadowfs: parent symlink escapes basePath: %q -> %q", parentDir, real)
			}
		}
	}
	return nil
}

// cleanEmptyDirs removes empty directories walking up to stopAt.
func cleanEmptyDirs(dir, stopAt string) {
	for dir != stopAt && strings.HasPrefix(dir, stopAt) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
