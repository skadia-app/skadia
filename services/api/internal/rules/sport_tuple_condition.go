package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

var sportTupleModifierKeys = map[string]struct{}{
	"is_electric": {},
	"is_indoor":   {},
	"is_virtual":  {},
	"is_commute":  {},
}

type sportTupleConditionValue struct {
	SportType      string
	SubType        string
	SubTypePresent bool
	Modifiers      map[string]bool
}

func ValidateSportTupleCondition(condition Condition, surface string) error {
	if condition.Field != "sport_tuple" {
		return fmt.Errorf("%s: field must be sport_tuple", surface)
	}
	if strings.ToLower(strings.TrimSpace(condition.Op)) != "in" {
		return fmt.Errorf("%s: sport_tuple op must be in", surface)
	}
	tuples, err := parseSportTupleConditionValues(condition.Value, surface)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for i, tuple := range tuples {
		if err := validateSportTupleConditionValue(tuple, fmt.Sprintf("%s.value[%d]", surface, i)); err != nil {
			return err
		}
		key := sportTupleConditionStableKey(tuple)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s.value[%d]: duplicate sport_tuple object", surface, i)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func evalSportTupleIn(condition Condition, activity map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(condition.Op)) != "in" {
		return false
	}
	tuples, err := parseSportTupleConditionValues(condition.Value, "sport_tuple")
	if err != nil {
		return false
	}
	activitySport := toStringValue(activity["sport_type"])
	activitySubType := toStringValue(activity["sub_type"])
	for _, tuple := range tuples {
		if tuple.SportType != activitySport {
			continue
		}
		if tuple.SubTypePresent && tuple.SubType != activitySubType {
			continue
		}
		if sportTupleModifiersMatch(tuple.Modifiers, activity) {
			return true
		}
	}
	return false
}

func sportTupleModifiersMatch(modifiers map[string]bool, activity map[string]any) bool {
	for key, want := range modifiers {
		raw, ok := activity[key]
		if !ok {
			return false
		}
		got, ok := raw.(bool)
		if !ok || got != want {
			return false
		}
	}
	return true
}

func parseSportTupleConditionValues(value any, surface string) ([]sportTupleConditionValue, error) {
	raw, err := sportTupleConditionJSON(value)
	if err != nil {
		return nil, fmt.Errorf("%s: sport_tuple value must be a JSON array", surface)
	}
	var items []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("%s: sport_tuple value must be a JSON object array: %w", surface, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%s: sport_tuple value must be non-empty", surface)
	}
	out := make([]sportTupleConditionValue, 0, len(items))
	for i, item := range items {
		tuple, err := parseSportTupleConditionValue(item, fmt.Sprintf("%s.value[%d]", surface, i))
		if err != nil {
			return nil, err
		}
		out = append(out, tuple)
	}
	return out, nil
}

func sportTupleConditionJSON(value any) ([]byte, error) {
	switch v := value.(type) {
	case json.RawMessage:
		return v, nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

func parseSportTupleConditionValue(item map[string]any, surface string) (sportTupleConditionValue, error) {
	tuple := sportTupleConditionValue{Modifiers: map[string]bool{}}
	for key, raw := range item {
		switch key {
		case "sport_type":
			value, ok := raw.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return sportTupleConditionValue{}, fmt.Errorf("%s.sport_type must be a string", surface)
			}
			tuple.SportType = strings.TrimSpace(value)
		case "sub_type":
			value, ok := raw.(string)
			if !ok {
				return sportTupleConditionValue{}, fmt.Errorf("%s.sub_type must be a string", surface)
			}
			tuple.SubType = strings.TrimSpace(value)
			tuple.SubTypePresent = true
		case "is_electric", "is_indoor", "is_virtual", "is_commute":
			value, ok := raw.(bool)
			if !ok {
				return sportTupleConditionValue{}, fmt.Errorf("%s.%s must be a boolean", surface, key)
			}
			tuple.Modifiers[key] = value
		default:
			return sportTupleConditionValue{}, fmt.Errorf("%s: unsupported key %q in sport_tuple object", surface, key)
		}
	}
	if tuple.SportType == "" {
		return sportTupleConditionValue{}, fmt.Errorf("%s.sport_type is required", surface)
	}
	return tuple, nil
}

func validateSportTupleConditionValue(tuple sportTupleConditionValue, surface string) error {
	def, ok := sport.GetSportDef(tuple.SportType)
	if !ok {
		return fmt.Errorf("%s.sport_type must be a canonical sport", surface)
	}
	if tuple.SubTypePresent && tuple.SubType != "" {
		if _, ok := def.SubTypes[tuple.SubType]; !ok {
			return fmt.Errorf("%s.sub_type %q is unknown for sport_type %q", surface, tuple.SubType, tuple.SportType)
		}
	}
	for key := range tuple.Modifiers {
		if _, ok := sportTupleModifierKeys[key]; !ok {
			return fmt.Errorf("%s: unsupported modifier key %q", surface, key)
		}
	}
	return nil
}

func sportTupleConditionStableKey(tuple sportTupleConditionValue) string {
	parts := []string{tuple.SportType}
	if tuple.SubTypePresent {
		parts = append(parts, "sub_type="+tuple.SubType)
	} else {
		parts = append(parts, "sub_type=*")
	}
	keys := make([]string, 0, len(tuple.Modifiers))
	for key := range tuple.Modifiers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%t", key, tuple.Modifiers[key]))
	}
	return strings.Join(parts, "|")
}
