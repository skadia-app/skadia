package komoot

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RoutesImporter imports planned tours (tour_planned) from Komoot as planned routes.
type RoutesImporter struct {
	Client *Client
}

func (i *RoutesImporter) Name() string        { return "routes" }
func (i *RoutesImporter) Description() string { return "Geplante Routen" }
func (i *RoutesImporter) Priority() int       { return 30 }

// Import iterates all tour_planned tours and imports them as planned routes.
func (i *RoutesImporter) Import(ctx context.Context, p KomootImportParams) (*KomootImportResult, error) {
	result := &KomootImportResult{TourActivityMap: make(map[int64]string)}

	tours, err := i.Client.ListTours(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("komoot routes: list tours: %w", err)
	}

	// Filter to tour_planned only
	var planned []Tour
	for _, t := range tours {
		if t.Type == "tour_planned" {
			planned = append(planned, t)
		}
	}

	// Get user enrichment settings for duplicate detection thresholds
	hausdorffM := 100.0
	lengthDiffKm := 2.0
	settings, err := p.DB.GetEnrichmentSettings(ctx, p.UserID)
	if err != nil {
		slog.Warn("komoot routes: cannot get enrichment settings, using defaults", "error", err)
	} else {
		hausdorffM = settings.RouteDuplicateHausdorffM
		lengthDiffKm = settings.RouteDuplicateLengthDiffKm
	}

	total := len(planned)
	slog.Info("komoot routes: found planned tours", "count", total)

	for idx, tour := range planned {
		if p.OnProgress != nil {
			p.OnProgress(idx+1, total, fmt.Sprintf("routes: %d/%d %s", idx+1, total, tour.Name))
		}

		// Download GPX
		gpxBytes, err := i.Client.DownloadGPX(ctx, tour.ID)
		if err != nil {
			slog.Warn("komoot routes: GPX download failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("gpx download tour %d: %s", tour.ID, err))
			continue
		}

		// Parse GPX → GeoJSON + metrics
		geojson, distanceM, elevGain, elevLoss, err := gpxToGeoJSON(gpxBytes)
		if err != nil || geojson == "" {
			slog.Warn("komoot routes: GPX parse failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("gpx parse tour %d: %s", tour.ID, err))
			continue
		}

		// Sport mapping
		sportType := "hiking"
		if mapping, ok := KomootSportMapping[tour.Sport]; ok {
			sportType = mapping.SportType
		}

		// Source info
		sourceID := fmt.Sprintf("%d", tour.ID)
		sourcePlatform := "komoot"
		sourceURL := fmt.Sprintf("https://www.komoot.com/tour/%d", tour.ID)

		// Total distance: prefer tour.Distance, fallback to calculated
		var totalDistM float64
		if tour.Distance > 0 {
			totalDistM = tour.Distance
		} else if distanceM != nil {
			totalDistM = *distanceM
		}
		totalDistPtr := &totalDistM

		// Duplicate check (3-tier)
		dup, err := importer.CheckRouteDuplicate(
			ctx, p.DB, p.UserID,
			sourcePlatform, sourceID,
			geojson,
			totalDistM,
			hausdorffM,
			lengthDiffKm,
		)
		if err != nil {
			slog.Warn("komoot routes: duplicate check failed", "tour_id", tour.ID, "error", err)
		}
		if dup != nil {
			slog.Debug("komoot routes: duplicate found", "tour_id", tour.ID, "existing_id", dup.ExistingRouteID, "confidence", dup.Confidence)
			result.Skipped++
			continue
		}

		// Create route
		profile := "outdoor" // Komoot doesn't expose profile types via API
		routeData := &model.RouteImportData{
			Name:               tour.Name,
			Description:        nil,
			SportType:          sportType,
			Profile:            profile,
			GeometryGeoJSON:    geojson,
			WaypointsJSON:      "[]",
			SegmentsJSON:       "[]",
			TotalDistanceM:     totalDistPtr,
			TotalElevationGain: elevGain,
			TotalElevationLoss: elevLoss,
			SourcePlatform:     &sourcePlatform,
			SourceID:           &sourceID,
			SourceURL:          &sourceURL,
		}

		routeID, err := p.DB.CreateRouteFromImport(ctx, p.UserID, routeData)
		if err != nil {
			slog.Warn("komoot routes: create route failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("create route tour %d: %s", tour.ID, err))
			continue
		}

		slog.Debug("komoot routes: route created", "tour_id", tour.ID, "route_id", routeID)
		result.Imported++
	}

	slog.Info("komoot routes import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// --- GPX parsing ---

type gpxDoc struct {
	XMLName xml.Name   `xml:"gpx"`
	Tracks  []gpxTrack `xml:"trk"`
	Routes  []gpxRoute `xml:"rte"`
}

type gpxTrack struct {
	Segments []gpxTrackSeg `xml:"trkseg"`
}

type gpxTrackSeg struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxRoute struct {
	Points []gpxPoint `xml:"rtept"`
}

type gpxPoint struct {
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
	Ele float64 `xml:"ele"`
}

// gpxToGeoJSON parses GPX bytes and returns GeoJSON LineString + metrics.
// Handles both trk/trkseg/trkpt (recorded) and rte/rtept (planned routes).
// Returns 3D coordinates [lng, lat, ele] per RFC 7946 uniform dimensionality.
func gpxToGeoJSON(gpxBytes []byte) (string, *float64, *float64, *float64, error) {
	var doc gpxDoc
	if err := xml.Unmarshal(gpxBytes, &doc); err != nil {
		return "", nil, nil, nil, fmt.Errorf("gpx parse: %w", err)
	}

	// Collect all points (prefer track points, fall back to route points)
	var points []gpxPoint
	for _, trk := range doc.Tracks {
		for _, seg := range trk.Segments {
			points = append(points, seg.Points...)
		}
	}
	if len(points) == 0 {
		for _, rte := range doc.Routes {
			points = append(points, rte.Points...)
		}
	}
	if len(points) < 2 {
		return "", nil, nil, nil, fmt.Errorf("gpx has fewer than 2 track points")
	}

	// Build coordinate array [[lng, lat, ele], ...]
	coords := make([][]float64, 0, len(points))
	for _, p := range points {
		coords = append(coords, []float64{p.Lon, p.Lat, p.Ele})
	}

	gj := map[string]interface{}{
		"type":        "LineString",
		"coordinates": coords,
	}
	gjBytes, err := json.Marshal(gj)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("geojson marshal: %w", err)
	}

	// Calculate distance (Haversine) and elevation gain/loss
	var distM, elevGain, elevLoss float64
	for i := 1; i < len(points); i++ {
		distM += haversineM(points[i-1].Lat, points[i-1].Lon, points[i].Lat, points[i].Lon)
		dEle := points[i].Ele - points[i-1].Ele
		if dEle > 0 {
			elevGain += dEle
		} else {
			elevLoss += -dEle
		}
	}

	return string(gjBytes), &distM, &elevGain, &elevLoss, nil
}

// haversineM returns the great-circle distance in meters between two WGS84 coordinates.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0 // Earth radius in meters
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLam := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLam/2)*math.Sin(dLam/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
