package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

// ApplyRulesToActivity evaluates rules against a single activity.
// triggerEvent filters which rules to evaluate: "on_import", "on_enrichment_complete",
// "on_manual_apply", or "" for all (used by "Regeln anwenden" button).
// System rules (except electric_sync) have retraction semantics:
// - Match + no removal → add tag + set_field
// - No match + tag present → remove tag + invert set_field (bool→false)
// User rules only add, never retract.
func ApplyRulesToActivity(ctx context.Context, db *database.DB, activityID, userID string, allRules []model.AutomationRule, triggerEvent string) error {
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
		// Filter by trigger event (empty = all)
		if triggerEvent != "" && rule.TriggerEvent != triggerEvent {
			continue
		}
		// apply_retroactively filter: on_import only evaluates new activities
		// unless the rule is retroactive or a system rule.
		// "created_at" = Upload-Zeitpunkt (nicht started_at, weil started_at bei Garmin-Sync in der Vergangenheit sein kann).
		// "" (manual apply) skips this check entirely.
		if triggerEvent == "on_import" && !rule.ApplyRetroactive && !rule.IsSystem {
			if actCreatedAt, ok := actMap["created_at"].(time.Time); ok {
				if actCreatedAt.Before(rule.CreatedAt) {
					continue
				}
			}
		}
		// Skip electric_sync / virtual_sync / indoor_sync — managed by DB trigger, not rules engine
		if rule.SystemType != nil && (*rule.SystemType == "electric_sync" || *rule.SystemType == "virtual_sync" || *rule.SystemType == "indoor_sync") {
			continue
		}

		var actions []rules.Action
		if err := json.Unmarshal(rule.Actions, &actions); err != nil {
			continue
		}

		// Evaluate conditions
		matched := EvaluateRule(rule, actMap)

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
			// Stop-processing: if this rule matched and has stop_on_match, skip remaining rules
			if rule.StopOnMatch {
				break
			}
		} else if rule.IsSystem {
			// Retraction semantics:
			// - System rules retract: if conditions no longer match, tags/set_field values are reset to defaults.
			//   This keeps system-managed fields (is_round_trip, is_indoor) in sync with current activity data.
			// - User rules do NOT retract: non-matching leaves the activity unchanged.
			//   This preserves manual user edits and avoids unexpected data loss.
			setFieldDefaults := map[string]any{
				"is_round_trip":        false,
				"is_indoor":            false,
				"exclude_from_stats":   false,
				"exclude_from_heatmap": false,
				"hide_map":             false,
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

// EvaluateRule checks whether a rule's conditions match the given activity map.
func EvaluateRule(rule model.AutomationRule, actMap map[string]any) bool {
	if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
		var groups []rules.ConditionGroup
		if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil && len(groups) > 0 {
			return rules.EvaluateGroups(groups, actMap)
		}
	} else if len(rule.Conditions) > 0 {
		var conditions []rules.Condition
		if err := json.Unmarshal(rule.Conditions, &conditions); err == nil {
			return rules.Evaluate(conditions, actMap)
		}
	}
	return false
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
