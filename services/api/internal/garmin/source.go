package garmin

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

// ExportSource is the interface for reading Garmin export data.
type ExportSource interface {
	importutil.ExportSource
	ReadJSON(relativePath string, target any) error
}

// DirSource reads from a directory on disk.
type DirSource struct {
	importutil.DirSource
}

// NewDirSource creates a DirSource rooted at path.
func NewDirSource(path string) *DirSource {
	return &DirSource{DirSource: importutil.NewDirSource(path)}
}

func (s *DirSource) ReadJSON(relativePath string, target any) error {
	data, err := os.ReadFile(filepath.Join(s.Dir, relativePath))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
