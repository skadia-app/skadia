package rules

import (
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/tags"
)

type ruleActionMap map[string]any

type systemModifier struct {
	Action        string
	Field         string
	Param         string
	TagName       string
	HumanName     string
	OnlySportType string
}

func systemModifiers() []systemModifier {
	out := make([]systemModifier, 0)
	for _, key := range RulesApplicableModifierKeys {
		def, ok := sport.SportModifierByKey(key)
		if !ok {
			continue
		}
		modifier := systemModifier{
			Action:    def.ActionName,
			Field:     def.JSONKey,
			Param:     def.ActionParam,
			HumanName: def.LabelDE,
		}
		if s, ok := tags.SystemTagSyncByModifierKey(key); ok {
			modifier.TagName = s.SystemTag
			modifier.OnlySportType = s.OnlySportType
		}
		out = append(out, modifier)
	}
	return out
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
	cyclingActionContext := ruleActionsSetCycling(actions)

	for i, action := range actions {
		if err := validateRuleActionParams(action, fmt.Sprintf("actions[%d]", i)); err != nil {
			return nil, err
		}
		actionName, _ := action["action"].(string)
		if modifier, stateKey, value, ok := modifierWriteFromAction(action); ok {
			if state := states[stateKey]; state.seen && state.value != value {
				return nil, fmt.Errorf("conflicting writes to %s", stateKey)
			}
			if err := validateModifierAgainstExistingTags(modifier, value, out, tagNameByID, cyclingActionContext); err != nil {
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
				for _, modifier := range systemModifiers() {
					if !systemTagAppliesToModifier(modifier, tagName, cyclingActionContext) {
						continue
					}
					state := states[modifier.Field]
					if actionName == "add_tag" && state.seen && state.value {
						goto skipAction
					}
					if actionName == "add_tag" && state.seen && !state.value {
						return nil, fmt.Errorf("%s=false conflicts with add_tag(%s)", modifier.HumanName, modifier.TagName)
					}
					if actionName == "remove_tag" && state.seen && state.value {
						return nil, fmt.Errorf("%s=true conflicts with remove_tag(%s)", modifier.HumanName, modifier.TagName)
					}
				}
			}
		}

		out = append(out, action)
	skipAction:
	}

	return out, nil
}

func validateRuleActionParams(action ruleActionMap, surface string) error {
	actionName, _ := action["action"].(string)
	params, _ := action["params"].(map[string]any)
	switch actionName {
	case "set_gear":
		if !nonEmptyStringParam(params, "gear_id") {
			return fmt.Errorf("%s: set_gear requires params.gear_id", surface)
		}
	case "add_tag", "remove_tag":
		if !nonEmptyStringParam(params, "tag_id") {
			return fmt.Errorf("%s: %s requires params.tag_id", surface, actionName)
		}
	case "set_visibility":
		if !nonEmptyStringParam(params, "visibility") {
			return fmt.Errorf("%s: set_visibility requires params.visibility", surface)
		}
	case "set_sport_type":
		if !nonEmptyStringParam(params, "sport_type") {
			return fmt.Errorf("%s: set_sport_type requires params.sport_type", surface)
		}
	case "set_name":
		if !nonEmptyStringParam(params, "template") {
			return fmt.Errorf("%s: set_name requires params.template", surface)
		}
	case "set_intensity":
		if !nonEmptyStringParam(params, "intensity") {
			return fmt.Errorf("%s: set_intensity requires params.intensity", surface)
		}
	case "set_field":
		if !nonEmptyStringParam(params, "field") {
			return fmt.Errorf("%s: set_field requires params.field", surface)
		}
		if params == nil {
			return fmt.Errorf("%s: set_field requires params.value", surface)
		}
		if _, ok := params["value"]; !ok {
			return fmt.Errorf("%s: set_field requires params.value", surface)
		}
	case "notify", "webhook":
		if !nonEmptyStringParam(params, "template") && actionName == "notify" {
			return fmt.Errorf("%s: notify requires params.template", surface)
		}
		if !nonEmptyStringParam(params, "url") && actionName == "webhook" {
			return fmt.Errorf("%s: webhook requires params.url", surface)
		}
	}
	return nil
}

func nonEmptyStringParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	value, _ := params[key].(string)
	return value != ""
}

func ruleActionsSetCycling(actions []ruleActionMap) bool {
	for _, action := range actions {
		actionName, _ := action["action"].(string)
		if actionName != "set_sport_type" {
			continue
		}
		params, _ := action["params"].(map[string]any)
		if params == nil {
			continue
		}
		if sportType, _ := params["sport_type"].(string); sportType == "cycling" {
			return true
		}
	}
	return false
}

func systemTagAppliesToModifier(modifier systemModifier, tagName string, cyclingActionContext bool) bool {
	if modifier.TagName != tagName {
		return false
	}
	if modifier.OnlySportType == "cycling" {
		return cyclingActionContext
	}
	return true
}

func modifierForAction(action string) (systemModifier, bool) {
	for _, modifier := range systemModifiers() {
		if modifier.Action == action {
			return modifier, true
		}
	}
	return systemModifier{}, false
}

func modifierForField(field string) (systemModifier, bool) {
	for _, modifier := range systemModifiers() {
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

func validateModifierAgainstExistingTags(modifier systemModifier, value bool, actions []ruleActionMap, tagNameByID map[string]string, cyclingActionContext bool) error {
	for _, action := range actions {
		actionName, _ := action["action"].(string)
		tagName := tagNameForAction(action, tagNameByID)
		if !systemTagAppliesToModifier(modifier, tagName, cyclingActionContext) {
			continue
		}
		if actionName == "add_tag" && !value {
			return fmt.Errorf("%s=false conflicts with add_tag(%s)", modifier.HumanName, modifier.TagName)
		}
		if actionName == "remove_tag" && value {
			return fmt.Errorf("%s=true conflicts with remove_tag(%s)", modifier.HumanName, modifier.TagName)
		}
	}
	return nil
}

func removeSystemTagActions(actions []ruleActionMap, tagName string, tagNameByID map[string]string, actionName string) []ruleActionMap {
	out := make([]ruleActionMap, 0, len(actions))
	for _, action := range actions {
		name, _ := action["action"].(string)
		if name == actionName && tagNameForAction(action, tagNameByID) == tagName {
			continue
		}
		out = append(out, action)
	}
	return out
}
