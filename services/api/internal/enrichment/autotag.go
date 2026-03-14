package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// Condition represents a single autotag rule condition.
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq, neq, gt, lt, gte, lte, between, contains, in
	Value any    `json:"value"`
}

// AutoTagConditions represents the conditions for an autotag.
type AutoTagConditions struct {
	Operator string      `json:"operator"` // AND, OR (default: AND)
	Rules    []Condition `json:"rules"`
}

// EnrichmentContext provides enrichment fields computed by stage1 that autotag
// conditions may reference (start_city, time_of_day, etc.).
type EnrichmentContext struct {
	StartCity    string
	StartCountry string
	TimeOfDay    []string
	IsRoundTrip  bool
}

// EvaluateAutoTags loads all autotag definitions from DB and evaluates them
// against activity data. Returns a list of tag names that should be applied.
func EvaluateAutoTags(ctx context.Context, db *database.DB, userID string, data *model.ActivityData, enrichCtx *EnrichmentContext) []string {
	autoTags, err := db.GetAutoTags(ctx, userID)
	if err != nil {
		slog.Error("autotag: failed to load auto tags", "user_id", userID, "error", err)
		return nil
	}

	var matched []string
	for _, tag := range autoTags {
		if tag.Conditions == nil {
			continue
		}
		var conds AutoTagConditions
		if err := json.Unmarshal(tag.Conditions, &conds); err != nil {
			slog.Error("autotag: invalid conditions JSON", "tag", tag.Name, "error", err)
			continue
		}
		if evaluateConditions(conds, data, enrichCtx) {
			matched = append(matched, tag.Name)
		}
	}
	return matched
}

// evaluateConditions evaluates the complete condition set (AND/OR).
func evaluateConditions(conds AutoTagConditions, data *model.ActivityData, enrichCtx *EnrichmentContext) bool {
	if len(conds.Rules) == 0 {
		return false
	}

	op := strings.ToUpper(conds.Operator)
	if op == "" {
		op = "AND"
	}

	for _, rule := range conds.Rules {
		result := evaluateCondition(rule, data, enrichCtx)
		if op == "OR" && result {
			return true
		}
		if op == "AND" && !result {
			return false
		}
	}

	// AND: all passed; OR: none passed
	return op == "AND"
}

// evaluateCondition evaluates a single condition against activity data.
func evaluateCondition(cond Condition, data *model.ActivityData, enrichCtx *EnrichmentContext) bool {
	switch cond.Field {
	case "temperature_avg_c":
		return compareFloat(data.AvgTemperatureC, cond.Op, cond.Value)
	case "temperature_min_c":
		minTemp := minStreamTemperature(data.Streams, data.AvgTemperatureC)
		return compareFloat(minTemp, cond.Op, cond.Value)
	case "time_of_day":
		if enrichCtx == nil {
			return false
		}
		return compareStringSlice(enrichCtx.TimeOfDay, cond.Op, cond.Value)
	case "sport_type":
		return compareString(data.SportType, cond.Op, cond.Value)
	case "distance_m":
		return compareFloat(data.DistanceM, cond.Op, cond.Value)
	case "elevation_gain_m":
		return compareFloat(data.ElevationGainM, cond.Op, cond.Value)
	case "avg_speed_ms":
		return compareFloat(data.AvgSpeedMS, cond.Op, cond.Value)
	case "avg_hr":
		return compareFloat(data.AvgHeartRate, cond.Op, cond.Value)
	case "max_hr":
		return compareFloat(float64(data.MaxHeartRate), cond.Op, cond.Value)
	case "elapsed_time_s":
		return compareFloat(float64(data.ElapsedTimeS), cond.Op, cond.Value)
	case "is_round_trip":
		if enrichCtx == nil {
			return false
		}
		return compareBool(enrichCtx.IsRoundTrip, cond.Op, cond.Value)
	case "is_indoor":
		return compareBool(data.IsIndoor, cond.Op, cond.Value)
	case "start_city":
		if enrichCtx == nil {
			return false
		}
		return compareString(enrichCtx.StartCity, cond.Op, cond.Value)
	case "start_country":
		if enrichCtx == nil {
			return false
		}
		return compareString(enrichCtx.StartCountry, cond.Op, cond.Value)
	case "day_of_week":
		dow := float64(data.StartedAt.Weekday())
		return compareFloat(dow, cond.Op, cond.Value)
	case "month":
		month := float64(data.StartedAt.Month())
		return compareFloat(month, cond.Op, cond.Value)
	default:
		slog.Warn("autotag: unknown condition field", "field", cond.Field)
		return false
	}
}

// minStreamTemperature returns the minimum temperature from streams,
// falling back to avgTempC if no stream data exists.
func minStreamTemperature(streams []model.StreamPoint, avgTempC float64) float64 {
	minT := avgTempC
	found := false
	for i := range streams {
		if streams[i].TemperatureC != nil {
			t := float64(*streams[i].TemperatureC)
			if !found || t < minT {
				minT = t
				found = true
			}
		}
	}
	return minT
}

// compareFloat compares a float64 value against a condition.
func compareFloat(actual float64, op string, value any) bool {
	switch op {
	case "eq":
		v, ok := toFloat64(value)
		return ok && actual == v
	case "neq":
		v, ok := toFloat64(value)
		return ok && actual != v
	case "gt":
		v, ok := toFloat64(value)
		return ok && actual > v
	case "lt":
		v, ok := toFloat64(value)
		return ok && actual < v
	case "gte":
		v, ok := toFloat64(value)
		return ok && actual >= v
	case "lte":
		v, ok := toFloat64(value)
		return ok && actual <= v
	case "between":
		bounds := toFloat64Slice(value)
		if len(bounds) != 2 {
			return false
		}
		return actual >= bounds[0] && actual <= bounds[1]
	case "in":
		vals := toFloat64Slice(value)
		return slices.Contains(vals, actual)
	default:
		return false
	}
}

// compareString compares a string value against a condition.
func compareString(actual string, op string, value any) bool {
	switch op {
	case "eq":
		v := fmt.Sprintf("%v", value)
		return strings.EqualFold(actual, v)
	case "neq":
		v := fmt.Sprintf("%v", value)
		return !strings.EqualFold(actual, v)
	case "contains":
		v := fmt.Sprintf("%v", value)
		return strings.Contains(strings.ToLower(actual), strings.ToLower(v))
	case "in":
		vals := toStringSlice(value)
		for _, v := range vals {
			if strings.EqualFold(actual, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// compareStringSlice compares a string slice (e.g. time_of_day) against a condition.
func compareStringSlice(actual []string, op string, value any) bool {
	switch op {
	case "contains":
		v := fmt.Sprintf("%v", value)
		for _, a := range actual {
			if strings.EqualFold(a, v) {
				return true
			}
		}
		return false
	case "eq":
		v := fmt.Sprintf("%v", value)
		for _, a := range actual {
			if strings.EqualFold(a, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// compareBool compares a bool value against a condition.
func compareBool(actual bool, op string, value any) bool {
	v, ok := toBool(value)
	if !ok {
		return false
	}
	switch op {
	case "eq":
		return actual == v
	case "neq":
		return actual != v
	default:
		return false
	}
}

// toFloat64 converts an interface{} to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// toFloat64Slice converts an interface{} to []float64 (for "between" and "in" ops).
func toFloat64Slice(v any) []float64 {
	switch arr := v.(type) {
	case []any:
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			f, ok := toFloat64(item)
			if ok {
				result = append(result, f)
			}
		}
		return result
	default:
		return nil
	}
}

// toStringSlice converts an interface{} to []string (for "in" op).
func toStringSlice(v any) []string {
	switch arr := v.(type) {
	case []any:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	default:
		return nil
	}
}

// toBool converts an interface{} to bool.
func toBool(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	default:
		return false, false
	}
}
