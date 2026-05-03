package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

var sportTraitConditionFields = map[string]struct{}{
	"sport_environment":      {},
	"sport_equipment_family": {},
	"sport_movement_family":  {},
}

func ValidateSportTraitCondition(condition Condition, surface string) error {
	if _, ok := sportTraitConditionFields[condition.Field]; !ok {
		return fmt.Errorf("%s: unsupported trait field %q", surface, condition.Field)
	}
	if strings.ToLower(strings.TrimSpace(condition.Op)) != "in" {
		return fmt.Errorf("%s: %s op must be in", surface, condition.Field)
	}
	values, err := parseSportTraitConditionValues(condition.Value, surface)
	if err != nil {
		return err
	}
	allowed := sport.RuleTraitValues(condition.Field)
	for i, value := range values {
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("%s.value[%d]: unknown trait value %q for %s", surface, i, value, condition.Field)
		}
	}
	return nil
}

func evalSportTraitIn(condition Condition, activity map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(condition.Op)) != "in" {
		return false
	}
	values, err := parseSportTraitConditionValues(condition.Value, condition.Field)
	if err != nil {
		return false
	}
	traits, ok := sport.EffectiveTraitsForTuple(toStringValue(activity["sport_type"]), activitySubTypeValue(activity))
	if !ok {
		return false
	}
	candidates := sportTraitValuesForField(traits, condition.Field)
	for _, want := range values {
		for _, candidate := range candidates {
			if candidate == want {
				return true
			}
		}
	}
	return false
}

func isSportTraitConditionField(field string) bool {
	_, ok := sportTraitConditionFields[field]
	return ok
}

func parseSportTraitConditionValues(value any, surface string) ([]string, error) {
	raw, err := sportTraitConditionJSON(value)
	if err != nil {
		return nil, fmt.Errorf("%s: trait condition value must be a JSON array", surface)
	}
	var items []any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("%s: trait condition value must be an array: %w", surface, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%s: trait condition value must be non-empty", surface)
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for i, item := range items {
		value, ok := item.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s.value[%d] must be a non-empty string", surface, i)
		}
		value = strings.TrimSpace(value)
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func sportTraitConditionJSON(value any) ([]byte, error) {
	switch v := value.(type) {
	case json.RawMessage:
		return v, nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

func sportTraitValuesForField(traits sport.SportTraits, field string) []string {
	switch field {
	case "sport_environment":
		return traits.Environments
	case "sport_equipment_family":
		return traits.EquipmentFamilies
	case "sport_movement_family":
		return traits.MovementFamilies
	default:
		return nil
	}
}
