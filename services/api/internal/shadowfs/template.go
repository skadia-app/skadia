package shadowfs

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ActivityInfo holds the data needed to render a shadow file path.
type ActivityInfo struct {
	ID           string
	StartedAt    time.Time
	SportType    string
	SubCategory  string // sport sub-type / sub-category (may be empty)
	Title        string
	DistanceM    float64
	ElapsedTimeS int
	City         string
	FileExt      string // e.g. "fit", "gpx", "tcx"
}

// illegalChars matches characters not allowed in file/directory names.
var illegalChars = regexp.MustCompile(`[/\\:*?"<>|]`)

// sanitize replaces illegal filesystem characters with underscores and trims whitespace.
func sanitize(s string) string {
	s = illegalChars.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	return s
}

// RenderPath takes an ActivityInfo and a schema template and returns the rendered
// relative file path (without base path). The path includes the file extension.
//
// Supported placeholders:
//
//	{year}, {month}, {day}, {time} - from StartedAt
//	{sport} - sport type
//	{subCategory} - sport sub-category / sub-type (may be empty → "general")
//	{city} - city name (may be empty)
//	{title} - activity title (may be empty)
//	{distance_km} - distance in km (1 decimal)
//	{duration_h} - duration in h:mm format
func RenderPath(info ActivityInfo, schema string) string {
	t := info.StartedAt

	year := t.Format("2006")
	month := t.Format("01")
	day := t.Format("02")
	timeStr := t.Format("15-04")

	sport := info.SportType
	if sport == "" {
		sport = "unknown"
	}

	subCategory := info.SubCategory
	if subCategory == "" {
		subCategory = "general"
	}

	city := info.City
	if city == "" {
		city = "unknown"
	}

	title := info.Title
	if title == "" {
		title = sport
	}

	distKM := info.DistanceM / 1000.0
	distStr := fmt.Sprintf("%.1f", math.Round(distKM*10)/10)

	hours := info.ElapsedTimeS / 3600
	mins := (info.ElapsedTimeS % 3600) / 60
	durStr := fmt.Sprintf("%d-%02d", hours, mins)

	result := schema
	result = strings.ReplaceAll(result, "{year}", sanitize(year))
	result = strings.ReplaceAll(result, "{month}", sanitize(month))
	result = strings.ReplaceAll(result, "{day}", sanitize(day))
	result = strings.ReplaceAll(result, "{time}", sanitize(timeStr))
	result = strings.ReplaceAll(result, "{sport}", sanitize(sport))
	result = strings.ReplaceAll(result, "{subCategory}", sanitize(subCategory))
	result = strings.ReplaceAll(result, "{city}", sanitize(city))
	result = strings.ReplaceAll(result, "{title}", sanitize(title))
	result = strings.ReplaceAll(result, "{distance_km}", distStr)
	result = strings.ReplaceAll(result, "{duration_h}", durStr)

	// Add file extension
	ext := info.FileExt
	if ext == "" {
		ext = "fit"
	}
	result = result + "." + ext

	return result
}

// ResolveCollision appends _1, _2 etc. to the file stem if a file already
// exists at the given basePath + relativePath.
func ResolveCollision(basePath, relativePath string) string {
	full := filepath.Join(basePath, relativePath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return relativePath
	}

	ext := filepath.Ext(relativePath)
	stem := strings.TrimSuffix(relativePath, ext)

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		candidateFull := filepath.Join(basePath, candidate)
		if _, err := os.Stat(candidateFull); os.IsNotExist(err) {
			return candidate
		}
	}

	// Extreme fallback: use activity ID in filename (should never happen)
	return stem + "_overflow" + ext
}

// DefaultSchema is the default file naming schema.
const DefaultSchema = "{year}-{month}-{day}_{sport}_{subCategory}_{title}_{city}"
