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
	if sourcePath == "" {
		return "", fmt.Errorf("source_path is required")
	}

	cleaned := filepath.Clean(sourcePath)
	tmpDir := os.TempDir()

	// Ensure the cleaned path is within the temp directory.
	// filepath.Clean resolves ".." segments, preventing path traversal.
	if cleaned != tmpDir && !strings.HasPrefix(cleaned, tmpDir+string(filepath.Separator)) {
		return "", fmt.Errorf("source_path must be within %s", tmpDir)
	}

	return cleaned, nil
}
