package komoot

import (
	"context"
	"fmt"
	"log/slog"

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

	// Build knownIDs for incremental sync: skip API pages where all routes are already known.
	knownIDs, err := p.DB.GetKnownKomootRouteIDs(ctx, p.UserID)
	if err != nil {
		slog.Warn("komoot routes: cannot build known route IDs set", "error", err)
		knownIDs = map[int64]bool{}
	}

	tours, err := i.Client.ListTours(ctx, knownIDs)
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
		geojson, distanceM, elevGain, elevLoss, err := importer.GPXToGeoJSON(gpxBytes)
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
		} else {
			totalDistM = distanceM
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
			TotalElevationGain: &elevGain,
			TotalElevationLoss: &elevLoss,
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
