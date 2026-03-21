package enrichment

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// GraphGetter returns the graph tile for a coordinate, or nil if unavailable.
type GraphGetter func(lat, lng float64) *graph.Graph

// EnrichSurfaces matches the activity track against OSM ways and writes
// surface-type + highway-type statistics to activity_surfaces.
//
// Two paths:
//   - Graph (when graphGetter + stream provided): Viterbi on Tiled Graph, ~10ms
//   - SQL (fallback): Nearest-Way per resampled point, ~100ms
//
// Each resampled point gets exactly one classification:
//   - surface tag present → concrete type (asphalt, gravel, etc.)
//   - way matched but no surface tag → "unknown"
//   - no way within 25m → "offroad"
//
// Percentages always sum to 100%.
func EnrichSurfaces(ctx context.Context, db *database.DB, activityID string, enabled bool,
	graphGet GraphGetter, stream []EnrichmentStreamPoint) error {

	if !enabled {
		return nil
	}

	// Graph path: preferred when tiles cover the track
	if graphGet != nil && len(stream) > 0 {
		if err := enrichSurfacesGraph(ctx, db, graphGet, activityID, stream); err != nil {
			slog.Warn("surfaces: graph path failed, falling back to SQL",
				"activity_id", activityID, "error", err)
		} else {
			return nil
		}
	}

	// SQL path (always available)
	return enrichSurfacesSQL(ctx, db, activityID)
}
