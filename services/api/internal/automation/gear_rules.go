package automation

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// EvaluateGearLifecycleRule evaluates a gear-lifecycle rule.
// Returns gear IDs that have exceeded service thresholds.
//
// Gear lifecycle rules trigger based on accumulated usage metrics
// (distance, time, activity count) since last service.
//
// Note: gear_service_intervals (existing feature) provides simpler
// threshold-based service reminders. Gear lifecycle rules extend
// this with automation actions (e.g., auto-tag activities, send notifications).
//
// TODO: Full implementation TBD. Coexists with gear_service_intervals.
func EvaluateGearLifecycleRule(ctx context.Context, db *database.DB, userID string, rule model.AutomationRule) ([]string, error) {
	// Placeholder — full implementation TBD
	// Required design decisions:
	// 1. Condition schema: {"gear_id": "...", "metric": "distance_m", "op": "gte", "threshold": "5000000"}
	// 2. How to accumulate distance since last service reset
	// 3. Actions: add_tag to activities using that gear, send notification
	return nil, nil
}
