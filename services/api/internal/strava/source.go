package strava

import (
	"encoding/csv"
	"fmt"
	"io"
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
	// Containment check — relativePath must stay within the export directory.
	fullPath := filepath.Join(s.Dir, relativePath)
	cleanDir := filepath.Clean(s.Dir)
	if !strings.HasPrefix(filepath.Clean(fullPath)+string(filepath.Separator), cleanDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("path traversal detected in ReadCSV: %q", relativePath)
	}
	// Size limit to prevent OOM.
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read csv %s: %w", relativePath, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxReadFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read csv %s: %w", relativePath, err)
	}
	if int64(len(raw)) > maxReadFileBytes {
		return nil, fmt.Errorf("csv file %q exceeds maximum size of %d bytes", relativePath, maxReadFileBytes)
	}
	data := raw

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

// maxReadFileBytes limits the size of files read via ReadFile to prevent OOM (500 MB).
const maxReadFileBytes = 500 << 20

func (s *DirSource) ReadFile(relativePath string) ([]byte, error) {
	// M18: Containment check — relativePath must stay within the export directory.
	fullPath := filepath.Join(s.Dir, relativePath)
	cleanDir := filepath.Clean(s.Dir)
	if !strings.HasPrefix(filepath.Clean(fullPath)+string(filepath.Separator), cleanDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("path traversal detected in ReadFile: %q", relativePath)
	}
	// M19: Size limit to prevent OOM — read limit+1 to detect truncation and error.
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReadFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxReadFileBytes {
		return nil, fmt.Errorf("file %q exceeds maximum size of %d bytes", relativePath, maxReadFileBytes)
	}
	return data, nil
}

// csvColumnAliases maps English Strava export column names to their German
// equivalents. csvHeader uses this so that all downstream code can use either
// language as map key regardless of the export's actual locale.
var csvColumnAliases = map[string]string{
	// ── activities.csv ──────────────────────────────────────────────────
	"Activity ID":              "Aktivitäts-ID",
	"Activity Date":            "Aktivitätsdatum",
	"Activity Name":            "Name der Aktivität",
	"Activity Type":            "Aktivitätsart",
	"Activity Description":     "Aktivitätsbeschreibung",
	"Elapsed Time":             "Verstrichene Zeit",
	"Distance":                 "Distanz",
	"Max Heart Rate":           "Max. Herzfrequenz",
	"Relative Effort":          "Relative Leistung",
	"Commute":                  "Pendeln",
	"Activity Private Note":    "Hinweis zur Privatsphäre für Aktivitäten",
	"Activity Gear":            "Aktivitätsausrüstung",
	"Filename":                 "Dateiname",
	"Athlete Weight":           "Sportlergewicht",
	"Bike Weight":              "Fahrradgewicht",
	"Moving Time":              "Bewegungszeit",
	"Max Speed":                "Höchstgeschw.",
	"Average Speed":            "Durchschnittliche Geschwindigkeit",
	"Elevation Gain":           "Höhenzunahme",
	"Elevation Loss":           "Höhenunterschied",
	"Elevation Low":            "Min. Höhe",
	"Elevation High":           "Max. Höhe",
	"Max Grade":                "Max. Steigung",
	"Average Grade":            "Durchschnittliche Steigung",
	"Average Positive Grade":   "Durchschnittliche positive Steigung",
	"Average Negative Grade":   "Durchschnittliche negative Steigung",
	"Max Cadence":              "Max. Tritt-/Schrittfrequenz",
	"Average Cadence":          "Durchschnittliche Trittfrequenz",
	"Average Heart Rate":       "Durchschnittliche Herzfrequenz",
	"Max Watts":                "Max. Watt",
	"Average Watts":            "Durchschnittliche Watt",
	"Calories":                 "Kalorien",
	"Max Temperature":          "Max. Temperatur",
	"Average Temperature":      "Durchschnittliche Temperatur",
	"Total Work":               "Gesamtarbeit",
	"Number of Runs":           "Anzahl Läufe",
	"Uphill Time":              "Bergaufzeit",
	"Downhill Time":            "Bergabzeit",
	"Other Time":               "Andere Zeit",
	"Perceived Exertion":       "Gefühlte Anstrengung",
	"Type":                     "Art",
	"Start Time":               "Startzeit",
	"Weighted Average Power":   "Gewichtete durchschnittliche Leistung",
	"Power Count":              "Leistungszahl",
	"Total Weight Lifted":      "Insgesamt gestemmtes Gewicht",
	"Grade Adjusted Distance":  "Auf Steigung angepasste Distanz",
	"Weather Observation Time": "Wetterbeobachtungszeit",
	"Weather Condition":        "Wetterlage",
	"Weather Temperature":      "Wetter: Temperatur",
	"Apparent Temperature":     "Scheinbare Temperatur",
	"Dewpoint":                 "Taupunkt",
	"Humidity":                 "Luftfeuchtigkeit",
	"Weather Pressure":         "Wetter: Druck",
	"Wind Speed":               "Windgeschwindigkeit",
	"Wind Gust":                "Windböe",
	"Wind Bearing":             "Windrichtung",
	"Precipitation Intensity":  "Niederschlagsintensität",
	"Sunrise Time":             "Sonnenaufgangszeit",
	"Sunset Time":              "Sonnenuntergangszeit",
	"Moon Phase":               "Mondphase",
	"Bike":                     "Fahrrad",
	"Gear":                     "Ausrüstung",
	"Training Load":            "Trainingsbelastung",
	"Intensity":                "Intensität",
	"Pool Length":               "Pool-Länge",
	"Total Steps":              "Schritte insgesamt",
	"Media":                    "Medien",
	// ── bikes.csv ───────────────────────────────────────────────────────
	"Bike Name":                 "Fahrradname",
	"Bike Brand":                "Fahrradmarke",
	"Bike Model":                "Fahrradmodell",
	"Bike Default Sport Types":  "Fahrrad-Standardsportarten",
	// ── shoes.csv ───────────────────────────────────────────────────────
	"Shoe Name":                 "Schuhname",
	"Shoe Brand":                "Schuhmarke",
	"Shoe Model":                "Schuhmodell",
	"Shoe Default Sport Types":  "Schuh-Standardsportarten",
	// ── components.csv ──────────────────────────────────────────────────
	"Component Type":            "Komponentenart",
	"Component Brand":           "Komponentenmarke",
	"Component Model":           "Komponentenmodell",
	// ── profile.csv ─────────────────────────────────────────────────────
	"Sex":                       "Geschlecht",
	"Weight":                    "Gewicht",
	// ── general_preferences.csv ─────────────────────────────────────────
	"Date of Birth":                 "Geburtsdatum",
	"Functional Threshold Power":    "Funktionale Schwellenleistung",
	"Maximum Heartrate":             "Maximale Herzfrequenz",
	// ── privacy_zones.csv ───────────────────────────────────────────────
	"Address":                       "Adresse",
	"Latitude & Longitude":          "Breitengrad & Längengrad",
	// ── media.csv ───────────────────────────────────────────────────────
	"Media Filename":                "Medien-Dateiname",
}

// CSVHeader builds a column-name → column-index map from a header row.
// It populates both the original column name AND the alias from the other
// language so that downstream code works regardless of the export's locale.
// Exported for use by the handler scan functions.
func CSVHeader(row []string) map[string]int {
	h := make(map[string]int, len(row)*2)
	for i, col := range row {
		col = strings.TrimSpace(col)
		col = strings.TrimPrefix(col, "\xef\xbb\xbf")
		h[col] = i
	}
	// Add cross-language aliases. For duplicate column names (e.g. "Distance"
	// appears at col 6 and col 17), the map already holds the LAST index — the
	// alias mirrors that same index, matching the existing behaviour.
	for en, de := range csvColumnAliases {
		if idx, ok := h[en]; ok {
			if _, exists := h[de]; !exists {
				h[de] = idx
			}
		}
		if idx, ok := h[de]; ok {
			if _, exists := h[en]; !exists {
				h[en] = idx
			}
		}
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

// parseNumber parses numeric strings in either German (comma decimal, dot
// thousand separator) or English (dot decimal, comma thousand separator) format.
// Heuristic: if the string contains both '.' and ',' the LAST separator is the
// decimal mark. If only one separator type exists, commas are treated as decimal
// (German) when followed by ≤2 digits at end, otherwise as thousand separators.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0, false
	}

	hasComma := strings.Contains(s, ",")
	hasDot := strings.Contains(s, ".")

	switch {
	case hasComma && hasDot:
		// Both separators present — last one is the decimal mark.
		lastComma := strings.LastIndex(s, ",")
		lastDot := strings.LastIndex(s, ".")
		if lastComma > lastDot {
			// German: "1.234,56" → remove dots, comma→dot
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			// English: "1,234.56" → remove commas
			s = strings.ReplaceAll(s, ",", "")
		}
	case hasComma:
		// Only commas. "1,384" (thousand sep) vs "10,52" (decimal).
		// Heuristic: if exactly 3 digits after the LAST comma → thousand separator.
		lastComma := strings.LastIndex(s, ",")
		afterComma := s[lastComma+1:]
		if len(afterComma) == 3 && !strings.ContainsAny(afterComma, ".,") {
			// Thousand separator — just remove
			s = strings.ReplaceAll(s, ",", "")
		} else {
			// Decimal comma (German)
			s = strings.ReplaceAll(s, ",", ".")
		}
	// hasDot only: standard float, nothing to do
	}

	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err == nil
}

// parseGermanFloat is an alias for parseNumber, kept for readability in code
// that historically assumed German-only input.
func parseGermanFloat(s string) (float64, bool) { return parseNumber(s) }

// parseInt parses a number string to int, handling both locales.
func parseInt(s string) (int, bool) {
	f, ok := parseNumber(s)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// parseGermanInt is an alias for parseInt.
func parseGermanInt(s string) (int, bool) { return parseInt(s) }

// findNthColMulti finds the nth (1-based) occurrence of any of the given column
// names in the raw header row. Useful for columns that appear multiple times
// (e.g. "Distance"/"Distanz" at cols 6 and 17).
func findNthColMulti(headerRow []string, n int, names ...string) int {
	count := 0
	for i, h := range headerRow {
		h = strings.TrimSpace(h)
		for _, name := range names {
			if h == name {
				count++
				if count == n {
					return i
				}
				break
			}
		}
	}
	return -1
}

// parseWeightToKg parses weight strings like "84,0 kg", "180.0 lb", or plain
// numbers. Returns weight in kilograms.
func parseWeightToKg(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	isLb := strings.HasSuffix(lower, "lbs") || strings.HasSuffix(lower, "lb")
	// Strip known unit suffixes (longest first to avoid partial strips)
	for _, suffix := range []string{" lbs", " lb", "lbs", "lb", " kg", "kg"} {
		s = strings.TrimSuffix(s, suffix)
	}
	s = strings.TrimSpace(s)

	w, ok := parseNumber(s)
	if !ok || w <= 0 {
		return 0, false
	}
	if isLb {
		w *= 0.45359237
	}
	return w, true
}
