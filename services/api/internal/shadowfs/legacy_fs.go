// Legacy ShadowFS functions retained for backward compatibility with the hotfolder watcher
// All functions here are retained for backward compatibility with the hotfolder watcher
// and the legacy rename handlers. Do not add new code to this package.
package shadowfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ShadowFSConfig holds the configuration for the legacy shadow file system.
type ShadowFSConfig struct {
	BasePath         string
	HotfolderPath    string
	HotfolderEnabled bool
}

// ShadowMeta is the metadata written alongside each shadow file.
type ShadowMeta struct {
	SchemaVersion    int    `json:"schema_version"`
	SkadiaID        string `json:"skadia_id"`
	FileHashSHA256   string `json:"file_hash_sha256"`
	OriginalFilename string `json:"original_filename"`
	FileFormat       string `json:"file_format"`
	SportType        string `json:"sport_type"`
	UserID           string `json:"user_id"`
}

// validatePath ensures relativePath stays within basePath to prevent path traversal.
// Delegates to ValidateRelPath (local.go) which also checks symlink escapes.
func validatePath(basePath, relativePath string) error {
	return ValidateRelPath(basePath, relativePath)
}

// EnsureDir creates all directories along the given path.
func EnsureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// WriteShadowFile writes the original activity file and a .meta JSON sidecar
// to basePath/relativePath. It creates any necessary intermediate directories.
func WriteShadowFile(basePath, relativePath string, originalData []byte, metaData []byte) error {
	if err := validatePath(basePath, relativePath); err != nil {
		return err
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return fmt.Errorf("create base directory %s: %w", basePath, err)
	}
	root, err := os.OpenRoot(basePath)
	if err != nil {
		return fmt.Errorf("open base directory %s: %w", basePath, err)
	}
	defer root.Close()

	rel := filepath.Clean(relativePath)
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	if err := root.WriteFile(rel, originalData, 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", rel, err)
	}

	metaRel := rel + ".meta"
	if err := root.WriteFile(metaRel, metaData, 0o644); err != nil {
		return fmt.Errorf("write meta %s: %w", metaRel, err)
	}

	return nil
}

// MoveShadowFile moves a shadow file and its .meta sidecar from oldPath to newPath.
// Both paths are relative to basePath.
func MoveShadowFile(basePath, oldPath, newPath string) error {
	if err := validatePath(basePath, oldPath); err != nil {
		return err
	}
	if err := validatePath(basePath, newPath); err != nil {
		return err
	}
	root, err := os.OpenRoot(basePath)
	if err != nil {
		return fmt.Errorf("open base directory %s: %w", basePath, err)
	}
	defer root.Close()

	oldRel := filepath.Clean(oldPath)
	newRel := filepath.Clean(newPath)

	if newDir := filepath.Dir(newRel); newDir != "." {
		if err := root.MkdirAll(newDir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", newDir, err)
		}
	}

	if err := root.Rename(oldRel, newRel); err != nil {
		return fmt.Errorf("move %s -> %s: %w", oldRel, newRel, err)
	}

	// Move .meta sidecar if it exists
	oldMeta := oldRel + ".meta"
	if _, err := root.Stat(oldMeta); err == nil {
		newMeta := newRel + ".meta"
		if err := root.Rename(oldMeta, newMeta); err != nil {
			return fmt.Errorf("move meta %s -> %s: %w", oldMeta, newMeta, err)
		}
	}

	// Try to remove empty parent directories (best-effort)
	cleanEmptyDirs(filepath.Dir(filepath.Join(basePath, oldRel)), basePath)

	return nil
}

// DeleteShadowFile removes a shadow file and its .meta sidecar.
func DeleteShadowFile(basePath, relativePath string) error {
	if err := validatePath(basePath, relativePath); err != nil {
		return err
	}
	root, err := os.OpenRoot(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open base directory %s: %w", basePath, err)
	}
	defer root.Close()

	rel := filepath.Clean(relativePath)
	if err := root.Remove(rel); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", rel, err)
	}

	metaRel := rel + ".meta"
	if err := root.Remove(metaRel); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete meta %s: %w", metaRel, err)
	}

	cleanEmptyDirs(filepath.Dir(filepath.Join(basePath, rel)), basePath)

	return nil
}

// BuildShadowMeta creates a JSON-encoded metadata blob for the shadow file.
func BuildShadowMeta(activityID, fileHash, originalFilename, fileFormat, sportType, userID string) ([]byte, error) {
	meta := ShadowMeta{
		SchemaVersion:    1,
		SkadiaID:        activityID,
		FileHashSHA256:   fileHash,
		OriginalFilename: originalFilename,
		FileFormat:       fileFormat,
		SportType:        sportType,
		UserID:           userID,
	}
	return json.MarshalIndent(meta, "", "  ")
}
