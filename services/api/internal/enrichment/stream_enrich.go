package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// EnrichmentStreamPoint is a single row from activity_streams loaded once and
// shared across all stream-enrichment modules to avoid repeated decompression
// of the TimescaleDB hypertable.
type EnrichmentStreamPoint struct {
	Time      time.Time
	Lat       *float64
	Lng       *float64
	AltitudeM *float64
	SpeedMs   *float64
	DistanceM *float64
	HeartRate *int
	PowerW    *float64
	Cadence   *float64
}

// getEnrichmentStream loads all stream fields in a single query with no NULL
// filter — each module filters the in-memory slice itself.
func getEnrichmentStream(ctx context.Context, db *database.DB, activityID string) ([]EnrichmentStreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
		       heart_rate, power_w, cadence
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("getEnrichmentStream: query: %w", err)
	}
	defer rows.Close()

	var pts []EnrichmentStreamPoint
	for rows.Next() {
		var p EnrichmentStreamPoint
		if err := rows.Scan(
			&p.Time, &p.Lat, &p.Lng, &p.AltitudeM, &p.SpeedMs, &p.DistanceM,
			&p.HeartRate, &p.PowerW, &p.Cadence,
		); err != nil {
			return nil, fmt.Errorf("getEnrichmentStream: scan: %w", err)
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getEnrichmentStream: rows: %w", err)
	}
	return pts, nil
}

// GetEnrichmentStream loads stream rows for callers outside this package.
func GetEnrichmentStream(ctx context.Context, db *database.DB, activityID string) ([]EnrichmentStreamPoint, error) {
	return getEnrichmentStream(ctx, db, activityID)
}

// StreamEnrichmentSettings contains the per-user flags that gate individual
// stream-enrichment modules. Pass allStreamEnabled() to run every module
// regardless of user preferences.
type StreamEnrichmentSettings struct {
	EnableLiftDetection     bool
	EnableStopDetection     bool
	EnableAscentDetection   bool
	EnableIntervalDetection bool

	// SkipGearRefresh suppresses the per-activity gear-stats refresh inside
	// enrichStreamLifts / RecomputeLiftStats. Set this in bulk-recompute
	// paths (e.g. the backfill CLI) where a single UpdateGearStats call per
	// affected gear replaces N redundant per-activity refreshes.
	SkipGearRefresh bool
}

// StreamSettingsFromModel converts a model.EnrichmentSettings into the smaller
// StreamEnrichmentSettings struct used by RunStreamEnrichment.
func StreamSettingsFromModel(s *model.EnrichmentSettings) StreamEnrichmentSettings {
	return StreamEnrichmentSettings{
		EnableLiftDetection:     s.EnableLiftDetection,
		EnableStopDetection:     s.EnableStopDetection,
		EnableAscentDetection:   s.EnableAscentDetection,
		EnableIntervalDetection: s.EnableIntervalDetection,
	}
}

// streamSettingsFromModel is the package-internal alias kept for stage1.go.
func streamSettingsFromModel(s *model.EnrichmentSettings) StreamEnrichmentSettings {
	return StreamSettingsFromModel(s)
}

// AllStreamEnabled returns a StreamEnrichmentSettings with every flag set to
// true, used as a fallback when no user settings are available.
func AllStreamEnabled() StreamEnrichmentSettings {
	return StreamEnrichmentSettings{
		EnableLiftDetection:     true,
		EnableStopDetection:     true,
		EnableAscentDetection:   true,
		EnableIntervalDetection: true,
	}
}

// allStreamEnabled is the package-internal alias kept for stage1.go.
func allStreamEnabled() StreamEnrichmentSettings {
	return AllStreamEnabled()
}

// RunStreamEnrichment executes stream-based enrichment modules for an activity.
// Analog to RunGeoEnrichment: modules are run in sequence; failures are logged
// per module and do not abort subsequent modules.
//
// The activity stream (a compressed TimescaleDB hypertable) is loaded ONCE and
// shared across all modules to avoid repeated decompression overhead.
//
// Modules:
//   - lifts     → enrichStreamLifts     (Method-A OSM-snap + Method-B heuristic)
//   - stops     → enrichStreamStops     (stop detection + POI matching)
//   - ascents   → enrichStreamAscents   (climb detection)
//   - intervals → enrichStreamIntervals (interval detection)
//
// Lift detection runs first because its output (lift time-windows) can serve
// as hint context for downstream stop/ascent modules — a cable-car ride is
// neither a "stop" nor an "ascent" even though it superficially resembles
// both. Cross-module suppression is out of scope for Phase 4 (the slot is
// reserved here so module ordering becomes a deliberate choice rather than
// list-append happenstance).
func RunStreamEnrichment(ctx context.Context, db *database.DB, userID, activityID string, settings StreamEnrichmentSettings) error {
	slog.Info("running stream enrichment (Go SQL)", "activity_id", activityID)

	// Load activity_streams ONCE — decompression happens here, not per module.
	stream, err := getEnrichmentStream(ctx, db, activityID)
	if err != nil {
		slog.Warn("stream enrichment: failed to load stream, skipping all modules",
			"activity_id", activityID,
			"error", err,
		)
		return err
	}
	if len(stream) == 0 {
		slog.Debug("stream enrichment: empty stream, nothing to do", "activity_id", activityID)
		return nil
	}

	// Skip stream enrichment for water-category sports (diving, swimming, etc.).
	// Ascent detection, stop detection, and interval detection all rely on
	// altitude/speed data that is meaningless or zero underwater.
	//
	// Note: db.GetActivity returns (nil, nil) when the row is not found
	// (cf. pgx.ErrNoRows branch). Guard against nil here — the post-commit
	// fusion-cleanup goroutine outlives the request and can race with test
	// teardown or source-activity deletion.
	if act, err := db.GetActivity(ctx, activityID); err == nil && act != nil {
		if def, ok := sport.GetSportDef(act.SportType); ok && def.CategoryKey == "water" {
			slog.Debug("stream enrichment: skipping for water sport",
				"activity_id", activityID,
				"sport_type", act.SportType,
			)
			return nil
		}
	}

	// Capture SkipGearRefresh in a closure so the lifts module can forward it
	// to enrichStreamLiftsOpts without changing the shared module fn signature.
	skipGear := settings.SkipGearRefresh
	liftFn := func(ctx context.Context, db *database.DB, userID, activityID string, stream []EnrichmentStreamPoint) error {
		return enrichStreamLiftsOpts(ctx, db, userID, activityID, stream, skipGear)
	}

	type module struct {
		name    string
		enabled bool
		fn      func(context.Context, *database.DB, string, string, []EnrichmentStreamPoint) error
	}
	modules := []module{
		{"lifts", settings.EnableLiftDetection, liftFn},
		{"stops", settings.EnableStopDetection, enrichStreamStops},
		{"ascents", settings.EnableAscentDetection, enrichStreamAscents},
		{"intervals", settings.EnableIntervalDetection, enrichStreamIntervals},
	}

	for _, m := range modules {
		if !m.enabled {
			slog.Debug("stream enrichment module disabled, skipping",
				"module", m.name,
				"activity_id", activityID,
			)
			continue
		}
		if err := m.fn(ctx, db, userID, activityID, stream); err != nil {
			slog.Warn("stream enrichment module failed",
				"module", m.name,
				"activity_id", activityID,
				"error", err,
			)
		}
	}

	return nil
}
