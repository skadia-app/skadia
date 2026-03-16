package enrichment

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// StreamEnrichmentSettings contains the per-user flags that gate individual
// stream-enrichment modules. Pass allStreamEnabled() to run every module
// regardless of user preferences.
type StreamEnrichmentSettings struct {
	EnableStops     bool
	EnableAscents   bool
	EnableIntervals bool
}

// streamSettingsFromModel converts a model.EnrichmentSettings into the smaller
// StreamEnrichmentSettings struct used by RunStreamEnrichment.
func streamSettingsFromModel(s *model.EnrichmentSettings) StreamEnrichmentSettings {
	return StreamEnrichmentSettings{
		EnableStops:     s.EnableStops,
		EnableAscents:   s.EnableAscents,
		EnableIntervals: s.EnableIntervals,
	}
}

// allStreamEnabled returns a StreamEnrichmentSettings with every flag set to
// true, used as a fallback when no user settings are available.
func allStreamEnabled() StreamEnrichmentSettings {
	return StreamEnrichmentSettings{
		EnableStops:     true,
		EnableAscents:   true,
		EnableIntervals: true,
	}
}

// RunStreamEnrichment executes stream-based enrichment modules for an activity.
// Analog to RunGeoEnrichment: modules are run in sequence; failures are logged
// per module and do not abort subsequent modules.
//
// Modules:
//   - stops     → enrichStreamStops     (stop detection + POI matching)
//   - ascents   → enrichStreamAscents   (climb detection)
//   - intervals → enrichStreamIntervals (interval detection)
func RunStreamEnrichment(ctx context.Context, db *database.DB, userID, activityID string, settings StreamEnrichmentSettings) error {
	slog.Info("running stream enrichment (Go SQL)", "activity_id", activityID)

	type module struct {
		name    string
		enabled bool
		fn      func(context.Context, *database.DB, string) error
	}
	modules := []module{
		{"stops", settings.EnableStops, enrichStreamStops},
		{"ascents", settings.EnableAscents, enrichStreamAscents},
		{"intervals", settings.EnableIntervals, enrichStreamIntervals},
	}

	for _, m := range modules {
		if !m.enabled {
			slog.Debug("stream enrichment module disabled, skipping",
				"module", m.name,
				"activity_id", activityID,
			)
			continue
		}
		if err := m.fn(ctx, db, activityID); err != nil {
			slog.Warn("stream enrichment module failed",
				"module", m.name,
				"activity_id", activityID,
				"error", err,
			)
		}
	}

	return nil
}

