package rules

import (
	"encoding/json"
	"fmt"
)

type ruleActionMap map[string]any

type systemModifier struct {
	Action    string
	Field     string
	Param     string
	TagName   string
	HumanName string
}

var systemModifiers = []systemModifier{
	{Action: "set_electric", Field: "is_electric", Param: "electric", TagName: "E-Bike", HumanName: "E-Bike"},
	{Action: "set_indoor", Field: "is_indoor", Param: "indoor", TagName: "Indoor", HumanName: "Indoor"},
	{Action: "set_virtual", Field: "is_virtual", Param: "virtual", TagName: "virtuell", HumanName: "Virtuell"},
	{Action: "set_commute", Field: "is_commute", Param: "commute", TagName: "Pendeln", HumanName: "Pendeln"},
}

// NormalizeRuleActions validates and normalizes an automation-rule action array.
// tagNameByID is user-scoped metadata resolved by the caller; unknown tag IDs are
// preserved because imports may not have DB context for old explicit tag rules.
func NormalizeRuleActions(raw json.RawMessage, tagNameByID map[string]string) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return raw, nil
	}
	var actions []ruleActionMap
	if err := json.Unmarshal(raw, &actions); err != nil {
		return nil, fmt.Errorf("decode actions: %w", err)
	}
	normalized, err := normalizeActionMaps(actions, tagNameByID)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode actions: %w", err)
	}
	return out, nil
}

// NormalizeRuleBranches applies NormalizeRuleActions to every branch action list.
func NormalizeRuleBranches(raw json.RawMessage, tagNameByID map[string]string) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return raw, nil
	}
	var branches []map[string]any
	if err := json.Unmarshal(raw, &branches); err != nil {
		return nil, fmt.Errorf("decode branches: %w", err)
	}
	for i, branch := range branches {
		rawActions, ok := branch["actions"]
		if !ok {
			continue
		}
		actionBytes, err := json.Marshal(rawActions)
		if err != nil {
			return nil, fmt.Errorf("branches[%d].actions: encode: %w", i, err)
		}
		normalized, err := NormalizeRuleActions(actionBytes, tagNameByID)
		if err != nil {
			return nil, fmt.Errorf("branches[%d].actions: %w", i, err)
		}
		var decoded []ruleActionMap
		if err := json.Unmarshal(normalized, &decoded); err != nil {
			return nil, fmt.Errorf("branches[%d].actions: decode normalized: %w", i, err)
		}
		branch["actions"] = decoded
	}
	out, err := json.Marshal(branches)
	if err != nil {
		return nil, fmt.Errorf("encode branches: %w", err)
	}
	return out, nil
}

func normalizeActionMaps(actions []ruleActionMap, tagNameByID map[string]string) ([]ruleActionMap, error) {
	type modifierState struct {
		seen  bool
		value bool
	}
	states := map[string]modifierState{}
	out := make([]ruleActionMap, 0, len(actions))

	for _, action := range actions {
		actionName, _ := action["action"].(string)
		if modifier, stateKey, value, ok := modifierWriteFromAction(action); ok {
			if state := states[stateKey]; state.seen && state.value != value {
				return nil, fmt.Errorf("conflicting writes to %s", stateKey)
			}
			if err := validateModifierAgainstExistingTags(modifier, value, out, tagNameByID); err != nil {
				return nil, err
			}
			states[stateKey] = modifierState{seen: true, value: value}
			if value {
				out = removeSystemTagActions(out, modifier.TagName, tagNameByID, "add_tag")
			}
			out = append(out, action)
			continue
		}

		if actionName == "add_tag" || actionName == "remove_tag" {
			tagName := tagNameForAction(action, tagNameByID)
			if tagName != "" {
				for _, modifier := range systemModifiers {
					if tagName != modifier.TagName {
						continue
					}
					state := states[modifier.Action]
					if !state.seen {
						state = states[modifier.Field]
					}
					if actionName == "add_tag" && state.seen && state.value {
						goto skipAction
					}
					if actionName == "add_tag" && state.seen && !state.value {
						return nil, fmt.Errorf("%s(false) conflicts with add_tag(%s)", modifier.Action, modifier.HumanName)
					}
					if actionName == "remove_tag" && state.seen && state.value {
						return nil, fmt.Errorf("%s(true) conflicts with remove_tag(%s)", modifier.Action, modifier.HumanName)
					}
				}
			}
		}

		out = append(out, action)
	skipAction:
	}

	return out, nil
}

func modifierForAction(action string) (systemModifier, bool) {
	for _, modifier := range systemModifiers {
		if modifier.Action == action {
			return modifier, true
		}
	}
	return systemModifier{}, false
}

func modifierForField(field string) (systemModifier, bool) {
	for _, modifier := range systemModifiers {
		if modifier.Field == field {
			return modifier, true
		}
	}
	return systemModifier{}, false
}

func modifierWriteFromAction(action ruleActionMap) (systemModifier, string, bool, bool) {
	actionName, _ := action["action"].(string)
	if modifier, ok := modifierForAction(actionName); ok {
		return modifier, modifier.Field, actionBoolParam(action, modifier.Param, true), true
	}
	if actionName != "set_field" {
		return systemModifier{}, "", false, false
	}
	params, _ := action["params"].(map[string]any)
	if params == nil {
		return systemModifier{}, "", false, false
	}
	field, _ := params["field"].(string)
	modifier, ok := modifierForField(field)
	if !ok {
		return systemModifier{}, "", false, false
	}
	value, ok := params["value"].(bool)
	return modifier, field, value, ok
}

func actionBoolParam(action ruleActionMap, param string, fallback bool) bool {
	params, _ := action["params"].(map[string]any)
	if params == nil {
		return fallback
	}
	if v, ok := params[param].(bool); ok {
		return v
	}
	if v, ok := params["value"].(bool); ok {
		return v
	}
	return fallback
}

func tagNameForAction(action ruleActionMap, tagNameByID map[string]string) string {
	params, _ := action["params"].(map[string]any)
	if params == nil {
		return ""
	}
	tagID, _ := params["tag_id"].(string)
	if tagID == "" {
		return ""
	}
	return tagNameByID[tagID]
}

func validateModifierAgainstExistingTags(modifier systemModifier, value bool, actions []ruleActionMap, tagNameByID map[string]string) error {
	for _, action := range actions {
		actionName, _ := action["action"].(string)
		tagName := tagNameForAction(action, tagNameByID)
		if tagName != modifier.TagName {
			continue
		}
		if actionName == "add_tag" && !value {
			return fmt.Errorf("%s(false) conflicts with add_tag(%s)", modifier.Action, modifier.HumanName)
		}
		if actionName == "remove_tag" && value {
			return fmt.Errorf("%s(true) conflicts with remove_tag(%s)", modifier.Action, modifier.HumanName)
		}
	}
	return nil
}

func removeSystemTagActions(actions []ruleActionMap, tagName string, tagNameByID map[string]string, actionName string) []ruleActionMap {
	out := actions[:0]
	for _, action := range actions {
		name, _ := action["action"].(string)
		if name == actionName && tagNameForAction(action, tagNameByID) == tagName {
			continue
		}
		out = append(out, action)
	}
	return out
}
