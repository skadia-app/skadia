package automation

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// EvaluateAggregateRule evaluates an aggregate rule against a user's activity history.
// Returns the list of activity IDs that should be tagged.
//
// Aggregate rules operate on sets of activities, not individual ones.
// They support period-based conditions (e.g., "in the last 30 days")
// and aggregation functions (COUNT, SUM, AVG) over activity fields.
//
// TODO: Full implementation requires a separate condition schema spec
// for period/aggregation parameters. This is a design spike.
func EvaluateAggregateRule(ctx context.Context, db *database.DB, userID string, rule model.AutomationRule) ([]string, error) {
	// Placeholder — full implementation TBD
	// Required design decisions:
	// 1. Condition schema: {"field": "distance_m", "op": "sum_gte", "value": "1000000", "period": "30d"}
	// 2. SQL aggregation query pattern
	// 3. Result: activity IDs that crossed a threshold in the given period
	return nil, nil
}
