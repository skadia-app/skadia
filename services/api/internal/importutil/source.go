package importutil

import (
	"os"
	"path/filepath"
)

// ExportSource is the common base interface for reading export data.
// Platform-specific interfaces (garmin.ExportSource, strava.ExportSource) embed
// this and add their own methods (ReadJSON, ReadCSV, etc.).
type ExportSource interface {
	BaseDir() string
	FindFiles(pattern string) ([]string, error)
	Exists(relativePath string) bool
}

// DirSource reads from a directory on disk.
// Platform-specific sources embed this and add format-specific readers.
type DirSource struct {
	Dir string
}

// NewDirSource creates a DirSource rooted at path.
func NewDirSource(path string) DirSource {
	return DirSource{Dir: path}
}

func (s DirSource) BaseDir() string { return s.Dir }

func (s DirSource) FindFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.Dir, pattern))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, m := range matches {
		rel, err := filepath.Rel(s.Dir, m)
		if err != nil {
			continue
		}
		result = append(result, rel)
	}
	return result, nil
}

func (s DirSource) Exists(relativePath string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, relativePath))
	return err == nil
}
