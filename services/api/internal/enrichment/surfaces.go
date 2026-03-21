package enrichment

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// EnrichSurfaces matches the activity track against OSM ways and writes
// surface-type + highway-type statistics to activity_surfaces.
//
// Two paths:
//   - SQL (always available): Nearest-Way per resampled point, ~100ms
//   - Graph (when tiles available): Viterbi on Tiled Graph, ~10ms (Phase 3+4)
//
// Each resampled point gets exactly one classification:
//   - surface tag present → concrete type (asphalt, gravel, etc.)
//   - way matched but no surface tag → "unknown"
//   - no way within 25m → "offroad"
//
// Percentages always sum to 100%.
func EnrichSurfaces(ctx context.Context, db *database.DB, activityID string, enabled bool) error {
	if !enabled {
		return nil
	}

	// Phase 3+4: Graph path will be added here later (graphReader + stream parameters).
	// For now, SQL-only.

	return enrichSurfacesSQL(ctx, db, activityID)
}
