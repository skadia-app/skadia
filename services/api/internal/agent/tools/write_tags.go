package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

var tagHexColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var createTagInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "color": {"type": "string"},
    "text_color": {"type": "string"},
    "description": {"type": "string"},
    "conditions": {"type": "array"},
    "condition_groups": {"type": "array"},
    "conditions_logic": {"type": "string", "enum": ["and", "or"]},
    "lat": {"type": "number"},
    "lng": {"type": "number"}
  },
  "required": ["name"],
  "additionalProperties": false
}`)

var updateTagInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "tag_id": {"type": "string", "minLength": 1},
    "fields": {
      "type": "object",
      "properties": {
        "name": {"type": "string", "minLength": 1},
        "color": {"type": "string"},
        "text_color": {"type": "string"},
        "description": {"type": "string"},
        "conditions": {"type": "array"},
        "condition_groups": {"type": "array"},
        "conditions_logic": {"type": "string", "enum": ["and", "or"]},
        "lat": {"type": ["number", "null"]},
        "lng": {"type": ["number", "null"]}
      },
      "additionalProperties": false,
      "minProperties": 1
    }
  },
  "required": ["tag_id", "fields"],
  "additionalProperties": false
}`)

var deleteTagInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "tag_id": {"type": "string", "minLength": 1}
  },
  "required": ["tag_id"],
  "additionalProperties": false
}`)

func WriteTagTools() []*Tool {
	return []*Tool{
		CreateTagTool(),
		UpdateTagTool(),
		DeleteTagTool(),
	}
}

func RegisterTagTools(reg *Registry) {
	for _, tool := range WriteTagTools() {
		reg.Register(tool)
	}
}

func CreateTagTool() *Tool {
	return &Tool{
		Name:                 "create_tag",
		Description:          "Create a manual or automation-backed tag after user confirmation.",
		InputSchema:          createTagInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeCreateTag,
		ExecuteWrite:         executeCreateTag,
	}
}

func UpdateTagTool() *Tool {
	return &Tool{
		Name:                 "update_tag",
		Description:          "Update a user-owned tag after user confirmation.",
		InputSchema:          updateTagInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateTag,
		ExecuteWrite:         executeUpdateTag,
	}
}

func DeleteTagTool() *Tool {
	return &Tool{
		Name:                 "delete_tag",
		Description:          "Delete a user-owned tag after user confirmation.",
		InputSchema:          deleteTagInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeDeleteTag,
		ExecuteWrite:         executeDeleteTag,
	}
}

type createTagArgs struct {
	Name            string          `json:"name"`
	Color           string          `json:"color"`
	TextColor       string          `json:"text_color"`
	Description     *string         `json:"description"`
	Conditions      json.RawMessage `json:"conditions,omitempty"`
	ConditionGroups json.RawMessage `json:"condition_groups,omitempty"`
	ConditionsLogic string          `json:"conditions_logic,omitempty"`
	Lat             *float64        `json:"lat"`
	Lng             *float64        `json:"lng"`
}

type updateTagArgs struct {
	TagID  string `json:"tag_id"`
	Fields struct {
		Name            *string          `json:"name"`
		Color           *string          `json:"color"`
		TextColor       *string          `json:"text_color"`
		Description     *string          `json:"description"`
		Conditions      *json.RawMessage `json:"conditions,omitempty"`
		ConditionGroups *json.RawMessage `json:"condition_groups,omitempty"`
		ConditionsLogic *string          `json:"conditions_logic,omitempty"`
		Lat             *json.RawMessage `json:"lat"`
		Lng             *json.RawMessage `json:"lng"`
	} `json:"fields"`
}

type deleteTagArgs struct {
	TagID string `json:"tag_id"`
}

func executeCreateTag(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	in, err := decodeCreateTagArgs(args)
	if err != nil {
		return nil, err
	}
	tag, err := deps.DB.CreateTagTx(ctx, deps.Tx, userID, strings.TrimSpace(in.Name), defaultTagColor(in.Color), defaultTagTextColor(in.TextColor), tagHasAutomation(in.Conditions, in.ConditionGroups), rawBytes(in.Conditions), rawBytes(in.ConditionGroups), in.ConditionsLogic, in.Description, in.Lat, in.Lng)
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": tag.ID, "tag_id": tag.ID, "status": "ok"}, nil
}

func executeUpdateTag(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	in, err := decodeUpdateTagArgs(args)
	if err != nil {
		return nil, err
	}
	lat, lng, locationPresent, err := updateTagLocation(in)
	if err != nil {
		return nil, err
	}
	conditions, conditionGroups := rawPointerBytes(in.Fields.Conditions), rawPointerBytes(in.Fields.ConditionGroups)
	isAuto := updateTagIsAuto(in)
	tag, err := deps.DB.UpdateTagTx(ctx, deps.Tx, in.TagID, userID, trimmedStringValue(in.Fields.Name), tagColorValue(in.Fields.Color), tagColorValue(in.Fields.TextColor), isAuto, conditions, conditionGroups, in.Fields.ConditionsLogic, in.Fields.Description, lat, lng, locationPresent)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, fmt.Errorf("FORBIDDEN")
	}
	return map[string]any{"affected_id": tag.ID, "tag_id": tag.ID, "status": "ok"}, nil
}

func executeDeleteTag(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	var in deleteTagArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := deps.DB.DeleteTagTx(ctx, deps.Tx, in.TagID, userID); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.TagID, "tag_id": in.TagID, "status": "ok"}, nil
}

func summarizeCreateTag(_ context.Context, _ *Deps, _ string, args json.RawMessage) (string, error) {
	in, err := decodeCreateTagArgs(args)
	if err != nil {
		return "", err
	}
	suffix := ""
	if tagHasAutomation(in.Conditions, in.ConditionGroups) {
		suffix = " (Auto-Regel)"
	}
	return fmt.Sprintf("Neuen Tag '%s' anlegen%s", strings.TrimSpace(in.Name), suffix), nil
}

func summarizeUpdateTag(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	in, err := decodeUpdateTagArgs(args)
	if err != nil {
		return "", err
	}
	tag, err := deps.DB.GetTagByID(ctx, in.TagID, userID)
	if err != nil {
		return "", err
	}
	if tag == nil {
		return "", fmt.Errorf("FORBIDDEN")
	}
	return fmt.Sprintf("Tag '%s' aktualisieren: %s", tag.Name, strings.Join(updateTagFieldNames(in), ", ")), nil
}

func summarizeDeleteTag(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in deleteTagArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	tag, err := deps.DB.GetTagByID(ctx, in.TagID, userID)
	if err != nil {
		return "", err
	}
	if tag == nil {
		return "", fmt.Errorf("FORBIDDEN")
	}
	impact, err := deps.DB.GetTagDeleteImpact(ctx, userID, in.TagID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return "", fmt.Errorf("FORBIDDEN")
		}
		return "", err
	}
	zoneFlag := "keine Zone"
	if impact.HasZone {
		zoneFlag = "eine Zone"
	}
	return fmt.Sprintf("Tag '%s' löschen - entfernt %d Aktivitäts-Zuweisungen, %s, %d Routen-Links und %d Entfernungs-Historien", tag.Name, impact.ActivityAssignments, zoneFlag, impact.RouteLinks, impact.RemovalHistory), nil
}

func decodeCreateTagArgs(args json.RawMessage) (createTagArgs, error) {
	var in createTagArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return in, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, fmt.Errorf("INVALID_ARGUMENT")
	}
	if err := validateTagColor("color", defaultTagColor(in.Color)); err != nil {
		return in, err
	}
	if err := validateTagColor("text_color", defaultTagTextColor(in.TextColor)); err != nil {
		return in, err
	}
	if (in.Lat == nil) != (in.Lng == nil) {
		return in, fmt.Errorf("lat and lng must be provided together")
	}
	if err := rules.ValidateGearConditionJSON(in.Conditions, in.ConditionGroups, nil); err != nil {
		return in, err
	}
	return in, nil
}

func decodeUpdateTagArgs(args json.RawMessage) (updateTagArgs, error) {
	var in updateTagArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return in, err
	}
	if strings.TrimSpace(in.TagID) == "" {
		return in, fmt.Errorf("INVALID_ARGUMENT")
	}
	if in.Fields.Name != nil && strings.TrimSpace(*in.Fields.Name) == "" {
		return in, fmt.Errorf("INVALID_ARGUMENT")
	}
	if in.Fields.Color != nil {
		if err := validateTagColor("color", *in.Fields.Color); err != nil {
			return in, err
		}
	}
	if in.Fields.TextColor != nil {
		if err := validateTagColor("text_color", *in.Fields.TextColor); err != nil {
			return in, err
		}
	}
	var conditions, conditionGroups json.RawMessage
	if in.Fields.Conditions != nil {
		conditions = *in.Fields.Conditions
	}
	if in.Fields.ConditionGroups != nil {
		conditionGroups = *in.Fields.ConditionGroups
	}
	if err := rules.ValidateGearConditionJSON(conditions, conditionGroups, nil); err != nil {
		return in, err
	}
	if _, _, _, err := updateTagLocation(in); err != nil {
		return in, err
	}
	return in, nil
}

func updateTagLocation(in updateTagArgs) (*float64, *float64, bool, error) {
	if in.Fields.Lat == nil && in.Fields.Lng == nil {
		return nil, nil, false, nil
	}
	if in.Fields.Lat == nil || in.Fields.Lng == nil {
		return nil, nil, false, fmt.Errorf("lat and lng must be provided together")
	}
	if string(*in.Fields.Lat) == "null" && string(*in.Fields.Lng) == "null" {
		return nil, nil, true, nil
	}
	if string(*in.Fields.Lat) == "null" || string(*in.Fields.Lng) == "null" {
		return nil, nil, false, fmt.Errorf("lat and lng must be both numbers or both null")
	}
	var lat, lng float64
	if err := json.Unmarshal(*in.Fields.Lat, &lat); err != nil {
		return nil, nil, false, fmt.Errorf("decode lat: %w", err)
	}
	if err := json.Unmarshal(*in.Fields.Lng, &lng); err != nil {
		return nil, nil, false, fmt.Errorf("decode lng: %w", err)
	}
	return &lat, &lng, true, nil
}

func updateTagIsAuto(in updateTagArgs) *bool {
	if in.Fields.Conditions == nil && in.Fields.ConditionGroups == nil {
		return nil
	}
	v := false
	if in.Fields.Conditions != nil {
		v = v || len(*in.Fields.Conditions) > 2
	}
	if in.Fields.ConditionGroups != nil {
		v = v || len(*in.Fields.ConditionGroups) > 2
	}
	return &v
}

func updateTagFieldNames(in updateTagArgs) []string {
	var fields []string
	if in.Fields.Name != nil {
		fields = append(fields, "name")
	}
	if in.Fields.Color != nil {
		fields = append(fields, "color")
	}
	if in.Fields.TextColor != nil {
		fields = append(fields, "text_color")
	}
	if in.Fields.Description != nil {
		fields = append(fields, "description")
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
	if in.Fields.Lat != nil || in.Fields.Lng != nil {
		fields = append(fields, "location")
	}
	return fields
}

func tagHasAutomation(conditions, conditionGroups json.RawMessage) bool {
	return len(conditions) > 2 || len(conditionGroups) > 2
}

func rawBytes(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func rawPointerBytes(raw *json.RawMessage) *[]byte {
	if raw == nil {
		return nil
	}
	b := []byte(*raw)
	return &b
}

func defaultTagColor(v string) string {
	if strings.TrimSpace(v) == "" {
		return "#6366F1"
	}
	return strings.TrimSpace(v)
}

func defaultTagTextColor(v string) string {
	if strings.TrimSpace(v) == "" {
		return "#FFFFFF"
	}
	return strings.TrimSpace(v)
}

func validateTagColor(field, v string) error {
	if !tagHexColorRE.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("%s must be #RRGGBB", field)
	}
	return nil
}

func trimmedStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func tagColorValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
