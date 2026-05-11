package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

var allowedGearConditionFields = map[string]struct{}{
	"gear_id":                     {},
	"gear_total_distance_m":       {},
	"gear_total_moving_time_s":    {},
	"gear_total_time_s":           {},
	"gear_total_elevation_gain_m": {},
	"gear_total_elevation_loss_m": {},
	"gear_total_activities":       {},
}

func IsGearConditionField(field string) bool {
	_, ok := allowedGearConditionFields[field]
	return ok
}

// ValidateGearConditionJSON rejects unknown gear condition fields in rule JSON.
// Non-gear fields remain handled by the existing broad rule surface.
func ValidateGearConditionJSON(conditions, conditionGroups, branches json.RawMessage) error {
	if err := validateGearConditionArrayRaw(conditions, "conditions"); err != nil {
		return err
	}
	if err := validateGearConditionGroupsRaw(conditionGroups, "condition_groups"); err != nil {
		return err
	}
	if err := validateGearConditionBranchesRaw(branches); err != nil {
		return err
	}
	return nil
}

func validateGearConditionArrayRaw(raw json.RawMessage, surface string) error {
	if isEmptyJSON(raw) || !startsWithJSONDelim(raw, '[') {
		return nil
	}
	var conditions []Condition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return fmt.Errorf("%s: decode conditions: %w", surface, err)
	}
	return validateGearConditions(conditions, surface)
}

func validateGearConditionGroupsRaw(raw json.RawMessage, surface string) error {
	if isEmptyJSON(raw) || !startsWithJSONDelim(raw, '[') {
		return nil
	}
	var groups []ConditionGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		return fmt.Errorf("%s: decode condition groups: %w", surface, err)
	}
	for i, group := range groups {
		if err := validateGearConditions(group.Conditions, fmt.Sprintf("%s[%d]", surface, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateGearConditionBranchesRaw(raw json.RawMessage) error {
	if isEmptyJSON(raw) || !startsWithJSONDelim(raw, '[') {
		return nil
	}
	var branches []struct {
		ConditionGroups json.RawMessage `json:"condition_groups"`
	}
	if err := json.Unmarshal(raw, &branches); err != nil {
		return fmt.Errorf("branches: decode branches: %w", err)
	}
	for i, branch := range branches {
		if err := validateGearConditionGroupsRaw(branch.ConditionGroups, fmt.Sprintf("branches[%d].condition_groups", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateGearConditions(conditions []Condition, surface string) error {
	for i, condition := range conditions {
		field := strings.TrimSpace(condition.Field)
		if field == "" {
			continue
		}
		if IsGearConditionField(field) {
			continue
		}
		if field == "gear" || strings.HasPrefix(field, "gear") {
			return fmt.Errorf("%s[%d]: unknown gear condition field %q", surface, i, condition.Field)
		}
	}
	return nil
}
