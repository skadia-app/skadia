package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

var automationTriggerEvents = map[string]struct{}{
	"on_import":              {},
	"on_enrichment_complete": {},
	"on_manual_apply":        {},
	"on_gear_usage_update":   {},
}

var automationActionNames = map[string]struct{}{
	"set_gear":       {},
	"add_tag":        {},
	"remove_tag":     {},
	"set_visibility": {},
	"set_sport_type": {},
	"set_name":       {},
	"set_intensity":  {},
	"set_indoor":     {},
	"set_electric":   {},
	"set_virtual":    {},
	"set_commute":    {},
	"set_field":      {},
	"notify":         {},
	"webhook":        {},
}

var createAutomationRuleInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "priority": {"type": "integer"},
    "trigger_event": {"type": "string", "enum": ["on_import", "on_enrichment_complete", "on_manual_apply", "on_gear_usage_update"]},
    "conditions": {
      "type": "array",
      "items": {"$ref": "#/$defs/condition"}
    },
    "condition_groups": {
      "type": "array",
      "items": {"$ref": "#/$defs/condition_group"}
    },
    "conditions_logic": {"type": "string", "enum": ["and", "or"]},
    "actions": {
      "type": "array",
      "minItems": 1,
      "items": {"$ref": "#/$defs/action"}
    },
    "stop_on_match": {"type": "boolean"},
    "apply_retroactively": {"type": "boolean"},
    "branches": {"type": "array"}
  },
  "required": ["name", "trigger_event", "actions"],
  "additionalProperties": false,
  "$defs": {
    "condition": {
      "type": "object",
      "properties": {
        "field": {"type": "string", "minLength": 1},
        "op": {"type": "string", "enum": ["eq", "neq", "lt", "lte", "gt", "gte", "between", "contains", "in"]},
        "value": {
          "oneOf": [
            {"type": "string"},
            {"type": "number"},
            {"type": "boolean"},
            {"type": "array"}
          ]
        }
      },
      "required": ["field", "op", "value"],
      "additionalProperties": false
    },
    "condition_group": {
      "type": "object",
      "properties": {
        "logic": {"type": "string", "enum": ["and", "or"]},
        "conditions": {
          "type": "array",
          "minItems": 1,
          "items": {"$ref": "#/$defs/condition"}
        }
      },
      "required": ["logic", "conditions"],
      "additionalProperties": false
    },
    "action": {
      "type": "object",
      "properties": {
        "action": {"type": "string", "enum": ["set_gear", "add_tag", "remove_tag", "set_visibility", "set_sport_type", "set_name", "set_intensity", "set_indoor", "set_electric", "set_virtual", "set_commute", "set_field", "notify", "webhook"]},
        "params": {"type": "object"}
      },
      "required": ["action", "params"],
      "additionalProperties": false
    }
  }
}`)

var updateAutomationRuleInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "rule_id": {"type": "string", "minLength": 1},
    "fields": {
      "type": "object",
      "properties": {
        "name": {"type": "string", "minLength": 1},
        "is_active": {"type": "boolean"},
        "priority": {"type": "integer"},
        "trigger_event": {"type": "string", "enum": ["on_import", "on_enrichment_complete", "on_manual_apply", "on_gear_usage_update"]},
        "conditions": {
          "type": "array",
          "items": {"$ref": "#/$defs/condition"}
        },
        "condition_groups": {
          "type": "array",
          "items": {"$ref": "#/$defs/condition_group"}
        },
        "conditions_logic": {"type": "string", "enum": ["and", "or"]},
        "actions": {
          "type": "array",
          "minItems": 1,
          "items": {"$ref": "#/$defs/action"}
        },
        "stop_on_match": {"type": "boolean"},
        "apply_retroactively": {"type": "boolean"},
        "branches": {"type": "array"}
      },
      "additionalProperties": false,
      "minProperties": 1
    }
  },
  "required": ["rule_id", "fields"],
  "additionalProperties": false,
  "$defs": {
    "condition": {
      "type": "object",
      "properties": {
        "field": {"type": "string", "minLength": 1},
        "op": {"type": "string", "enum": ["eq", "neq", "lt", "lte", "gt", "gte", "between", "contains", "in"]},
        "value": {
          "oneOf": [
            {"type": "string"},
            {"type": "number"},
            {"type": "boolean"},
            {"type": "array"}
          ]
        }
      },
      "required": ["field", "op", "value"],
      "additionalProperties": false
    },
    "condition_group": {
      "type": "object",
      "properties": {
        "logic": {"type": "string", "enum": ["and", "or"]},
        "conditions": {
          "type": "array",
          "minItems": 1,
          "items": {"$ref": "#/$defs/condition"}
        }
      },
      "required": ["logic", "conditions"],
      "additionalProperties": false
    },
    "action": {
      "type": "object",
      "properties": {
        "action": {"type": "string", "enum": ["set_gear", "add_tag", "remove_tag", "set_visibility", "set_sport_type", "set_name", "set_intensity", "set_indoor", "set_electric", "set_virtual", "set_commute", "set_field", "notify", "webhook"]},
        "params": {"type": "object"}
      },
      "required": ["action", "params"],
      "additionalProperties": false
    }
  }
}`)

var ruleIDInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "rule_id": {"type": "string", "minLength": 1}
  },
  "required": ["rule_id"],
  "additionalProperties": false
}`)

func WriteRuleTools() []*Tool {
	return []*Tool{
		CreateAutomationRuleTool(),
		UpdateAutomationRuleTool(),
		DeleteAutomationRuleTool(),
		EnableAutomationRuleTool(),
		DisableAutomationRuleTool(),
	}
}

func RegisterRuleWriteTools(reg *Registry) {
	for _, tool := range WriteRuleTools() {
		reg.Register(tool)
	}
}

func CreateAutomationRuleTool() *Tool {
	return &Tool{
		Name:                 "create_automation_rule",
		Description:          "Create an automation rule after user confirmation.",
		InputSchema:          createAutomationRuleInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeCreateAutomationRule,
		ExecuteWrite:         executeCreateAutomationRule,
	}
}

func UpdateAutomationRuleTool() *Tool {
	return &Tool{
		Name:                 "update_automation_rule",
		Description:          "Update an automation rule after user confirmation.",
		InputSchema:          updateAutomationRuleInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateAutomationRule,
		ExecuteWrite:         executeUpdateAutomationRule,
	}
}

func DeleteAutomationRuleTool() *Tool {
	return &Tool{
		Name:                 "delete_automation_rule",
		Description:          "Delete an automation rule after user confirmation.",
		InputSchema:          ruleIDInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeDeleteAutomationRule,
		ExecuteWrite:         executeDeleteAutomationRule,
	}
}

func EnableAutomationRuleTool() *Tool {
	return toggleAutomationRuleTool("enable_automation_rule", "Enable an automation rule after user confirmation.", true)
}

func DisableAutomationRuleTool() *Tool {
	return toggleAutomationRuleTool("disable_automation_rule", "Disable an automation rule after user confirmation.", false)
}

func toggleAutomationRuleTool(name, description string, active bool) *Tool {
	return &Tool{
		Name:                 name,
		Description:          description,
		InputSchema:          ruleIDInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
			return summarizeToggleAutomationRule(ctx, deps, userID, args, active)
		},
		ExecuteWrite: func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return executeToggleAutomationRule(ctx, deps, userID, args, active)
		},
	}
}

type createAutomationRuleArgs model.AutomationRuleCreate

type updateAutomationRuleArgs struct {
	RuleID string `json:"rule_id"`
	Fields struct {
		Name             *string          `json:"name"`
		IsActive         *bool            `json:"is_active"`
		Priority         *int             `json:"priority"`
		Conditions       *json.RawMessage `json:"conditions,omitempty"`
		ConditionsLogic  *string          `json:"conditions_logic,omitempty"`
		ConditionGroups  *json.RawMessage `json:"condition_groups,omitempty"`
		Actions          *json.RawMessage `json:"actions,omitempty"`
		TriggerEvent     *string          `json:"trigger_event"`
		StopOnMatch      *bool            `json:"stop_on_match"`
		ApplyRetroactive *bool            `json:"apply_retroactively"`
		Branches         *json.RawMessage `json:"branches,omitempty"`
	} `json:"fields"`
}

type automationRuleIDArgs struct {
	RuleID string `json:"rule_id"`
}

func executeCreateAutomationRule(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	create, err := decodeCreateAutomationRuleArgs(ctx, deps.Deps, userID, args)
	if err != nil {
		return nil, err
	}
	rule, err := deps.DB.CreateRuleTx(ctx, deps.Tx, userID, create)
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": rule.ID, "rule_id": rule.ID, "status": "ok"}, nil
}

func executeUpdateAutomationRule(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	in, update, err := decodeUpdateAutomationRuleArgs(ctx, deps.Deps, userID, args)
	if err != nil {
		return nil, err
	}
	rule, err := deps.DB.UpdateRuleTx(ctx, deps.Tx, in.RuleID, userID, update)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, fmt.Errorf("FORBIDDEN")
	}
	return map[string]any{"affected_id": rule.ID, "rule_id": rule.ID, "status": "ok"}, nil
}

func executeDeleteAutomationRule(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	in, err := decodeAutomationRuleIDArgs(args)
	if err != nil {
		return nil, err
	}
	if err := deps.DB.DeleteRuleTx(ctx, deps.Tx, in.RuleID, userID); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.RuleID, "rule_id": in.RuleID, "status": "ok"}, nil
}

func executeToggleAutomationRule(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage, active bool) (any, error) {
	in, err := decodeAutomationRuleIDArgs(args)
	if err != nil {
		return nil, err
	}
	rule, err := deps.DB.UpdateRuleTx(ctx, deps.Tx, in.RuleID, userID, &model.AutomationRuleUpdate{IsActive: &active})
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, fmt.Errorf("FORBIDDEN")
	}
	return map[string]any{"affected_id": rule.ID, "rule_id": rule.ID, "status": "ok"}, nil
}

func summarizeCreateAutomationRule(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	create, err := decodeCreateAutomationRuleArgs(ctx, deps, userID, args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Neue Automation-Regel '%s' anlegen (Trigger: %s)", create.Name, create.TriggerEvent), nil
}

func summarizeUpdateAutomationRule(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	in, _, err := decodeUpdateAutomationRuleArgs(ctx, deps, userID, args)
	if err != nil {
		return "", err
	}
	rule, err := ownedAutomationRule(ctx, deps, userID, in.RuleID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Regel '%s' aktualisieren: %s", rule.Name, strings.Join(updateAutomationRuleFieldNames(in), ", ")), nil
}

func summarizeDeleteAutomationRule(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	in, err := decodeAutomationRuleIDArgs(args)
	if err != nil {
		return "", err
	}
	rule, err := ownedAutomationRule(ctx, deps, userID, in.RuleID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Regel '%s' löschen", rule.Name), nil
}

func summarizeToggleAutomationRule(ctx context.Context, deps *Deps, userID string, args json.RawMessage, active bool) (string, error) {
	in, err := decodeAutomationRuleIDArgs(args)
	if err != nil {
		return "", err
	}
	rule, err := ownedAutomationRule(ctx, deps, userID, in.RuleID)
	if err != nil {
		return "", err
	}
	action := "deaktivieren"
	if active {
		action = "aktivieren"
	}
	return fmt.Sprintf("Regel '%s' %s", rule.Name, action), nil
}

func decodeCreateAutomationRuleArgs(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (*model.AutomationRuleCreate, error) {
	var in createAutomationRuleArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	create := model.AutomationRuleCreate(in)
	create.Name = strings.TrimSpace(create.Name)
	if create.Name == "" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	if err := validateAutomationTriggerEvent(create.TriggerEvent); err != nil {
		return nil, err
	}
	if err := validateAutomationRuleMinimum(create.Conditions, create.ConditionGroups, create.Actions, create.Branches); err != nil {
		return nil, err
	}
	if err := normalizeAutomationRuleCreate(ctx, deps, userID, &create); err != nil {
		return nil, err
	}
	return &create, nil
}

func decodeUpdateAutomationRuleArgs(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (updateAutomationRuleArgs, *model.AutomationRuleUpdate, error) {
	var in updateAutomationRuleArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return in, nil, err
	}
	if strings.TrimSpace(in.RuleID) == "" {
		return in, nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	if in.Fields.Name != nil {
		name := strings.TrimSpace(*in.Fields.Name)
		if name == "" {
			return in, nil, fmt.Errorf("INVALID_ARGUMENT")
		}
		in.Fields.Name = &name
	}
	if in.Fields.TriggerEvent != nil {
		if err := validateAutomationTriggerEvent(*in.Fields.TriggerEvent); err != nil {
			return in, nil, err
		}
	}
	update := &model.AutomationRuleUpdate{
		Name:             in.Fields.Name,
		IsActive:         in.Fields.IsActive,
		Priority:         in.Fields.Priority,
		Conditions:       in.Fields.Conditions,
		ConditionsLogic:  in.Fields.ConditionsLogic,
		ConditionGroups:  in.Fields.ConditionGroups,
		Actions:          in.Fields.Actions,
		TriggerEvent:     in.Fields.TriggerEvent,
		StopOnMatch:      in.Fields.StopOnMatch,
		ApplyRetroactive: in.Fields.ApplyRetroactive,
	}
	if in.Fields.Branches != nil {
		update.Branches = *in.Fields.Branches
	}
	if err := normalizeAutomationRuleUpdate(ctx, deps, userID, update); err != nil {
		return in, nil, err
	}
	return in, update, nil
}

func decodeAutomationRuleIDArgs(args json.RawMessage) (automationRuleIDArgs, error) {
	var in automationRuleIDArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return in, err
	}
	if strings.TrimSpace(in.RuleID) == "" {
		return in, fmt.Errorf("INVALID_ARGUMENT")
	}
	return in, nil
}

func normalizeAutomationRuleCreate(ctx context.Context, deps *Deps, userID string, create *model.AutomationRuleCreate) error {
	rewritten, _, err := rules.NormalizeAndValidateSportTaxonomyRuleJSON(rules.SportTaxonomyRuleJSON{
		Conditions:      create.Conditions,
		ConditionGroups: create.ConditionGroups,
		Branches:        create.Branches,
		Actions:         create.Actions,
	})
	if err != nil {
		return err
	}
	create.Conditions = rewritten.Conditions
	create.ConditionGroups = rewritten.ConditionGroups
	create.Branches = rewritten.Branches
	create.Actions = rewritten.Actions
	if err := rules.ValidateGearConditionJSON(create.Conditions, create.ConditionGroups, create.Branches); err != nil {
		return err
	}
	if err := validateKnownAutomationActions(create.Actions, create.Branches); err != nil {
		return err
	}
	tagNames, err := automationRuleTagNames(ctx, deps, userID)
	if err != nil {
		return err
	}
	if create.Actions, err = rules.NormalizeRuleActions(create.Actions, tagNames); err != nil {
		return err
	}
	if create.Branches, err = rules.NormalizeRuleBranches(create.Branches, tagNames); err != nil {
		return err
	}
	return nil
}

func normalizeAutomationRuleUpdate(ctx context.Context, deps *Deps, userID string, update *model.AutomationRuleUpdate) error {
	input := rules.SportTaxonomyRuleJSON{Branches: update.Branches}
	if update.Conditions != nil {
		input.Conditions = *update.Conditions
	}
	if update.ConditionGroups != nil {
		input.ConditionGroups = *update.ConditionGroups
	}
	if update.Actions != nil {
		input.Actions = *update.Actions
	}
	rewritten, _, err := rules.NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err != nil {
		return err
	}
	if update.Conditions != nil {
		*update.Conditions = rewritten.Conditions
	}
	if update.ConditionGroups != nil {
		*update.ConditionGroups = rewritten.ConditionGroups
	}
	if update.Actions != nil {
		*update.Actions = rewritten.Actions
	}
	update.Branches = rewritten.Branches

	var conditions, conditionGroups json.RawMessage
	if update.Conditions != nil {
		conditions = *update.Conditions
	}
	if update.ConditionGroups != nil {
		conditionGroups = *update.ConditionGroups
	}
	if err := rules.ValidateGearConditionJSON(conditions, conditionGroups, update.Branches); err != nil {
		return err
	}
	var actions json.RawMessage
	if update.Actions != nil {
		actions = *update.Actions
	}
	if err := validateKnownAutomationActions(actions, update.Branches); err != nil {
		return err
	}
	tagNames, err := automationRuleTagNames(ctx, deps, userID)
	if err != nil {
		return err
	}
	if update.Actions != nil {
		normalized, err := rules.NormalizeRuleActions(*update.Actions, tagNames)
		if err != nil {
			return err
		}
		update.Actions = &normalized
	}
	if len(update.Branches) > 0 && string(update.Branches) != "null" {
		if update.Branches, err = rules.NormalizeRuleBranches(update.Branches, tagNames); err != nil {
			return err
		}
	}
	return nil
}

func validateAutomationRuleMinimum(conditions, conditionGroups, actions, branches json.RawMessage) error {
	hasBranches := len(branches) > 0 && string(branches) != "null"
	if hasBranches {
		return nil
	}
	if len(conditions) == 0 && len(conditionGroups) == 0 {
		return fmt.Errorf("conditions or condition_groups are required")
	}
	if len(actions) == 0 {
		return fmt.Errorf("actions are required")
	}
	return nil
}

func validateAutomationTriggerEvent(event string) error {
	if _, ok := automationTriggerEvents[event]; !ok {
		return fmt.Errorf("invalid trigger_event %q", event)
	}
	return nil
}

func validateKnownAutomationActions(actions, branches json.RawMessage) error {
	if err := validateKnownAutomationActionsArray(actions, "actions"); err != nil {
		return err
	}
	if len(branches) == 0 || string(branches) == "null" {
		return nil
	}
	var decoded []struct {
		Actions json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(branches, &decoded); err != nil {
		return fmt.Errorf("decode branches: %w", err)
	}
	for i, branch := range decoded {
		if err := validateKnownAutomationActionsArray(branch.Actions, fmt.Sprintf("branches[%d].actions", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateKnownAutomationActionsArray(raw json.RawMessage, surface string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var decoded []struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("%s: decode actions: %w", surface, err)
	}
	for i, action := range decoded {
		if _, ok := automationActionNames[action.Action]; !ok {
			return fmt.Errorf("%s[%d]: unknown action %q", surface, i, action.Action)
		}
	}
	return nil
}

func automationRuleTagNames(ctx context.Context, deps *Deps, userID string) (map[string]string, error) {
	tags, err := deps.DB.ListTags(ctx, userID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(tags))
	for _, tag := range tags {
		names[tag.ID] = tag.Name
	}
	return names, nil
}

func ownedAutomationRule(ctx context.Context, deps *Deps, userID, ruleID string) (*model.AutomationRule, error) {
	rule, err := deps.DB.GetRule(ctx, ruleID, userID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, fmt.Errorf("FORBIDDEN")
	}
	return rule, nil
}

func updateAutomationRuleFieldNames(in updateAutomationRuleArgs) []string {
	var fields []string
	if in.Fields.Name != nil {
		fields = append(fields, "name")
	}
	if in.Fields.IsActive != nil {
		fields = append(fields, "is_active")
	}
	if in.Fields.Priority != nil {
		fields = append(fields, "priority")
	}
	if in.Fields.TriggerEvent != nil {
		fields = append(fields, "trigger_event")
	}
	if in.Fields.Conditions != nil {
		fields = append(fields, "conditions")
	}
	if in.Fields.ConditionGroups != nil {
		fields = append(fields, "condition_groups")
	}
	if in.Fields.ConditionsLogic != nil {
		fields = append(fields, "conditions_logic")
	}
	if in.Fields.Actions != nil {
		fields = append(fields, "actions")
	}
	if in.Fields.StopOnMatch != nil {
		fields = append(fields, "stop_on_match")
	}
	if in.Fields.ApplyRetroactive != nil {
		fields = append(fields, "apply_retroactively")
	}
	if in.Fields.Branches != nil {
		fields = append(fields, "branches")
	}
	return fields
}
