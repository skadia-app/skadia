package shadowfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ResolveCollision appends _1, _2 etc. to the file stem if a file already
// exists at basePath + relativePath. Returns the collision-free relative path.
func ResolveCollision(basePath, relativePath string) string {
	cleaned := filepath.Clean(relativePath)
	if cleaned == "" || cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return relativePath
	}
	root, err := os.OpenRoot(basePath)
	if err != nil {
		return relativePath
	}
	defer root.Close()

	if _, err := root.Stat(cleaned); err != nil {
		// Stat failed — either the file doesn't exist (no collision) or there was
		// a permissions/IO error. In both cases we return relativePath. If a permission
		// error caused a false "no collision", the subsequent O_EXCL open in writeFile
		// will fail and trigger the race-retry loop, so no data is silently overwritten.
		return relativePath
	}

	ext := filepath.Ext(relativePath)
	stem := strings.TrimSuffix(relativePath, ext)

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := root.Stat(filepath.Clean(candidate)); err != nil {
			return candidate
		}
	}
	return stem + "_" + uuid.New().String()[:8] + ext
}
