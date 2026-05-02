package rules

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type SportTaxonomyRewriteWarning struct {
	Surface string
	Reason  string
}

type SportTaxonomyRuleJSON struct {
	Conditions      json.RawMessage
	ConditionGroups json.RawMessage
	Branches        json.RawMessage
	Actions         json.RawMessage
}

func RewriteSportTaxonomyRuleJSON(input SportTaxonomyRuleJSON) (SportTaxonomyRuleJSON, []SportTaxonomyRewriteWarning, error) {
	out := input
	var warnings []SportTaxonomyRewriteWarning
	var err error

	out.Conditions, err = rewriteConditionArrayRaw(input.Conditions, "conditions", false)
	if err != nil {
		return SportTaxonomyRuleJSON{}, warnings, err
	}
	out.ConditionGroups, err = rewriteConditionGroupsRaw(input.ConditionGroups, "condition_groups")
	if err != nil {
		return SportTaxonomyRuleJSON{}, warnings, err
	}
	out.Actions, err = rewriteActionsRaw(input.Actions, "actions")
	if err != nil {
		return SportTaxonomyRuleJSON{}, warnings, err
	}
	out.Branches, err = rewriteBranchesRaw(input.Branches)
	if err != nil {
		return SportTaxonomyRuleJSON{}, warnings, err
	}
	return out, warnings, nil
}

func rewriteConditionArrayRaw(raw json.RawMessage, surface string, rejectLegacyInOR bool) (json.RawMessage, error) {
	if isEmptyJSON(raw) {
		return raw, nil
	}
	if !startsWithJSONDelim(raw, '[') {
		return raw, nil
	}
	var conditions []Condition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return nil, fmt.Errorf("%s: decode conditions: %w", surface, err)
	}
	rewritten, err := rewriteConditions(conditions, surface, rejectLegacyInOR)
	if err != nil {
		return nil, err
	}
	return marshalRaw(rewritten, surface)
}

func rewriteConditionGroupsRaw(raw json.RawMessage, surface string) (json.RawMessage, error) {
	if isEmptyJSON(raw) {
		return raw, nil
	}
	if !startsWithJSONDelim(raw, '[') {
		return raw, nil
	}
	var groups []ConditionGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, fmt.Errorf("%s: decode condition groups: %w", surface, err)
	}
	for i := range groups {
		logic := strings.ToLower(strings.TrimSpace(groups[i].Logic))
		rejectLegacyInOR := logic == "or"
		rewritten, err := rewriteConditions(groups[i].Conditions, fmt.Sprintf("%s[%d]", surface, i), rejectLegacyInOR)
		if err != nil {
			return nil, err
		}
		groups[i].Conditions = rewritten
	}
	return marshalRaw(groups, surface)
}

func rewriteConditions(conditions []Condition, surface string, rejectLegacyInOR bool) ([]Condition, error) {
	rewritten := make([]Condition, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Field != "sport_type" {
			rewritten = append(rewritten, condition)
			continue
		}
		op := strings.ToLower(strings.TrimSpace(condition.Op))
		if op != "eq" {
			if err := validateCanonicalSportCondition(condition, surface, op); err != nil {
				return nil, err
			}
			rewritten = append(rewritten, condition)
			continue
		}
		value, ok := condition.Value.(string)
		if !ok {
			return nil, fmt.Errorf("%s: sport_type condition value must be a string", surface)
		}
		tuple, patch, err := sport.NormalizeSportTuple(value, "")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", surface, err)
		}
		if rejectLegacyInOR && sportConditionNeedsTupleRewrite(value, tuple, patch) {
			return nil, fmt.Errorf("%s: or group contains legacy sport predicate %q", surface, value)
		}
		rewritten = append(rewritten, Condition{Field: "sport_type", Op: "eq", Value: tuple.SportType})
		if tuple.SubType != "" {
			rewritten = append(rewritten, Condition{Field: "sub_type", Op: "eq", Value: tuple.SubType})
		}
		if patch.IsElectric != nil {
			rewritten = append(rewritten, Condition{Field: "is_electric", Op: "eq", Value: *patch.IsElectric})
		}
		if patch.IsIndoor != nil {
			rewritten = append(rewritten, Condition{Field: "is_indoor", Op: "eq", Value: *patch.IsIndoor})
		}
	}
	return rewritten, nil
}

func validateCanonicalSportCondition(condition Condition, surface string, op string) error {
	switch op {
	case "neq":
		value, ok := condition.Value.(string)
		if !ok {
			return fmt.Errorf("%s: sport_type condition value must be a string", surface)
		}
		if err := validateCanonicalSportValue(value, surface, op); err != nil {
			return err
		}
		return nil
	case "in", "not_in":
		values, err := sportConditionListValues(condition.Value)
		if err != nil {
			return fmt.Errorf("%s: sport_type %s condition value must be a string array", surface, op)
		}
		for _, value := range values {
			if err := validateCanonicalSportValue(value, surface, op); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported sport_type operator %q for sport taxonomy rewrite", surface, condition.Op)
	}
}

func validateCanonicalSportValue(value string, surface string, op string) error {
	tuple, patch, err := sport.NormalizeSportTuple(value, "")
	if err != nil {
		return fmt.Errorf("%s: %w", surface, err)
	}
	if sportConditionNeedsTupleRewrite(value, tuple, patch) {
		return fmt.Errorf("%s: unsupported sport_type operator %q for legacy or subtype sport predicate %q", surface, op, value)
	}
	return nil
}

func sportConditionListValues(value any) ([]string, error) {
	switch values := value.(type) {
	case []string:
		return values, nil
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			value, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("non-string item")
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("not a list")
	}
}

func sportConditionNeedsTupleRewrite(value string, tuple sport.Tuple, patch sport.ModifierPatch) bool {
	return value != tuple.SportType || tuple.SubType != "" || patch.IsElectric != nil || patch.IsIndoor != nil || patch.IsVirtual != nil
}

func rewriteActionsRaw(raw json.RawMessage, surface string) (json.RawMessage, error) {
	if isEmptyJSON(raw) {
		return raw, nil
	}
	if !startsWithJSONDelim(raw, '[') {
		return raw, nil
	}
	var actions []map[string]any
	if err := json.Unmarshal(raw, &actions); err != nil {
		return nil, fmt.Errorf("%s: decode actions: %w", surface, err)
	}
	rewritten, err := rewriteActionMaps(actions, surface)
	if err != nil {
		return nil, err
	}
	return marshalRaw(rewritten, surface)
}

func rewriteActionMaps(actions []map[string]any, surface string) ([]map[string]any, error) {
	rewritten := make([]map[string]any, 0, len(actions))
	modifierWrites := map[string]modifierWriteState{}
	for i, action := range actions {
		name := actionName(action)
		if converted, ok, err := rewriteSetFieldSportType(action, surface, i); ok || err != nil {
			if err != nil {
				return nil, err
			}
			action = converted
			name = actionName(action)
		}
		if name != "set_sport_type" {
			if field, value, ok := modifierWrite(action); ok {
				if previous, exists := modifierWrites[field]; exists {
					if previous.value != value {
						return nil, fmt.Errorf("%s[%d]: conflicting writes to %s (previous action index %d)", surface, i, field, previous.index)
					}
				} else {
					modifierWrites[field] = modifierWriteState{index: i, value: value}
				}
			}
			rewritten = append(rewritten, action)
			continue
		}
		params, _ := action["params"].(map[string]any)
		if params == nil {
			rewritten = append(rewritten, action)
			continue
		}
		rawSport, _ := params["sport_type"].(string)
		if rawSport == "" {
			rewritten = append(rewritten, action)
			continue
		}
		rawSubType, _ := params["sub_type"].(string)
		tuple, patch, err := sport.NormalizeSportTuple(rawSport, rawSubType)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", surface, i, err)
		}
		newParams := cloneMap(params)
		newParams["sport_type"] = tuple.SportType
		if tuple.SubType != "" {
			newParams["sub_type"] = tuple.SubType
		} else {
			delete(newParams, "sub_type")
		}
		newAction := cloneMap(action)
		newAction["action"] = "set_sport_type"
		delete(newAction, "type")
		newAction["params"] = newParams
		rewritten = append(rewritten, newAction)
		if patch.IsElectric != nil {
			if previous, exists := modifierWrites["is_electric"]; exists {
				if previous.value != *patch.IsElectric {
					return nil, fmt.Errorf("%s[%d]: conflicting writes to is_electric (previous action index %d)", surface, i, previous.index)
				}
			} else {
				modifierWrites["is_electric"] = modifierWriteState{index: i, value: *patch.IsElectric}
				rewritten = append(rewritten, map[string]any{
					"action": "set_electric",
					"params": map[string]any{"electric": *patch.IsElectric},
				})
			}
		}
		if patch.IsIndoor != nil {
			if previous, exists := modifierWrites["is_indoor"]; exists {
				if previous.value != *patch.IsIndoor {
					return nil, fmt.Errorf("%s[%d]: conflicting writes to is_indoor (previous action index %d)", surface, i, previous.index)
				}
			} else {
				modifierWrites["is_indoor"] = modifierWriteState{index: i, value: *patch.IsIndoor}
				rewritten = append(rewritten, map[string]any{
					"action": "set_indoor",
					"params": map[string]any{"indoor": *patch.IsIndoor},
				})
			}
		}
		if patch.IsVirtual != nil {
			if previous, exists := modifierWrites["is_virtual"]; exists {
				if previous.value != *patch.IsVirtual {
					return nil, fmt.Errorf("%s[%d]: conflicting writes to is_virtual (previous action index %d)", surface, i, previous.index)
				}
			} else {
				modifierWrites["is_virtual"] = modifierWriteState{index: i, value: *patch.IsVirtual}
				rewritten = append(rewritten, map[string]any{
					"action": "set_virtual",
					"params": map[string]any{"virtual": *patch.IsVirtual},
				})
			}
		}
	}
	return rewritten, nil
}

type modifierWriteState struct {
	index int
	value bool
}

func rewriteSetFieldSportType(action map[string]any, surface string, index int) (map[string]any, bool, error) {
	if actionName(action) != "set_field" {
		return nil, false, nil
	}
	params, _ := action["params"].(map[string]any)
	if params == nil {
		return nil, false, nil
	}
	field, _ := params["field"].(string)
	if field != "sport_type" {
		return nil, false, nil
	}
	rawSport, _ := params["value"].(string)
	if rawSport == "" {
		return nil, false, fmt.Errorf("%s[%d]: set_field sport_type value must be a string", surface, index)
	}
	newAction := cloneMap(action)
	newAction["action"] = "set_sport_type"
	delete(newAction, "type")
	newAction["params"] = map[string]any{"sport_type": rawSport}
	return newAction, true, nil
}

func modifierWrite(action map[string]any) (string, bool, bool) {
	switch actionName(action) {
	case "set_electric":
		return "is_electric", boolParam(action, "electric", true), true
	case "set_indoor":
		return "is_indoor", boolParam(action, "indoor", true), true
	case "set_virtual":
		return "is_virtual", boolParam(action, "virtual", true), true
	case "set_commute":
		return "is_commute", boolParam(action, "commute", true), true
	case "set_field":
		params, _ := action["params"].(map[string]any)
		field, _ := params["field"].(string)
		switch field {
		case "is_electric", "is_indoor", "is_virtual", "is_commute":
			value, ok := params["value"].(bool)
			return field, value, ok
		}
	}
	return "", false, false
}

func boolParam(action map[string]any, name string, fallback bool) bool {
	params, _ := action["params"].(map[string]any)
	if params == nil {
		return fallback
	}
	if value, ok := params[name].(bool); ok {
		return value
	}
	if value, ok := params["value"].(bool); ok {
		return value
	}
	return fallback
}

func rewriteBranchesRaw(raw json.RawMessage) (json.RawMessage, error) {
	if isEmptyJSON(raw) {
		return raw, nil
	}
	if !startsWithJSONDelim(raw, '[') {
		return raw, nil
	}
	var branches []map[string]any
	if err := json.Unmarshal(raw, &branches); err != nil {
		return nil, fmt.Errorf("branches: decode: %w", err)
	}
	for i, branch := range branches {
		if rawGroups, ok, err := rawMessageFromValue(branch["condition_groups"]); err != nil {
			return nil, fmt.Errorf("branches[%d].condition_groups: %w", i, err)
		} else if ok {
			rewritten, err := rewriteConditionGroupsRaw(rawGroups, fmt.Sprintf("branches[%d].condition_groups", i))
			if err != nil {
				return nil, err
			}
			var decoded any
			if err := json.Unmarshal(rewritten, &decoded); err != nil {
				return nil, fmt.Errorf("branches[%d].condition_groups: decode rewritten: %w", i, err)
			}
			branch["condition_groups"] = decoded
		}
		if rawActions, ok, err := rawMessageFromValue(branch["actions"]); err != nil {
			return nil, fmt.Errorf("branches[%d].actions: %w", i, err)
		} else if ok {
			rewritten, err := rewriteActionsRaw(rawActions, fmt.Sprintf("branches[%d].actions", i))
			if err != nil {
				return nil, err
			}
			var decoded any
			if err := json.Unmarshal(rewritten, &decoded); err != nil {
				return nil, fmt.Errorf("branches[%d].actions: decode rewritten: %w", i, err)
			}
			branch["actions"] = decoded
		}
	}
	return marshalRaw(branches, "branches")
}

func actionName(action map[string]any) string {
	if name, _ := action["action"].(string); name != "" {
		return name
	}
	name, _ := action["type"].(string)
	return name
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func rawMessageFromValue(value any) (json.RawMessage, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	switch v := value.(type) {
	case json.RawMessage:
		return v, true, nil
	case []byte:
		return json.RawMessage(v), true, nil
	case string:
		return json.RawMessage(v), true, nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false, err
		}
		return raw, true, nil
	}
}

func isEmptyJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

func startsWithJSONDelim(raw json.RawMessage, delim byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed[0] == delim
}

func marshalRaw(value any, surface string) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: encode: %w", surface, err)
	}
	return raw, nil
}
