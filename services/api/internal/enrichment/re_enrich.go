package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const reEnrichWorkers = 3

// ReEnrichBatch re-runs selected enrichment modules for a list of activities.
// Each activity is processed independently: each module does its own DELETE +
// INSERT internally. On failure the module logs the error and continues.
//
// Three parallel workers process activities concurrently. For geo modules,
// geoEnrichSem (capacity=1) ensures only one activity runs heavy PostGIS
// operations at a time — other workers load streams in the meantime.
func ReEnrichBatch(ctx context.Context, db *database.DB, events *event.Publisher,
	userID string, modules, activityIDs []string, jobID string) {

	total := len(activityIDs)
	var processed, errors atomic.Int64

	// Load enrichment settings once
	var enrichSettings *model.EnrichmentSettings
	if s, err := db.GetEnrichmentSettings(ctx, userID); err == nil {
		enrichSettings = s
	}

	sem := make(chan struct{}, reEnrichWorkers)
	var wg sync.WaitGroup

	for _, actID := range activityIDs {
		select {
		case <-ctx.Done():
			wg.Wait()
			_ = db.CompleteBackgroundJob(ctx, jobID, true, "aborted: shutdown")
			return
		case sem <- struct{}{}:
		}

		wg.Go(func() {
			defer func() { <-sem }()

			if err := reEnrichActivity(ctx, db, userID, actID, modules, enrichSettings); err != nil {
				slog.Error("re-enrich: activity failed",
					"activity_id", actID, "error", err)
				errors.Add(1)
			} else {
				processed.Add(1)
			}

			// Progress update every 5 activities
			done := processed.Load() + errors.Load()
			if done%5 == 0 || done == int64(total) {
				_ = db.UpdateBackgroundJobProgress(ctx, jobID, done)
			}
		})
	}

	wg.Wait()

	// Finalize
	hasErrors := errors.Load() > 0
	errMsg := ""
	if hasErrors {
		errMsg = fmt.Sprintf("%d/%d activities had errors", errors.Load(), total)
	}
	_ = db.CompleteBackgroundJob(ctx, jobID, hasErrors, errMsg)
	_ = db.UpdateBackgroundJobProgress(ctx, jobID, int64(total))

	// Single analytics invalidation (Signatur: string, NICHT struct!)
	if events != nil {
		events.PublishAnalyticsInvalidate(userID)
	}

	slog.Info("re-enrich: batch complete",
		"user_id", userID,
		"total", total,
		"processed", processed.Load(),
		"errors", errors.Load(),
	)
}

// reEnrichActivity processes a single activity: runs the requested modules.
// Each module is self-contained — it deletes its own old data before inserting
// new data. No pre-delete step is needed here.
func reEnrichActivity(ctx context.Context, db *database.DB, userID, activityID string,
	modules []string, settings *model.EnrichmentSettings) error {

	// Determine which module types are requested
	hasStream := containsAny(modules, "ascents", "stops", "intervals")
	hasGeo := containsAny(modules, "peaks", "settlements", "forests", "water", "borders")

	// Load stream once if needed
	var stream []EnrichmentStreamPoint
	if hasStream {
		var err error
		stream, err = getEnrichmentStream(ctx, db, activityID)
		if err != nil {
			return fmt.Errorf("load stream: %w", err)
		}
	}

	// Build settings gates
	geoSettings := allGeoEnabled()
	streamSettings := allStreamEnabled()
	if settings != nil {
		geoSettings = geoSettingsFromModel(settings)
		streamSettings = streamSettingsFromModel(settings)
	}

	// KEIN separates deleteModuleData hier!
	// Jedes Modul macht intern DELETE + INSERT (stops sogar in einer TX).
	// Ein separates DELETE vor dem Modul-Aufruf wuerde bei Crash zwischen
	// DELETE und Modul-Start zu Datenverlust fuehren (alte Daten weg, neue
	// nicht da). Stattdessen: Module sind self-contained — sie loeschen
	// ihre eigenen alten Daten als ersten Schritt ihres Laufs.

	// Run stream modules
	if hasStream && len(stream) > 0 {
		for _, m := range modules {
			switch m {
			case "stops":
				if streamSettings.EnableStopDetection {
					if err := enrichStreamStops(ctx, db, userID, activityID, stream); err != nil {
						slog.Warn("re-enrich: stops failed", "activity_id", activityID, "error", err)
					}
				}
			case "ascents":
				if streamSettings.EnableAscentDetection {
					if err := enrichStreamAscents(ctx, db, userID, activityID, stream); err != nil {
						slog.Warn("re-enrich: ascents failed", "activity_id", activityID, "error", err)
					}
				}
			case "intervals":
				if streamSettings.EnableIntervalDetection {
					if err := enrichStreamIntervals(ctx, db, userID, activityID, stream); err != nil {
						slog.Warn("re-enrich: intervals failed", "activity_id", activityID, "error", err)
					}
				}
			}
		}
	}

	// Run geo modules via RunGeoEnrichment which manages geoEnrichSem internally
	if hasGeo {
		if err := RunGeoEnrichment(ctx, db, activityID, geoSettings); err != nil {
			slog.Warn("re-enrich: geo enrichment failed", "activity_id", activityID, "error", err)
		}
	}

	// Surfaces — separated from RunGeoEnrichment, has its own orchestration.
	if containsAny(modules, "surfaces") {
		enabled := settings == nil || settings.EnableSurfaceDetection
		if err := EnrichSurfaces(ctx, db, activityID, enabled, nil, nil); err != nil {
			slog.Warn("re-enrich: surfaces failed", "activity_id", activityID, "error", err)
		}
	}

	return nil
}

// containsAny reports whether slice contains any of the given values.
func containsAny(slice []string, values ...string) bool {
	for _, v := range values {
		for _, s := range slice {
			if s == v {
				return true
			}
		}
	}
	return false
}
