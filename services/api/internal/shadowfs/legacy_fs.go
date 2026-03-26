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
	fullPath := filepath.Join(basePath, relativePath)
	dir := filepath.Dir(fullPath)

	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(fullPath, originalData, 0644); err != nil {
		return fmt.Errorf("write file %s: %w", fullPath, err)
	}

	metaPath := fullPath + ".meta"
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("write meta %s: %w", metaPath, err)
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
	oldFull := filepath.Join(basePath, oldPath)
	newFull := filepath.Join(basePath, newPath)

	newDir := filepath.Dir(newFull)
	if err := EnsureDir(newDir); err != nil {
		return fmt.Errorf("create directory %s: %w", newDir, err)
	}

	if err := os.Rename(oldFull, newFull); err != nil {
		return fmt.Errorf("move %s -> %s: %w", oldFull, newFull, err)
	}

	// Move .meta sidecar if it exists
	oldMeta := oldFull + ".meta"
	if _, err := os.Stat(oldMeta); err == nil {
		newMeta := newFull + ".meta"
		if err := os.Rename(oldMeta, newMeta); err != nil {
			return fmt.Errorf("move meta %s -> %s: %w", oldMeta, newMeta, err)
		}
	}

	// Try to remove empty parent directories (best-effort)
	cleanEmptyDirs(filepath.Dir(oldFull), basePath)

	return nil
}

// DeleteShadowFile removes a shadow file and its .meta sidecar.
func DeleteShadowFile(basePath, relativePath string) error {
	if err := validatePath(basePath, relativePath); err != nil {
		return err
	}
	fullPath := filepath.Join(basePath, relativePath)

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", fullPath, err)
	}

	metaPath := fullPath + ".meta"
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete meta %s: %w", metaPath, err)
	}

	cleanEmptyDirs(filepath.Dir(fullPath), basePath)

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
