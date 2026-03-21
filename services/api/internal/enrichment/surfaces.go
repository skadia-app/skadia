package enrichment

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// EnrichSurfaces matches the activity track against OSM ways and writes
// surface-type + highway-type statistics to activity_surfaces.
//
// Two paths:
//   - Graph (when graphReader covers the track): Viterbi on Tiled Graph, ~10ms
//   - SQL (fallback): Nearest-Way per resampled point, ~100ms
//
// graphReader may be nil (SQL-only mode).
// stream may be nil (SQL-only mode — SQL uses activity_tracks directly).
//
// Each resampled point gets exactly one classification:
//   - surface tag present → concrete type (asphalt, gravel, etc.)
//   - way matched but no surface tag → "unknown"
//   - no way within 25m → "offroad"
//
// Percentages always sum to 100%.
func EnrichSurfaces(ctx context.Context, db *database.DB, activityID string, enabled bool,
	graphReader routing.GraphReader, stream []EnrichmentStreamPoint) error {

	if !enabled {
		return nil
	}

	// Graph path: preferred when tiles cover the track
	if graphReader != nil && len(stream) > 0 {
		bbox := streamBBox(stream)
		if graphReader.HasTilesFor(bbox.minLat, bbox.minLon, bbox.maxLat, bbox.maxLon) {
			if err := enrichSurfacesGraph(ctx, db, graphReader.GraphAt, activityID, stream); err != nil {
				slog.Warn("surfaces: graph path failed, falling back to SQL",
					"activity_id", activityID, "error", err)
			} else {
				return nil
			}
		}
	}

	// SQL path (fallback)
	return enrichSurfacesSQL(ctx, db, activityID)
}

type bbox struct {
	minLat, minLon, maxLat, maxLon float64
}

func streamBBox(stream []EnrichmentStreamPoint) bbox {
	b := bbox{minLat: 90, minLon: 180, maxLat: -90, maxLon: -180}
	for _, sp := range stream {
		if sp.Lat == nil || sp.Lng == nil {
			continue
		}
		b.minLat = min(b.minLat, *sp.Lat)
		b.maxLat = max(b.maxLat, *sp.Lat)
		b.minLon = min(b.minLon, *sp.Lng)
		b.maxLon = max(b.maxLon, *sp.Lng)
	}
	return b
}
