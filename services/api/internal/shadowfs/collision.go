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
	full := filepath.Join(basePath, relativePath)
	if _, err := os.Stat(full); err != nil {
		// os.Stat failed — either the file doesn't exist (no collision) or there was
		// a permissions/IO error. In both cases we return relativePath. If a permission
		// error caused a false "no collision", the subsequent O_EXCL open in writeFile
		// will fail and trigger the race-retry loop, so no data is silently overwritten.
		return relativePath
	}

	ext := filepath.Ext(relativePath)
	stem := strings.TrimSuffix(relativePath, ext)

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(basePath, candidate)); err != nil {
			return candidate
		}
	}
	return stem + "_" + uuid.New().String()[:8] + ext
}
