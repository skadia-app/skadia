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
	cleanBase := filepath.Clean(basePath)
	if realBase, err := filepath.EvalSymlinks(cleanBase); err == nil {
		cleanBase = realBase
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
		return fmt.Errorf("shadowfs: path traversal blocked: %q", relPath)
	}
	fullPath := filepath.Join(cleanBase, cleaned)
	if fullPath != cleanBase && !strings.HasPrefix(fullPath, cleanBase+string(filepath.Separator)) {
		return fmt.Errorf("shadowfs: path escapes basePath: %q", relPath)
	}

	// Verify existing paths (and existing parent directories) cannot escape
	// cleanBase via symlinks using kernel-enforced os.Root when cleanBase exists.
	root, err := os.OpenRoot(cleanBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("shadowfs: open basePath: %w", err)
	}
	defer root.Close()

	if cleaned == "." {
		return nil
	}
	// Walk from cleaned up to the nearest existing ancestor inside root; os.Root
	// rejects any symlink component that resolves outside cleanBase.
	cur := cleaned
	for cur != "." && cur != "" {
		info, statErr := root.Stat(cur)
		if statErr == nil {
			_ = info
			return nil
		}
		if !os.IsNotExist(statErr) {
			return fmt.Errorf("shadowfs: path escapes basePath or unreadable: %w", statErr)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
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
