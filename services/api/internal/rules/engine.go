package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Condition represents a single matching criterion for an automation rule.
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// Action represents an operation to perform when a rule matches.
type Action struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// ActivityChange describes a single field mutation resulting from an action.
type ActivityChange struct {
	Field string
	Value any
}

// Evaluate checks if an activity matches all conditions.
// Supported fields: sport_type, distance_m, elapsed_time_s, is_indoor, has_gps, intensity, time_of_day
// Supported ops: eq, neq, gt, lt, gte, lte, contains, in
func Evaluate(conditions []Condition, activity map[string]any) bool {
	for _, c := range conditions {
		if !evalCondition(c, activity) {
			return false
		}
	}
	return true
}

func evalCondition(c Condition, activity map[string]any) bool {
	val, ok := activity[c.Field]
	if !ok {
		return false
	}

	switch c.Op {
	case "eq":
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", c.Value)
	case "neq":
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", c.Value)
	case "gt":
		return toFloat(val) > toFloat(c.Value)
	case "lt":
		return toFloat(val) < toFloat(c.Value)
	case "gte":
		return toFloat(val) >= toFloat(c.Value)
	case "lte":
		return toFloat(val) <= toFloat(c.Value)
	case "contains":
		return strings.Contains(
			strings.ToLower(fmt.Sprintf("%v", val)),
			strings.ToLower(fmt.Sprintf("%v", c.Value)),
		)
	case "in":
		return evalIn(val, c.Value)
	default:
		return false
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func evalIn(val, list any) bool {
	s := fmt.Sprintf("%v", val)
	switch items := list.(type) {
	case []any:
		for _, item := range items {
			if fmt.Sprintf("%v", item) == s {
				return true
			}
		}
	case []string:
		for _, item := range items {
			if item == s {
				return true
			}
		}
	}
	return false
}

// Apply executes actions and returns the resulting changes.
// Supported actions: set_gear, add_tag, set_visibility
func Apply(actions []Action) []ActivityChange {
	var changes []ActivityChange
	for _, a := range actions {
		switch a.Action {
		case "set_gear":
			var params struct {
				GearID string `json:"gear_id"`
			}
			if json.Unmarshal(a.Params, &params) == nil && params.GearID != "" {
				changes = append(changes, ActivityChange{Field: "gear_id", Value: params.GearID})
			}
		case "add_tag":
			var params struct {
				Tag string `json:"tag"`
			}
			if json.Unmarshal(a.Params, &params) == nil && params.Tag != "" {
				changes = append(changes, ActivityChange{Field: "add_tag", Value: params.Tag})
			}
		case "set_visibility":
			var params struct {
				Visibility string `json:"visibility"`
			}
			if json.Unmarshal(a.Params, &params) == nil && params.Visibility != "" {
				changes = append(changes, ActivityChange{Field: "visibility", Value: params.Visibility})
			}
		}
	}
	return changes
}
