package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

// ApplyRulesToActivity evaluates all given rules against a single activity.
// System rules (except electric_sync) have retraction semantics:
// - Match + no removal → add tag + set_field
// - No match + tag present → remove tag + invert set_field (bool→false)
// User rules only add, never retract.
func ApplyRulesToActivity(ctx context.Context, db *database.DB, activityID, userID string, allRules []model.AutomationRule) error {
	actMap, err := db.GetActivityAsMap(ctx, activityID)
	if err != nil || actMap == nil {
		return err
	}

	// NOTE: No transaction wrapping individual ApplyRuleChange calls.
	// ApplyRuleChange and IncrementRuleApplied use db.Pool directly,
	// and refactoring them to accept pgx.Tx would touch 10+ call sites.
	// Inconsistency risk is low: each ApplyRuleChange is idempotent
	// (ON CONFLICT DO NOTHING for add_tag, simple UPDATE for fields).
	// A future refactor could introduce a querier interface.
	for _, rule := range allRules {
		if !rule.IsActive {
			continue
		}
		// Skip electric_sync / virtual_sync — managed by DB trigger, not rules engine
		if rule.SystemType != nil && (*rule.SystemType == "electric_sync" || *rule.SystemType == "virtual_sync") {
			continue
		}

		var actions []rules.Action
		if err := json.Unmarshal(rule.Actions, &actions); err != nil {
			continue
		}

		// Evaluate conditions
		matched := false
		if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
			var groups []rules.ConditionGroup
			if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil && len(groups) > 0 {
				matched = rules.EvaluateGroups(groups, actMap)
			}
		} else if len(rule.Conditions) > 0 {
			var conditions []rules.Condition
			if err := json.Unmarshal(rule.Conditions, &conditions); err == nil {
				matched = rules.Evaluate(conditions, actMap)
			}
		}

		changes := rules.Apply(actions)

		if matched {
			for _, change := range changes {
				val := change.Value
				if change.Field == "set_name_template" {
					if tmpl, ok := val.(string); ok {
						val = resolveNameTemplate(tmpl, actMap)
					}
				}
				if err := db.ApplyRuleChange(ctx, activityID, change.Field, val); err != nil {
					slog.Warn("apply rule change", "activity_id", activityID, "rule_id", rule.ID, "field", change.Field, "error", err)
				}
			}
			if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
				slog.Warn("increment rule applied", "rule_id", rule.ID, "error", err)
			}
		} else if rule.IsSystem {
			// Retraction: remove tags + reset set_field for system rules
			setFieldDefaults := map[string]any{
				"is_round_trip": false,
				"is_indoor":     false,
			}
			for _, change := range changes {
				switch change.Field {
				case "add_tag":
					tagID, _ := change.Value.(string)
					if tagID != "" {
						db.ApplyRuleChange(ctx, activityID, "remove_tag", tagID)
					}
				default:
					if defVal, ok := setFieldDefaults[change.Field]; ok {
						db.ApplyRuleChange(ctx, activityID, change.Field, defVal)
					}
				}
			}
		}
		// User rules: no match → do nothing (no retraction)
	}
	return nil
}

// resolveNameTemplate substitutes a fixed set of {token} placeholders in tmpl
// with values from the activity map. The substitution is a simple sequential
// strings.ReplaceAll pass — tokens cannot be nested or escaped.
func resolveNameTemplate(tmpl string, act map[string]any) string {
	result := tmpl
	if city, ok := act["start_city"].(string); ok {
		result = strings.ReplaceAll(result, "{city}", city)
	}
	if dist, ok := act["distance_m"].(float64); ok {
		result = strings.ReplaceAll(result, "{distance_km}", fmt.Sprintf("%.0f km", dist/1000))
	}
	if sport, ok := act["sport_type"].(string); ok {
		result = strings.ReplaceAll(result, "{sport}", sport)
	}
	return result
}
