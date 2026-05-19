package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

var createGearInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "gear_type": {"type": "string", "minLength": 1},
    "brand": {"type": "string"},
    "model_name": {"type": "string"},
    "scope": {"type": "string"},
    "applicable_sport_predicates": {"type": "array"},
    "attributes": {"type": "object"},
    "modifiers": {"type": "object"},
    "components": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "minLength": 1},
          "type": {"type": "string", "minLength": 1},
          "brand": {"type": "string"},
          "model": {"type": "string"},
          "purchase_price": {"type": "number"},
          "purchase_date": {"type": "string"},
          "weight_g": {"type": "integer"},
          "serial_number": {"type": "string"},
          "notes": {"type": "string"},
          "preset_gear_type_key": {"type": "string"}
        },
        "required": ["name", "type"],
        "additionalProperties": false
      }
    }
  },
  "required": ["name", "gear_type"],
  "additionalProperties": false
}`)

var updateGearMetadataInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {"type": "string", "minLength": 1},
    "fields": {
      "type": "object",
      "properties": {
        "name": {"type": "string", "minLength": 1},
        "brand": {"type": "string"},
        "model_name": {"type": "string"},
        "notes": {"type": "string"},
        "attributes": {"type": "object"}
      },
      "additionalProperties": false,
      "minProperties": 1
    }
  },
  "required": ["gear_id", "fields"],
  "additionalProperties": false
}`)

var retireGearInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {"type": "string", "minLength": 1},
    "retirement_date_iso": {"type": "string", "minLength": 1}
  },
  "required": ["gear_id"],
  "additionalProperties": false
}`)

var setActivityGearInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "minLength": 1},
    "gear_id": {"type": "string", "minLength": 1},
    "role_key": {"type": "string", "enum": ["primary"]}
  },
  "required": ["activity_id", "gear_id", "role_key"],
  "additionalProperties": false
}`)

func WriteGearTools() []*Tool {
	return []*Tool{
		CreateGearTool(),
		UpdateGearMetadataTool(),
		RetireGearTool(),
		SetActivityGearTool(),
		AddGearComponentTool(),
		UpdateGearComponentStatusTool(),
		RecordGearServiceEventTool(),
		CreateChainRotationTool(),
		UpdateGearDefaultPredicatesTool(),
		BatchSetActivityGearTool(),
	}
}

func CreateGearTool() *Tool {
	return &Tool{
		Name:                 "create_gear",
		Description:          "Create a new top-level gear item such as a bike, shoes, watch, or sensor after user confirmation. Do not use for components such as chains, tires, cassettes, brake pads, forks, or shocks; attach those as components.",
		InputSchema:          createGearInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeCreateGear,
		ExecuteWrite:         executeCreateGear,
	}
}

func UpdateGearMetadataTool() *Tool {
	return &Tool{
		Name:                 "update_gear_metadata",
		Description:          "Update safe metadata fields on an existing gear item after user confirmation.",
		InputSchema:          updateGearMetadataInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateGearMetadata,
		ExecuteWrite:         executeUpdateGearMetadata,
	}
}

func RetireGearTool() *Tool {
	return &Tool{
		Name:                 "retire_gear",
		Description:          "Mark an existing gear item as retired after user confirmation.",
		InputSchema:          retireGearInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeRetireGear,
		ExecuteWrite:         executeRetireGear,
	}
}

func SetActivityGearTool() *Tool {
	return &Tool{
		Name:                 "set_activity_gear",
		Description:          "Assign an owned gear item as the primary gear for an owned activity after user confirmation.",
		InputSchema:          setActivityGearInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeSetActivityGear,
		ExecuteWrite:         executeSetActivityGear,
	}
}

type createGearArgs struct {
	Name                      string                          `json:"name"`
	GearType                  string                          `json:"gear_type"`
	Brand                     *string                         `json:"brand"`
	ModelName                 *string                         `json:"model_name"`
	Scope                     string                          `json:"scope"`
	ApplicableSportPredicates *[]gearpkg.GearDefaultPredicate `json:"applicable_sport_predicates"`
	Attributes                map[string]any                  `json:"attributes"`
	Modifiers                 map[string]any                  `json:"modifiers"`
	Components                []model.GearComponentInput      `json:"components"`
}

type updateGearMetadataArgs struct {
	GearID string `json:"gear_id"`
	Fields struct {
		Name       *string        `json:"name"`
		Brand      *string        `json:"brand"`
		ModelName  *string        `json:"model_name"`
		Notes      *string        `json:"notes"`
		Attributes map[string]any `json:"attributes"`
	} `json:"fields"`
}

type retireGearArgs struct {
	GearID            string  `json:"gear_id"`
	RetirementDateISO *string `json:"retirement_date_iso"`
}

type setActivityGearArgs struct {
	ActivityID string `json:"activity_id"`
	GearID     string `json:"gear_id"`
	RoleKey    string `json:"role_key"`
}

func executeCreateGear(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("create_gear", createGearInputSchema, args); err != nil {
		return nil, err
	}
	var in createGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.GearType) == "" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	if err := validateAgentGearType(in.GearType); err != nil {
		return nil, err
	}
	components, err := cleanInitialGearComponents(in.Components)
	if err != nil {
		return nil, err
	}
	g, err := deps.DB.CreateGearTx(ctx, deps.Tx, userID, &model.GearCreate{
		Name:                      strings.TrimSpace(in.Name),
		Type:                      strings.TrimSpace(in.GearType),
		Scope:                     strings.TrimSpace(in.Scope),
		Brand:                     in.Brand,
		Model:                     in.ModelName,
		ApplicableSportPredicates: in.ApplicableSportPredicates,
		Attributes:                in.Attributes,
		Modifiers:                 in.Modifiers,
		Components:                components,
	})
	if err != nil {
		return nil, err
	}
	componentResults := make([]map[string]any, 0, len(g.Components))
	for _, component := range g.Components {
		componentResults = append(componentResults, map[string]any{
			"component_id": component.ID,
			"name":         component.Name,
			"type":         component.Type,
		})
	}
	return map[string]any{"affected_id": g.ID, "gear_id": g.ID, "components": componentResults, "status": "ok"}, nil
}

func executeUpdateGearMetadata(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("update_gear_metadata", updateGearMetadataInputSchema, args); err != nil {
		return nil, err
	}
	var in updateGearMetadataArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	update := &model.GearUpdate{
		Name:       trimPtr(in.Fields.Name),
		Brand:      trimPtr(in.Fields.Brand),
		Model:      trimPtr(in.Fields.ModelName),
		Notes:      in.Fields.Notes,
		Attributes: in.Fields.Attributes,
	}
	g, err := deps.DB.UpdateGearTx(ctx, deps.Tx, in.GearID, update)
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": g.ID, "gear_id": g.ID, "status": "ok"}, nil
}

func executeRetireGear(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("retire_gear", retireGearInputSchema, args); err != nil {
		return nil, err
	}
	var in retireGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	if in.RetirementDateISO != nil {
		if _, err := deps.DB.UpdateGearTx(ctx, deps.Tx, in.GearID, &model.GearUpdate{RetiredAt: trimPtr(in.RetirementDateISO)}); err != nil {
			return nil, err
		}
	}
	if err := deps.DB.UpdateGearStatusTx(ctx, deps.Tx, in.GearID, userID, "retired"); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.GearID, "gear_id": in.GearID, "status": "ok"}, nil
}

func executeSetActivityGear(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("set_activity_gear", setActivityGearInputSchema, args); err != nil {
		return nil, err
	}
	var in setActivityGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedActivity(ctx, deps, in.ActivityID, userID); err != nil {
		return nil, err
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	if err := database.SetPrimaryGearForActivityTx(ctx, deps.Tx, in.ActivityID, in.GearID, "user"); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.ActivityID, "activity_id": in.ActivityID, "gear_id": in.GearID, "status": "ok"}, nil
}

func summarizeCreateGear(_ context.Context, _ *Deps, _ string, args json.RawMessage) (string, error) {
	var in createGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	if err := validateAgentGearType(in.GearType); err != nil {
		return "", err
	}
	if len(in.Components) > 0 {
		return fmt.Sprintf("Neues Gear anlegen: %s (%s), Komponenten: %d", in.Name, in.GearType, len(in.Components)), nil
	}
	return fmt.Sprintf("Neues Gear anlegen: %s (%s)", in.Name, in.GearType), nil
}

func validateAgentGearType(typeKey string) error {
	typeKey = strings.TrimSpace(typeKey)
	if _, ok := gearpkg.LookupGearType(typeKey); ok {
		return nil
	}
	if _, ok := gearpkg.LookupComponentType(typeKey); ok {
		return fmt.Errorf("gear.component_type_requires_component_tool")
	}
	return fmt.Errorf("gear.unknown_gear_type")
}

func cleanInitialGearComponents(in []model.GearComponentInput) ([]model.GearComponentInput, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]model.GearComponentInput, 0, len(in))
	for _, component := range in {
		component.ID = ""
		component.Name = strings.TrimSpace(component.Name)
		componentType, err := normalizeAgentComponentType(component.Type)
		if err != nil {
			return nil, err
		}
		component.Type = componentType
		if component.Name == "" || component.Type == "" {
			return nil, fmt.Errorf("INVALID_ARGUMENT")
		}
		out = append(out, component)
	}
	return out, nil
}

func normalizeAgentComponentType(typeKey string) (string, error) {
	typeKey = strings.TrimSpace(typeKey)
	if typeKey == "" {
		return "", fmt.Errorf("INVALID_ARGUMENT")
	}
	for _, componentType := range gearpkg.Schema.ComponentTypes {
		tail := componentType.Key
		if idx := strings.LastIndex(tail, "."); idx >= 0 && idx+1 < len(tail) {
			tail = tail[idx+1:]
		}
		if strings.EqualFold(typeKey, componentType.Key) ||
			strings.EqualFold(typeKey, tail) ||
			strings.EqualFold(typeKey, componentType.LabelDE) {
			return tail, nil
		}
	}
	return "", fmt.Errorf("gear.unknown_component_type")
}

func summarizeUpdateGearMetadata(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in updateGearMetadataArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Gear %s aktualisieren: %s", gearName, strings.Join(updateGearFieldNames(in), ", ")), nil
}

func summarizeRetireGear(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in retireGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Gear %s in den Ruhestand versetzen", gearName), nil
}

func summarizeSetActivityGear(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in setActivityGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	activityName, err := ownedActivityName(ctx, deps, in.ActivityID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Gear %s der Aktivität %s als primary zuweisen", gearName, activityName), nil
}

func requireOwnedGear(ctx context.Context, deps *WriteExecutionDeps, gearID, userID string) error {
	owned, err := deps.DB.GearOwnedBy(ctx, gearID, userID)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("FORBIDDEN")
	}
	return nil
}

func requireOwnedActivity(ctx context.Context, deps *WriteExecutionDeps, activityID, userID string) error {
	owned, err := deps.DB.ActivityOwnedBy(ctx, activityID, userID)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("FORBIDDEN")
	}
	return nil
}

func ownedGearName(ctx context.Context, deps *Deps, gearID, userID string) (string, error) {
	owned, err := deps.DB.GearOwnedBy(ctx, gearID, userID)
	if err != nil {
		return "", err
	}
	if !owned {
		return "", fmt.Errorf("FORBIDDEN")
	}
	g, err := deps.DB.GetGear(ctx, gearID)
	if err != nil {
		return "", err
	}
	return g.Name, nil
}

func ownedActivityName(ctx context.Context, deps *Deps, activityID, userID string) (string, error) {
	owned, err := deps.DB.ActivityOwnedBy(ctx, activityID, userID)
	if err != nil {
		return "", err
	}
	if !owned {
		return "", fmt.Errorf("FORBIDDEN")
	}
	a, err := deps.DB.GetActivity(ctx, activityID)
	if err != nil {
		return "", err
	}
	if a.Title != nil && strings.TrimSpace(*a.Title) != "" {
		return *a.Title, nil
	}
	if a.FileHash != nil && strings.TrimSpace(*a.FileHash) != "" {
		return *a.FileHash, nil
	}
	return activityID, nil
}

func updateGearFieldNames(in updateGearMetadataArgs) []string {
	var fields []string
	if in.Fields.Name != nil {
		fields = append(fields, "name")
	}
	if in.Fields.Brand != nil {
		fields = append(fields, "brand")
	}
	if in.Fields.ModelName != nil {
		fields = append(fields, "model_name")
	}
	if in.Fields.Notes != nil {
		fields = append(fields, "notes")
	}
	if in.Fields.Attributes != nil {
		fields = append(fields, "attributes")
	}
	if len(fields) == 0 {
		fields = append(fields, "metadata")
	}
	return fields
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	return &trimmed
}
