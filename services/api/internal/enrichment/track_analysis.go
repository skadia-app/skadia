package enrichment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// EnrichTrackAnalysis writes the unified per-track OSM-derived analysis.
//
// It replaces the old surfaces-only worker for callers that can supply user
// settings. Nil settings intentionally mean "disabled/no-op"; callers that
// need the historical default must pass explicit settings.
func EnrichTrackAnalysis(
	ctx context.Context,
	db *database.DB,
	activityID string,
	settings *model.EnrichmentSettings,
	graphReader routing.GraphReader,
	stream []EnrichmentStreamPoint,
) error {
	if settings == nil || (!settings.EnableSurfaceDetection && !settings.EnableDifficultyScales) {
		return nil
	}
	if db == nil {
		return fmt.Errorf("track_analysis: db is nil for active enrichment")
	}

	if _, err := db.Pool.Exec(ctx, "DELETE FROM activity_enrichments WHERE activity_id = $1", activityID); err != nil {
		return fmt.Errorf("track_analysis: delete existing enrichments: %w", err)
	}

	if graphReader != nil && len(stream) > 0 {
		bbox := streamBBox(stream)
		if graphReader.HasTilesFor(bbox.minLat, bbox.minLon, bbox.maxLat, bbox.maxLon) {
			if err := enrichTrackAnalysisGraph(ctx, db, graphReader.GraphAt, activityID, stream, settings); err != nil {
				slog.Warn("track_analysis: graph path failed, falling back to SQL",
					"activity_id", activityID, "error", err)
			} else {
				return nil
			}
		}
	}

	return enrichTrackAnalysisSQL(ctx, db, activityID, settings)
}

func defaultSurfaceTrackAnalysisSettings() *model.EnrichmentSettings {
	return &model.EnrichmentSettings{
		EnableSurfaceDetection: true,
		EnableDifficultyScales: false,
	}
}
