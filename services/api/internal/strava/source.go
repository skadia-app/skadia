package strava

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

// ExportSource is the interface for reading Strava export data.
type ExportSource interface {
	importutil.ExportSource
	ReadCSV(relativePath string) ([][]string, error)
	ReadFile(relativePath string) ([]byte, error)
}

// DirSource reads from a directory on disk.
type DirSource struct {
	importutil.DirSource
}

// NewDirSource creates a DirSource rooted at path.
func NewDirSource(path string) *DirSource {
	return &DirSource{DirSource: importutil.NewDirSource(path)}
}

// ReadCSV reads a CSV file and returns all rows (including header as row 0).
// Handles the BOM that some Strava exports include.
func (s *DirSource) ReadCSV(relativePath string) ([][]string, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, relativePath))
	if err != nil {
		return nil, fmt.Errorf("read csv %s: %w", relativePath, err)
	}

	// Strip UTF-8 BOM if present
	content := string(data)
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")

	r := csv.NewReader(strings.NewReader(content))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv %s: %w", relativePath, err)
	}
	return rows, nil
}

func (s *DirSource) ReadFile(relativePath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Dir, relativePath))
}

// csvHeader builds a column-name → column-index map from a header row.
func csvHeader(row []string) map[string]int {
	h := make(map[string]int, len(row))
	for i, col := range row {
		col = strings.TrimSpace(col)
		col = strings.TrimPrefix(col, "\xef\xbb\xbf")
		h[col] = i
	}
	return h
}

// colVal safely returns the value at the given column index (empty string if out of bounds).
func colVal(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// parseGermanFloat parses a number like "10,52" or "10.52" to float64.
func parseGermanFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0, false
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err == nil
}

// parseGermanInt parses a German-formatted number string to int.
func parseGermanInt(s string) (int, bool) {
	f, ok := parseGermanFloat(s)
	if !ok {
		return 0, false
	}
	return int(f), true
}
