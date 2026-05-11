package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

// EvaluateGearLifecycleRule evaluates a gear-lifecycle rule against current
// materialized gear usage. It returns matching gear IDs and records stale
// explicit gear references as non-blocking rule warnings.
func EvaluateGearLifecycleRule(ctx context.Context, db *database.DB, userID string, rule model.AutomationRule) ([]string, error) {
	gearMaps, err := lifecycleGearMaps(ctx, db, userID)
	if err != nil {
		return nil, err
	}

	explicitGearIDs := gearIDConditions(rule)
	var matched []string
	for _, gearMap := range gearMaps {
		if !EvaluateGearLifecycleMap(rule, gearMap) {
			continue
		}
		gearID, _ := gearMap["gear_id"].(string)
		if gearID != "" {
			matched = append(matched, gearID)
		}
	}

	invalidReference := false
	for _, gearID := range explicitGearIDs {
		message, invalid, err := gearLifecycleReferenceWarning(ctx, db, userID, gearID)
		if err != nil {
			return nil, err
		}
		if invalid {
			invalidReference = true
			if err := db.SetRuleWarning(ctx, rule.ID, userID, ruleWarningGearReferenceInvalid, message); err != nil {
				slog.Warn("set gear lifecycle rule warning", "rule_id", rule.ID, "error", err)
			}
			break
		}
	}
	if !invalidReference {
		if err := db.ClearRuleWarning(ctx, rule.ID, userID); err != nil {
			slog.Warn("clear gear lifecycle rule warning", "rule_id", rule.ID, "error", err)
		}
	}
	return matched, nil
}

func EvaluateGearLifecycleMap(rule model.AutomationRule, gearMap map[string]any) bool {
	if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
		var groups []rules.ConditionGroup
		if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil && len(groups) > 0 {
			return rules.EvaluateGroups(groups, gearMap)
		}
	}
	if len(rule.Conditions) > 0 && string(rule.Conditions) != "null" {
		var conditions []rules.Condition
		if err := json.Unmarshal(rule.Conditions, &conditions); err == nil && len(conditions) > 0 {
			return rules.Evaluate(conditions, gearMap)
		}
	}
	return false
}

func lifecycleGearMaps(ctx context.Context, db *database.DB, userID string) ([]map[string]any, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id::text, total_distance_m, total_moving_time_s, total_time_s,
		       total_elevation_gain_m, total_elevation_loss_m, total_activities
		FROM gear
		WHERE user_id = $1 AND is_retired = false
		ORDER BY name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list gear lifecycle candidates: %w", err)
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var id string
		var totalDistance, totalGain, totalLoss float64
		var totalMoving, totalTime, totalActivities int
		if err := rows.Scan(&id, &totalDistance, &totalMoving, &totalTime, &totalGain, &totalLoss, &totalActivities); err != nil {
			return nil, fmt.Errorf("scan gear lifecycle candidate: %w", err)
		}
		items = append(items, map[string]any{
			"gear_id":                     id,
			"gear_total_distance_m":       totalDistance,
			"gear_total_moving_time_s":    totalMoving,
			"gear_total_time_s":           totalTime,
			"gear_total_elevation_gain_m": totalGain,
			"gear_total_elevation_loss_m": totalLoss,
			"gear_total_activities":       totalActivities,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gear lifecycle candidates: %w", err)
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items, nil
}

func gearIDConditions(rule model.AutomationRule) []string {
	seen := map[string]bool{}
	var out []string
	for _, condition := range lifecycleRuleConditions(rule) {
		if condition.Field != "gear_id" {
			continue
		}
		for _, value := range gearIDConditionValues(condition) {
			if value != "" && !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	return out
}

func gearIDConditionValues(condition rules.Condition) []string {
	switch condition.Op {
	case "eq":
		if value, ok := condition.Value.(string); ok {
			return []string{value}
		}
	case "in":
		rawValues, ok := condition.Value.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(rawValues))
		for _, raw := range rawValues {
			if value, ok := raw.(string); ok {
				out = append(out, value)
			}
		}
		return out
	}
	return nil
}

func lifecycleRuleConditions(rule model.AutomationRule) []rules.Condition {
	var out []rules.Condition
	if len(rule.Conditions) > 0 && string(rule.Conditions) != "null" {
		var conditions []rules.Condition
		if err := json.Unmarshal(rule.Conditions, &conditions); err == nil {
			out = append(out, conditions...)
		}
	}
	if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
		var groups []rules.ConditionGroup
		if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil {
			for _, group := range groups {
				out = append(out, group.Conditions...)
			}
		}
	}
	return out
}

func gearLifecycleReferenceWarning(ctx context.Context, db *database.DB, userID, gearID string) (string, bool, error) {
	var name, status string
	var isRetired bool
	err := db.Pool.QueryRow(ctx, `
		SELECT name, status, is_retired
		FROM gear
		WHERE id = $1 AND user_id = $2`, gearID, userID).Scan(&name, &status, &isRetired)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", false, fmt.Errorf("check gear lifecycle reference: %w", err)
		}
		return "Ausrüstung id=" + gearID + " ist nicht mehr für Gear-Lifecycle-Regeln verfügbar", true, nil
	}
	if isRetired || status == "retired" {
		return "Ausrüstung " + name + " (id=" + gearID + ") ist ausgemustert und nicht mehr für Gear-Lifecycle-Regeln verfügbar", true, nil
	}
	return "", false, nil
}
