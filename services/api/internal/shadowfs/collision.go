package shadowfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveCollision appends _1, _2 etc. to the file stem if a file already
// exists at basePath + relativePath. Returns the collision-free relative path.
func ResolveCollision(basePath, relativePath string) string {
	full := filepath.Join(basePath, relativePath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return relativePath
	}

	ext := filepath.Ext(relativePath)
	stem := strings.TrimSuffix(relativePath, ext)

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(basePath, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	return stem + "_overflow" + ext
}
