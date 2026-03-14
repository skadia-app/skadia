package garmin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ExportSource is the interface for reading Garmin export data.
type ExportSource interface {
	BaseDir() string
	ReadJSON(relativePath string, target any) error
	FindFiles(pattern string) ([]string, error)
	Exists(relativePath string) bool
}

// DirSource reads from a directory on disk.
type DirSource struct {
	baseDir string
}

// NewDirSource creates a DirSource rooted at path.
func NewDirSource(path string) *DirSource {
	return &DirSource{baseDir: path}
}

func (s *DirSource) BaseDir() string { return s.baseDir }

func (s *DirSource) ReadJSON(relativePath string, target any) error {
	data, err := os.ReadFile(filepath.Join(s.baseDir, relativePath))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (s *DirSource) FindFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.baseDir, pattern))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, m := range matches {
		rel, err := filepath.Rel(s.baseDir, m)
		if err != nil {
			continue
		}
		result = append(result, rel)
	}
	return result, nil
}

func (s *DirSource) Exists(relativePath string) bool {
	_, err := os.Stat(filepath.Join(s.baseDir, relativePath))
	return err == nil
}
