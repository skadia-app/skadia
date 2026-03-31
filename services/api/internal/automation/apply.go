package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

// shouldEvaluateRule checks if a rule should be evaluated for the given trigger event.
func shouldEvaluateRule(rule model.AutomationRule, triggerEvent string) bool {
	if !rule.IsActive {
		return false
	}
	if triggerEvent != "" && rule.TriggerEvent != triggerEvent {
		return false
	}
	if rule.SystemType != nil {
		switch *rule.SystemType {
		case "electric_sync", "virtual_sync", "indoor_sync", "commute_sync":
			return false
		}
	}
	return true
}

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
	ruleMatchCounts := map[string]int{}
	ruleEvaluatedIDs := map[string]bool{}
	for _, rule := range allRules {
		if !shouldEvaluateRule(rule, triggerEvent) {
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

		var actions []rules.Action
		if err := json.Unmarshal(rule.Actions, &actions); err != nil {
			continue
		}

		ruleEvaluatedIDs[rule.ID] = true

		// Evaluate conditions
		matched := EvaluateRule(rule, actMap)

		changes := rules.Apply(actions)

		if matched {
			// Check for branches (ELSE/ELSE-IF) — overrides default actions if present.
			// Branches are evaluated sequentially; first matching branch wins.
			// Old rules without branches fall through to the default actions block.
			if len(rule.Branches) > 0 && string(rule.Branches) != "null" {
				type branch struct {
					Type            string                 `json:"type"` // "if", "else_if", "else"
					ConditionGroups []rules.ConditionGroup `json:"condition_groups"`
					Actions         json.RawMessage        `json:"actions"`
				}
				var branches []branch
				if err := json.Unmarshal(rule.Branches, &branches); err == nil && len(branches) > 0 {
					for _, br := range branches {
						branchMatched := false
						if br.Type == "else" {
							branchMatched = true
						} else if len(br.ConditionGroups) > 0 {
							branchMatched = rules.EvaluateGroups(br.ConditionGroups, actMap)
						}
						if branchMatched {
							var branchActions []rules.Action
							if err2 := json.Unmarshal(br.Actions, &branchActions); err2 == nil {
								for _, change := range rules.Apply(branchActions) {
									val := change.Value
									if change.Field == "set_name_template" {
										if tmpl, ok := val.(string); ok {
											val = resolveNameTemplate(tmpl, actMap)
										}
									}
									if change.Field == "notify" {
										if tmpl, ok := val.(string); ok && tmpl != "" {
											body := resolveNameTemplate(tmpl, actMap)
											if err3 := db.CreateNotification(ctx, userID, "rule", rule.Name, body); err3 != nil {
												slog.Warn("rule notify (branch)", "activity_id", activityID, "rule_id", rule.ID, "error", err3)
											}
										}
										continue
									}
									if err3 := db.ApplyRuleChange(ctx, activityID, change.Field, val); err3 != nil {
										slog.Warn("apply branch change", "activity_id", activityID, "rule_id", rule.ID, "field", change.Field, "error", err3)
									}
								}
							}
							break // first matching branch wins
						}
					}
					if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
						slog.Warn("increment rule applied", "rule_id", rule.ID, "error", err)
					}
					ruleMatchCounts[rule.ID]++
					if rule.StopOnMatch {
						break
					}
					continue
				}
			}

			for _, change := range changes {
				val := change.Value
				if change.Field == "set_name_template" {
					if tmpl, ok := val.(string); ok {
						val = resolveNameTemplate(tmpl, actMap)
					}
				}
				if change.Field == "notify" {
					if tmpl, ok := val.(string); ok && tmpl != "" {
						body := resolveNameTemplate(tmpl, actMap)
						if err := db.CreateNotification(ctx, userID, "rule", rule.Name, body); err != nil {
							slog.Warn("rule notify", "activity_id", activityID, "rule_id", rule.ID, "error", err)
						}
					}
					continue
				}
				if err := db.ApplyRuleChange(ctx, activityID, change.Field, val); err != nil {
					slog.Warn("apply rule change", "activity_id", activityID, "rule_id", rule.ID, "field", change.Field, "error", err)
				}
			}
			if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
				slog.Warn("increment rule applied", "rule_id", rule.ID, "error", err)
			}
			ruleMatchCounts[rule.ID]++
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

	// Update last_evaluated_at and last_matched_count for each matched rule.
	now := time.Now()
	for ruleID, matchCount := range ruleMatchCounts {
		db.Pool.Exec(ctx,
			`UPDATE automation_rules SET last_evaluated_at = $1, last_matched_count = $2 WHERE id = $3`,
			now, matchCount, ruleID)
	}
	// Update last_evaluated_at for evaluated-but-unmatched rules.
	for ruleID := range ruleEvaluatedIDs {
		if _, alreadyUpdated := ruleMatchCounts[ruleID]; alreadyUpdated {
			continue
		}
		db.Pool.Exec(ctx,
			`UPDATE automation_rules SET last_evaluated_at = $1 WHERE id = $2`,
			now, ruleID)
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

// toFloat converts common numeric types to float64. Returns 0 for unsupported types.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

var reToken = regexp.MustCompile(`\{([^}]+)\}`)

// resolveNameTemplate substitutes {token} and {token|fallback} placeholders in
// tmpl with values from the activity map. Supported tokens:
//
//   - Legacy aliases: {city} → start_city, {sport} → sport_type
//   - Computed:       {distance_km}, {weekday}, {elevation_gain}, {duration}
//   - Direct fields:  {start_city}, {start_state}, {start_country}, …
//
// If a field is missing or nil and no fallback is provided, the token is
// replaced with an empty string.
func resolveNameTemplate(tmpl string, actMap map[string]any) string {
	// Token aliases for backwards compatibility
	aliases := map[string]string{
		"city":  "start_city",
		"sport": "sport_type",
	}

	return reToken.ReplaceAllStringFunc(tmpl, func(match string) string {
		inner := match[1 : len(match)-1] // strip { and }
		field, fallback, hasFallback := strings.Cut(inner, "|")
		field = strings.TrimSpace(field)

		// Apply alias
		if alias, ok := aliases[field]; ok {
			field = alias
		}

		// Special computed tokens
		switch field {
		case "distance_km":
			if d, ok := actMap["distance_m"]; ok && d != nil {
				return fmt.Sprintf("%d km", int(math.Round(toFloat(d)/1000)))
			}
		case "weekday":
			if t, ok := actMap["started_at"].(time.Time); ok {
				days := []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
				return days[t.Weekday()]
			}
		case "elevation_gain":
			if v, ok := actMap["elevation_gain_m"]; ok && v != nil {
				return fmt.Sprintf("%.0f", toFloat(v))
			}
		case "duration":
			if v, ok := actMap["moving_time_s"]; ok && v != nil {
				s := int(toFloat(v))
				if s >= 3600 {
					return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
				}
				return fmt.Sprintf("%dm", s/60)
			}
		case "country":
			field = "start_country"
		}

		// Direct field lookup
		if val, ok := actMap[field]; ok && val != nil {
			return fmt.Sprintf("%v", val)
		}

		// Fallback or empty string
		if hasFallback {
			return fallback
		}
		return ""
	})
}
