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

// sendWebhookChange fires a webhook from an ActivityChange with field=="webhook".
// It extracts url/allowlist from the value map and delegates to SendWebhook.
// Errors are logged as warnings and do not propagate (best-effort delivery).
func sendWebhookChange(activityID string, rule model.AutomationRule, change rules.ActivityChange) {
	urlMap, ok := change.Value.(map[string]any)
	if !ok {
		return
	}
	webhookURL, _ := urlMap["url"].(string)
	if webhookURL == "" {
		return
	}
	var allowlist []string
	if al, ok := urlMap["allowlist"].([]any); ok {
		for _, a := range al {
			if s, ok := a.(string); ok {
				allowlist = append(allowlist, s)
			}
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"activity_id": activityID,
		"rule_id":     rule.ID,
		"rule_name":   rule.Name,
	})
	if err := SendWebhook(webhookURL, payload, allowlist); err != nil {
		slog.Warn("webhook failed", "rule_id", rule.ID, "url", webhookURL, "error", err)
	}
}

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

	// Pre-compute track geofence results for this single activity
	injectTrackGeofenceResults(ctx, db, activityID, actMap, allRules)

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

		// === Branches-rules: evaluate each branch independently ===
		if len(rule.Branches) > 0 && string(rule.Branches) != "null" {
			type branch struct {
				Type            string                 `json:"type"` // "if", "else_if", "else"
				ConditionGroups []rules.ConditionGroup `json:"condition_groups"`
				Actions         json.RawMessage        `json:"actions"`
			}
			var branches []branch
			if err := json.Unmarshal(rule.Branches, &branches); err != nil {
				slog.Warn("malformed branches JSON", "rule_id", rule.ID, "error", err)
			} else if len(branches) > 0 {
				anyBranchMatched := false
				for i, br := range branches {
					branchMatched := false
					if br.Type == "else" {
						if i > 0 { // ELSE only valid after at least one IF/ELSE_IF
							branchMatched = true
						} else {
							continue // skip lone ELSE at position 0
						}
					} else if len(br.ConditionGroups) > 0 {
						branchMatched = rules.EvaluateGroups(br.ConditionGroups, actMap)
					}
					if branchMatched {
						anyBranchMatched = true
						var branchActions []rules.Action
						if err2 := json.Unmarshal(br.Actions, &branchActions); err2 == nil {
							applyChanges(ctx, db, activityID, userID, rule, branchActions, actMap)
						}
						break // first matching branch wins
					}
				}
				if anyBranchMatched {
					if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
						slog.Warn("increment rule applied", "rule_id", rule.ID, "error", err)
					}
					ruleMatchCounts[rule.ID]++
					if rule.StopOnMatch {
						break
					}
				} else if rule.IsSystem {
					// Retraction for branches-rules is not implemented; warn so it doesn't silently do nothing.
					slog.Warn("system rule with branches did not match — retraction for branches not implemented", "rule_id", rule.ID)
				}
			}
			continue // branches-rule handled, skip flat evaluation
		}

		// === Flat-condition rules ===
		matched := EvaluateRule(rule, actMap)

		if matched {
			applyChanges(ctx, db, activityID, userID, rule, actions, actMap)
			if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
				slog.Warn("increment rule applied", "rule_id", rule.ID, "error", err)
			}
			ruleMatchCounts[rule.ID]++
			if rule.StopOnMatch {
				break
			}
		} else if rule.IsSystem {
			// Retraction: system rules reset tags/fields when conditions no longer match
			changes := rules.Apply(actions)
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
// EvaluateRule checks whether a rule matches the given activity.
// For branches-rules, returns true if ANY branch matches.
// For flat-condition rules, evaluates conditions normally.
func EvaluateRule(rule model.AutomationRule, actMap map[string]any) bool {
	// Branches-rules: check if any branch matches
	if len(rule.Branches) > 0 && string(rule.Branches) != "null" {
		type branch struct {
			Type            string                 `json:"type"`
			ConditionGroups []rules.ConditionGroup `json:"condition_groups"`
		}
		var branches []branch
		if err := json.Unmarshal(rule.Branches, &branches); err == nil && len(branches) > 0 {
			for i, br := range branches {
				if br.Type == "else" {
					if i > 0 { // ELSE only valid after at least one IF/ELSE_IF
						return true
					}
					continue // skip lone ELSE at position 0
				}
				if len(br.ConditionGroups) > 0 && rules.EvaluateGroups(br.ConditionGroups, actMap) {
					return true
				}
			}
			return false
		}
	}
	// Flat conditions
	if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
		var groups []rules.ConditionGroup
		if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil && len(groups) > 0 {
			return rules.EvaluateGroups(groups, actMap)
		}
	}
	if len(rule.Conditions) > 0 {
		var conditions []rules.Condition
		if err := json.Unmarshal(rule.Conditions, &conditions); err == nil && len(conditions) > 0 {
			return rules.Evaluate(conditions, actMap)
		}
	}
	return false
}

var reToken = regexp.MustCompile(`\{([^}]+)\}`)

// resolveNameTemplate substitutes {token} and {token|fallback} placeholders in
// tmpl with values from the activity map. Supported tokens:
//
//   - Legacy aliases: {city} → start_city, {sport} → sport_type
//   - Computed:       {distance_km}, {weekday}, {elevation_gain}, {duration}
//   - Direct fields:  {start_city}, {start_state}, {start_country}, …
//
// applyChanges applies a set of rule actions to an activity, handling
// template resolution, notifications, webhooks, and DB changes.
func applyChanges(ctx context.Context, db *database.DB, activityID, userID string, rule model.AutomationRule, ruleActions []rules.Action, actMap map[string]any) {
	for _, change := range rules.Apply(ruleActions) {
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
		if change.Field == "webhook" {
			sendWebhookChange(activityID, rule, change)
			continue
		}
		if err := db.ApplyRuleChange(ctx, activityID, change.Field, val); err != nil {
			slog.Warn("apply rule change", "activity_id", activityID, "rule_id", rule.ID, "field", change.Field, "error", err)
		}
	}
}

// InjectTrackGeofenceResults pre-computes track intersection results for geofence
// conditions with check:"track" and injects them into the activity map.
func InjectTrackGeofenceResults(ctx context.Context, db *database.DB, activityID string, actMap map[string]any, allRules []model.AutomationRule) {
	injectTrackGeofenceResults(ctx, db, activityID, actMap, allRules)
}

// injectTrackGeofenceResults pre-computes track intersection results for geofence
// conditions with check:"track" and injects them into the activity map.
func injectTrackGeofenceResults(ctx context.Context, db *database.DB, activityID string, actMap map[string]any, allRules []model.AutomationRule) {
	type trackGeofence struct {
		key     string
		shape   string
		lat     float64
		lon     float64
		radiusM float64
		polygon string
	}

	seen := map[string]bool{}
	var geofences []trackGeofence

	for _, rule := range allRules {
		if !rule.IsActive {
			continue
		}
		scanGroups := func(raw json.RawMessage) {
			if len(raw) == 0 || string(raw) == "null" || !strings.Contains(string(raw), `"track"`) {
				return
			}
			var groups []rules.ConditionGroup
			if err := json.Unmarshal(raw, &groups); err != nil {
				return
			}
			for _, g := range groups {
				for _, c := range g.Conditions {
					if c.Field != "geofence" {
						continue
					}
					params, ok := c.Value.(map[string]any)
					if !ok {
						continue
					}
					check, _ := params["check"].(string)
					if check != "track" {
						continue
					}
					key := rules.GeofenceTrackKey(params)
					if seen[key] {
						continue
					}
					seen[key] = true
					shape, _ := params["shape"].(string)
					if shape == "" {
						shape = "circle"
					}
					gf := trackGeofence{key: key, shape: shape}
					if shape == "polygon" {
						gf.polygon, _ = params["polygon"].(string)
					} else {
						gf.lat, _ = params["lat"].(float64)
						gf.lon, _ = params["lon"].(float64)
						gf.radiusM, _ = params["radius_m"].(float64)
					}
					geofences = append(geofences, gf)
				}
			}
		}

		scanGroups(rule.ConditionGroups)

		if len(rule.Branches) > 0 && string(rule.Branches) != "null" {
			type branch struct {
				ConditionGroups json.RawMessage `json:"condition_groups"`
			}
			var branches []branch
			if err := json.Unmarshal(rule.Branches, &branches); err == nil {
				for _, b := range branches {
					scanGroups(b.ConditionGroups)
				}
			}
		}
	}

	if len(geofences) == 0 {
		return
	}

	ids := []string{activityID}
	for _, gf := range geofences {
		var result map[string]bool
		var err error
		if gf.shape == "polygon" {
			result, err = db.BatchTrackIntersectsPolygon(ctx, ids, gf.polygon)
		} else {
			result, err = db.BatchTrackIntersectsCircle(ctx, ids, gf.lat, gf.lon, gf.radiusM)
		}
		if err != nil {
			slog.Warn("track geofence query failed", "key", gf.key, "error", err)
			continue
		}
		actMap[gf.key] = result[activityID]
	}
}

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
				return fmt.Sprintf("%d km", int(math.Round(rules.ToFloat(d)/1000)))
			}
		case "weekday":
			if t, ok := actMap["started_at"].(time.Time); ok {
				days := []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
				return days[t.Weekday()]
			}
		case "elevation_gain":
			if v, ok := actMap["elevation_gain_m"]; ok && v != nil {
				return fmt.Sprintf("%.0f", rules.ToFloat(v))
			}
		case "duration":
			if v, ok := actMap["moving_time_s"]; ok && v != nil {
				s := int(rules.ToFloat(v))
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
