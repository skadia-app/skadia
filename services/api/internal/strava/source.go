package strava

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportSource is the interface for reading Strava export data.
type ExportSource interface {
	BaseDir() string
	ReadCSV(relativePath string) ([][]string, error)
	FindFiles(pattern string) ([]string, error)
	Exists(relativePath string) bool
	ReadFile(relativePath string) ([]byte, error)
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

// ReadCSV reads a CSV file and returns all rows (including header as row 0).
// Handles the BOM that some Strava exports include.
func (s *DirSource) ReadCSV(relativePath string) ([][]string, error) {
	data, err := os.ReadFile(filepath.Join(s.baseDir, relativePath))
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

func (s *DirSource) ReadFile(relativePath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.baseDir, relativePath))
}

// csvHeader builds a column-name → column-index map from a header row.
func csvHeader(row []string) map[string]int {
	h := make(map[string]int, len(row))
	for i, col := range row {
		// Trim BOM and whitespace from individual cell values
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
	// Remove German thousand separators (.) if followed by more digits
	// e.g. "1.234,56" → "1234.56"
	// Strategy: if there is a comma, the comma is the decimal separator
	if strings.Contains(s, ",") {
		// Remove dots used as thousands separators
		s = strings.ReplaceAll(s, ".", "")
		// Replace decimal comma with dot
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
