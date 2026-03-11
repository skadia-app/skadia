package strava

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
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

	result := &ImportResult{}

	for _, row := range rows[1:] {
		if ctx.Err() != nil {
			break
		}
		if len(row) == 0 {
			continue
		}

		name := stravaCSVField(row, colIdx, "route name", "name")
		routeID := stravaCSVField(row, colIdx, "route id", "id")
		gpxFile := stravaCSVField(row, colIdx, "filename", "gpx filename")

		if name == "" || gpxFile == "" {
			result.Skipped++
			continue
		}

		gpxPath := "routes/" + gpxFile
		if !strings.HasSuffix(strings.ToLower(gpxPath), ".gpx") {
			gpxPath += ".gpx"
		}
		gpxBytes, err := p.Source.ReadFile(gpxPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read gpx %s: %v", gpxPath, err))
			continue
		}

		geojson, distM, elevGain, elevLoss, err := stravaParseGPXToGeoJSON(gpxBytes)
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
			100.0, // hausdorff threshold metres
			2.0,   // length diff threshold km
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
			SportType:          "cycling",
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

// stravaGPXDoc is a minimal GPX structure for track-point parsing.
type stravaGPXDoc struct {
	XMLName xml.Name `xml:"gpx"`
	Tracks  []struct {
		Segments []struct {
			Points []struct {
				Lat float64 `xml:"lat,attr"`
				Lon float64 `xml:"lon,attr"`
				Ele float64 `xml:"ele"`
			} `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
}

// stravaParseGPXToGeoJSON parses GPX bytes and returns (geojson LineString, distanceM,
// elevationGainM, elevationLossM, error).
func stravaParseGPXToGeoJSON(data []byte) (geojson string, distM, elevGain, elevLoss float64, err error) {
	var g stravaGPXDoc
	if err = xml.NewDecoder(bytes.NewReader(data)).Decode(&g); err != nil {
		return "", 0, 0, 0, fmt.Errorf("parse gpx: %w", err)
	}

	type pt struct{ lat, lon, ele float64 }
	var pts []pt
	for _, trk := range g.Tracks {
		for _, seg := range trk.Segments {
			for _, p := range seg.Points {
				pts = append(pts, pt{p.Lat, p.Lon, p.Ele})
			}
		}
	}
	if len(pts) < 2 {
		return "", 0, 0, 0, fmt.Errorf("gpx: fewer than 2 track points")
	}

	// Build GeoJSON LineString coordinates as [longitude, latitude, elevation].
	coords := make([]interface{}, 0, len(pts))
	for _, p := range pts {
		coords = append(coords, []float64{p.lon, p.lat, p.ele})
	}
	b, err := json.Marshal(map[string]interface{}{
		"type":        "LineString",
		"coordinates": coords,
	})
	if err != nil {
		return "", 0, 0, 0, err
	}

	// Compute distance and elevation gain/loss from successive track points.
	for idx := 1; idx < len(pts); idx++ {
		distM += stravaHaversineM(pts[idx-1].lat, pts[idx-1].lon, pts[idx].lat, pts[idx].lon)
		diff := pts[idx].ele - pts[idx-1].ele
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss += -diff
		}
	}

	return string(b), distM, elevGain, elevLoss, nil
}

// stravaHaversineM returns the great-circle distance in metres between two WGS-84 points.
func stravaHaversineM(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
