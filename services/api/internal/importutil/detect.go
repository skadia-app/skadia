package importutil

import (
	"path/filepath"
	"strings"
)

// DetectFormat identifies the activity file format from raw bytes and filename.
// Returns "fit", "gpx", "tcx", or "" if unknown.
func DetectFormat(data []byte, filename string) string {
	// FIT: The header byte encodes its own length (12 or 14) and the last
	// four bytes of the header must contain the ".FIT" signature.
	if len(data) >= 14 {
		headerSize := int(data[0])
		validHeader := (headerSize == 12 || headerSize == 14) && headerSize <= len(data)
		if validHeader && string(data[headerSize-4:headerSize]) == ".FIT" {
			return "fit"
		}
	}

	// XML-based formats
	content := string(data[:min(len(data), 1024)])
	lower := strings.ToLower(content)

	if strings.Contains(lower, "<gpx") {
		return "gpx"
	}
	if strings.Contains(lower, "<trainingcenterdatabase") {
		return "tcx"
	}

	// Fallback: check extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".fit":
		return "fit"
	case ".gpx":
		return "gpx"
	case ".tcx":
		return "tcx"
	}

	return ""
}
