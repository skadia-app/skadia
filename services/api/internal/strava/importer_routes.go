package strava

import (
	"context"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RoutesImporter imports planned routes from a Strava export (routes.csv + routes/*.gpx).
type RoutesImporter struct{}

func (i *RoutesImporter) Name() string        { return "routes" }
func (i *RoutesImporter) Description() string { return "Geplante Routen" }
func (i *RoutesImporter) Priority() int       { return 50 }

func (i *RoutesImporter) Detect(src ExportSource) (bool, int, error) {
	if !src.Exists("routes.csv") {
		return false, 0, nil
	}
	rows, err := src.ReadCSV("routes.csv")
	if err != nil {
		return false, 0, err
	}
	if len(rows) <= 1 {
		return false, 0, nil // header only, no routes
	}
	return true, len(rows) - 1, nil
}

func (i *RoutesImporter) Import(ctx context.Context, p ImportParams) (*ImportResult, error) {
	rows, err := p.Source.ReadCSV("routes.csv")
	if err != nil {
		return nil, fmt.Errorf("routes: read csv: %w", err)
	}
	if len(rows) <= 1 {
		return &ImportResult{}, nil
	}

	// Build column index map from header row (case-insensitive, trimmed).
	header := rows[0]
	colIdx := make(map[string]int, len(header))
	for j, h := range header {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = j
	}

	settings, err := p.DB.GetEnrichmentSettings(ctx, p.UserID)
	if err != nil {
		settings = &model.EnrichmentSettings{
			RouteDuplicateHausdorffM:   100.0,
			RouteDuplicateLengthDiffKm: 2.0,
		}
	}

	result := &ImportResult{}

	for _, row := range rows[1:] {
		if ctx.Err() != nil {
			break
		}
		if len(row) == 0 {
			continue
		}

		name := stravaCSVField(row, colIdx, "route name", "routenname", "name")
		routeID := stravaCSVField(row, colIdx, "route id", "routenid", "id")
		gpxFile := stravaCSVField(row, colIdx, "filename", "routendateiname", "gpx filename")
		csvSport := stravaCSVField(row, colIdx, "sport type", "sportart", "type", "activity type")
		sportType := stravaRouteTypeToSport(csvSport)

		if name == "" || gpxFile == "" {
			result.Skipped++
			continue
		}

		gpxPath := gpxFile
		if !strings.HasPrefix(gpxPath, "routes/") {
			gpxPath = "routes/" + gpxPath
		}
		if !strings.HasSuffix(strings.ToLower(gpxPath), ".gpx") {
			gpxPath += ".gpx"
		}
		gpxBytes, err := p.Source.ReadFile(gpxPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read gpx %s: %v", gpxPath, err))
			continue
		}

		geojson, distM, elevGain, elevLoss, err := importer.GPXToGeoJSON(gpxBytes)
		if err != nil || geojson == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("parse gpx %s: %v", gpxPath, err))
			continue
		}

		sourceID := routeID
		if sourceID == "" {
			sourceID = name
		}
		const platform = "strava"

		dup, err := importer.CheckRouteDuplicate(
			ctx, p.DB, p.UserID,
			platform, sourceID,
			geojson, distM,
			settings.RouteDuplicateHausdorffM,
			settings.RouteDuplicateLengthDiffKm,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("dup check %s: %v", name, err))
			continue
		}
		if dup != nil {
			result.Skipped++
			continue
		}

		sourcePlatform := platform
		sid := sourceID
		d := distM
		eg := elevGain
		el := elevLoss

		_, err = p.DB.CreateRouteFromImport(ctx, p.UserID, &model.RouteImportData{
			Name:               name,
			SportType:          sportType,
			Profile:            "outdoor",
			GeometryGeoJSON:    geojson,
			WaypointsJSON:      "[]",
			SegmentsJSON:       "[]",
			TotalDistanceM:     &d,
			TotalElevationGain: &eg,
			TotalElevationLoss: &el,
			SourcePlatform:     &sourcePlatform,
			SourceID:           &sid,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("create route %s: %v", name, err))
			continue
		}
		result.Imported++
	}

	return result, nil
}

// stravaCSVField looks up a field value by trying multiple lowercased header name candidates.
func stravaCSVField(row []string, colIdx map[string]int, candidates ...string) string {
	for _, c := range candidates {
		if idx, ok := colIdx[c]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}
	return ""
}

// stravaRouteTypeToSport maps Strava CSV sport/type values to Skadia sport_type.
func stravaRouteTypeToSport(t string) string {
	switch strings.ToLower(t) {
	case "run", "running":
		return "running"
	case "trail run", "trail running", "trailrun":
		return "trail_running"
	case "ride", "cycling", "bike":
		return "cycling"
	case "hike", "hiking":
		return "hiking"
	case "walk", "walking":
		return "walking"
	case "alpine ski", "skiing":
		return "alpine_skiing"
	default:
		return "cycling" // Strava's most common route type
	}
}
