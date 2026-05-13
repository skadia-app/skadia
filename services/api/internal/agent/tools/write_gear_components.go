package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

var addGearComponentInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {"type": "string", "minLength": 1},
    "name": {"type": "string", "minLength": 1},
    "type": {"type": "string", "minLength": 1},
    "parent_component_id": {"type": "string", "minLength": 1},
    "brand": {"type": "string"},
    "model": {"type": "string"},
    "purchase_price": {"type": "number"},
    "purchase_date": {"type": "string"},
    "weight_g": {"type": "integer"},
    "serial_number": {"type": "string"},
    "notes": {"type": "string"},
    "preset_gear_type_key": {"type": "string"}
  },
  "required": ["gear_id", "name", "type"],
  "additionalProperties": false
}`)

var updateGearComponentStatusInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "component_id": {"type": "string", "minLength": 1},
    "status": {"type": "string", "enum": ["active", "stored", "retired"]}
  },
  "required": ["component_id", "status"],
  "additionalProperties": false
}`)

var recordGearServiceEventInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "component_id": {"type": "string", "minLength": 1},
    "interval_id": {"type": "string", "minLength": 1},
    "performed_at": {"type": "string", "minLength": 1},
    "distance_at_m": {"type": "number"},
    "description": {"type": "string"},
    "cost": {"type": "number"},
    "notes": {"type": "string"}
  },
  "required": ["component_id", "performed_at"],
  "additionalProperties": false
}`)

var updateGearDefaultPredicatesInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {"type": "string", "minLength": 1},
    "predicates": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "sport_type": {"type": "string", "minLength": 1},
          "sub_type": {"type": "string"},
          "modifiers": {
            "type": "object",
            "properties": {
              "is_electric": {"type": "boolean"},
              "is_indoor": {"type": "boolean"},
              "is_virtual": {"type": "boolean"},
              "is_commute": {"type": "boolean"}
            },
            "additionalProperties": false
          }
        },
        "required": ["sport_type"],
        "additionalProperties": false
      }
    }
  },
  "required": ["gear_id", "predicates"],
  "additionalProperties": false
}`)

var batchSetActivityGearInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "gear_id": {"type": "string", "format": "uuid"},
    "role_key": {"type": "string", "enum": ["primary"]},
    "activity_ids": {
      "type": "array",
      "items": {"type": "string", "format": "uuid"},
      "uniqueItems": true,
      "maxItems": 5000
    },
    "selection": {
      "type": "object",
      "properties": {
        "activity_ids": {"type": "array", "items": {"type": "string", "format": "uuid"}, "maxItems": 5000},
        "sport_query": {"type": "string"},
        "sport_type": {"type": "string"},
        "sub_type": {"type": "string"},
        "days": {"type": "integer", "minimum": 1},
        "date_from": {"type": "string", "format": "date"},
        "date_to": {"type": "string", "format": "date"},
        "min_distance_km": {"type": "number", "minimum": 0},
        "max_distance_km": {"type": "number", "minimum": 0},
        "title_search": {"type": "string"},
        "gear_id": {"type": "string", "format": "uuid"},
        "tag_name": {"type": "string"},
        "is_electric": {"type": "boolean"},
        "is_indoor": {"type": "boolean"},
        "is_virtual": {"type": "boolean"},
        "is_commute": {"type": "boolean"},
        "all_history": {"type": "boolean"}
      },
      "additionalProperties": false
    }
  },
  "required": ["gear_id", "role_key"],
  "additionalProperties": false
}`)

func AddGearComponentTool() *Tool {
	return &Tool{
		Name:                 "add_gear_component",
		Description:          "Add a component to an owned gear item after user confirmation.",
		InputSchema:          addGearComponentInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeAddGearComponent,
		ExecuteWrite:         executeAddGearComponent,
	}
}

func UpdateGearComponentStatusTool() *Tool {
	return &Tool{
		Name:                 "update_gear_component_status",
		Description:          "Set an owned gear component to active, stored, or retired after user confirmation.",
		InputSchema:          updateGearComponentStatusInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateGearComponentStatus,
		ExecuteWrite:         executeUpdateGearComponentStatus,
	}
}

func RecordGearServiceEventTool() *Tool {
	return &Tool{
		Name:                 "record_gear_service_event",
		Description:          "Record a maintenance/service event for an owned gear component after user confirmation.",
		InputSchema:          recordGearServiceEventInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeRecordGearServiceEvent,
		ExecuteWrite:         executeRecordGearServiceEvent,
	}
}

func UpdateGearDefaultPredicatesTool() *Tool {
	return &Tool{
		Name:                 "update_gear_default_predicates",
		Description:          "Replace an owned gear item's default activity predicates after user confirmation.",
		InputSchema:          updateGearDefaultPredicatesInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateGearDefaultPredicates,
		ExecuteWrite:         executeUpdateGearDefaultPredicates,
	}
}

func BatchSetActivityGearTool() *Tool {
	return &Tool{
		Name:                 "batch_set_activity_gear",
		Description:          "Assign an owned gear item as primary gear for a prepared list of owned activities after user confirmation.",
		InputSchema:          batchSetActivityGearInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		PrepareWrite:         prepareBatchSetActivityGear,
		SummaryForUser:       summarizeBatchSetActivityGear,
		ExecuteWrite:         executeBatchSetActivityGear,
	}
}

type addGearComponentArgs struct {
	GearID            string   `json:"gear_id"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	ParentComponentID *string  `json:"parent_component_id"`
	Brand             *string  `json:"brand"`
	Model             *string  `json:"model"`
	PurchasePrice     *float64 `json:"purchase_price"`
	PurchaseDate      *string  `json:"purchase_date"`
	WeightG           *int     `json:"weight_g"`
	SerialNumber      *string  `json:"serial_number"`
	Notes             *string  `json:"notes"`
	PresetGearTypeKey *string  `json:"preset_gear_type_key"`
}

type updateGearComponentStatusArgs struct {
	ComponentID string `json:"component_id"`
	Status      string `json:"status"`
}

type recordGearServiceEventArgs struct {
	ComponentID string   `json:"component_id"`
	IntervalID  *string  `json:"interval_id"`
	PerformedAt string   `json:"performed_at"`
	DistanceAtM *float64 `json:"distance_at_m"`
	Description *string  `json:"description"`
	Cost        *float64 `json:"cost"`
	Notes       *string  `json:"notes"`
}

type updateGearDefaultPredicatesArgs struct {
	GearID     string                         `json:"gear_id"`
	Predicates []gearpkg.GearDefaultPredicate `json:"predicates"`
}

type batchSetActivityGearArgs struct {
	GearID      string          `json:"gear_id"`
	RoleKey     string          `json:"role_key"`
	ActivityIDs []string        `json:"activity_ids,omitempty"`
	Selection   json.RawMessage `json:"selection,omitempty"`
}

func executeAddGearComponent(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("add_gear_component", addGearComponentInputSchema, args); err != nil {
		return nil, err
	}
	var in addGearComponentArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if strings.TrimSpace(in.GearID) == "" || in.Name == "" || in.Type == "" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	if in.ParentComponentID != nil {
		parentGearID, _, err := requireOwnedComponent(ctx, deps, *in.ParentComponentID, userID)
		if err != nil {
			return nil, err
		}
		if parentGearID != in.GearID {
			return nil, fmt.Errorf("FORBIDDEN")
		}
	}
	component, err := createGearComponentTx(ctx, deps, in.GearID, &model.GearComponentCreate{
		Name:              in.Name,
		Type:              in.Type,
		ParentComponentID: trimPtr(in.ParentComponentID),
		Brand:             trimPtr(in.Brand),
		Model:             trimPtr(in.Model),
		PurchasePrice:     in.PurchasePrice,
		PurchaseDate:      trimPtr(in.PurchaseDate),
		WeightG:           in.WeightG,
		SerialNumber:      trimPtr(in.SerialNumber),
		Notes:             in.Notes,
		PresetGearTypeKey: trimPtr(in.PresetGearTypeKey),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": component.ID, "gear_id": in.GearID, "component_id": component.ID, "status": "ok"}, nil
}

func executeUpdateGearComponentStatus(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("update_gear_component_status", updateGearComponentStatusInputSchema, args); err != nil {
		return nil, err
	}
	var in updateGearComponentStatusArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	gearID, _, err := requireOwnedComponent(ctx, deps, in.ComponentID, userID)
	if err != nil {
		return nil, err
	}
	if err := updateGearComponentStatusTx(ctx, deps, in.ComponentID, userID, in.Status); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.ComponentID, "gear_id": gearID, "component_id": in.ComponentID, "component_status": in.Status, "status": "ok"}, nil
}

func executeRecordGearServiceEvent(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("record_gear_service_event", recordGearServiceEventInputSchema, args); err != nil {
		return nil, err
	}
	var in recordGearServiceEventArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	gearID, _, err := requireOwnedComponent(ctx, deps, in.ComponentID, userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.PerformedAt) == "" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	event, err := createServiceEventTx(ctx, deps, in.ComponentID, &database.ServiceEventCreate{
		IntervalID:  trimPtr(in.IntervalID),
		PerformedAt: strings.TrimSpace(in.PerformedAt),
		DistanceAtM: in.DistanceAtM,
		Description: in.Description,
		Cost:        in.Cost,
		Notes:       in.Notes,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": event.ID, "gear_id": gearID, "component_id": in.ComponentID, "service_event_id": event.ID, "status": "ok"}, nil
}

func executeUpdateGearDefaultPredicates(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("update_gear_default_predicates", updateGearDefaultPredicatesInputSchema, args); err != nil {
		return nil, err
	}
	var in updateGearDefaultPredicatesArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	predicates, err := gearpkg.NormalizeGearDefaultPredicates(in.Predicates)
	if err != nil {
		return nil, err
	}
	g, err := deps.DB.UpdateGearTx(ctx, deps.Tx, in.GearID, &model.GearUpdate{ApplicableSportPredicates: &predicates})
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": g.ID, "gear_id": g.ID, "predicate_count": len(predicates), "status": "ok"}, nil
}

func executeBatchSetActivityGear(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("batch_set_activity_gear", batchSetActivityGearInputSchema, args); err != nil {
		return nil, err
	}
	var in batchSetActivityGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GearID) == "" || in.RoleKey != "primary" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	if len(in.ActivityIDs) == 0 {
		return nil, fmt.Errorf("UNSAFE_SELECTION: batch_set_activity_gear requires prepared concrete activity_ids")
	}
	if len(in.Selection) > 0 && string(in.Selection) != "null" && string(in.Selection) != "{}" {
		return nil, fmt.Errorf("UNSAFE_SELECTION: ExecuteWrite rejects unresolved activity selections")
	}
	if err := requireOwnedGear(ctx, deps, in.GearID, userID); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(in.ActivityIDs))
	for _, activityID := range in.ActivityIDs {
		activityID = strings.TrimSpace(activityID)
		if activityID == "" {
			return nil, fmt.Errorf("INVALID_ARGUMENT")
		}
		if _, ok := seen[activityID]; ok {
			return nil, fmt.Errorf("INVALID_ARGUMENT: duplicate activity_id")
		}
		seen[activityID] = struct{}{}
		if err := requireOwnedActivity(ctx, deps, activityID, userID); err != nil {
			return nil, err
		}
	}
	for activityID := range seen {
		if err := database.SetPrimaryGearForActivityTx(ctx, deps.Tx, activityID, in.GearID, "user"); err != nil {
			return nil, err
		}
	}
	return map[string]any{"affected_count": len(seen), "gear_id": in.GearID, "role_key": in.RoleKey, "status": "ok"}, nil
}

func prepareBatchSetActivityGear(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (*PreparedWrite, error) {
	var in batchSetActivityGearArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	in.GearID = strings.TrimSpace(in.GearID)
	if in.GearID == "" || in.RoleKey != "primary" {
		return nil, fmt.Errorf("INVALID_ARGUMENT")
	}
	owned, err := deps.DB.GearOwnedBy(ctx, in.GearID, userID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, fmt.Errorf("FORBIDDEN")
	}
	var criteria activitySelectionCriteria
	if len(in.Selection) > 0 && string(in.Selection) != "null" && string(in.Selection) != "{}" {
		if err := json.Unmarshal(in.Selection, &criteria); err != nil {
			return nil, fmt.Errorf("decode selection: %w", err)
		}
	} else {
		criteria.ActivityIDs = in.ActivityIDs
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	selection, err := resolveActivitySelectionIDs(ctx, pool, userID, criteria, time.Now().UTC(), maxBulkTagActivityIDs)
	if err != nil {
		return nil, err
	}
	if selection.Truncated {
		return nil, fmt.Errorf("selection matches %d activities; refine below %d before assigning gear", selection.Count, maxBulkTagActivityIDs)
	}
	if len(criteria.ActivityIDs) > 0 && selection.Count != len(criteria.ActivityIDs) {
		return nil, fmt.Errorf("one or more activity_ids are not owned by the user")
	}
	prepared := batchSetActivityGearArgs{
		GearID:      in.GearID,
		RoleKey:     in.RoleKey,
		ActivityIDs: selection.IDs,
	}
	preparedRaw, err := json.Marshal(prepared)
	if err != nil {
		return nil, err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return nil, err
	}
	return &PreparedWrite{
		Summary: fmt.Sprintf("Gear %s für %d Aktivitäten als primary setzen", gearName, selection.Count),
		Preview: map[string]any{
			"kind":           "batch_set_activity_gear",
			"gear_id":        in.GearID,
			"gear_name":      gearName,
			"role_key":       in.RoleKey,
			"activity_count": selection.Count,
			"criteria":       selection.Criteria,
			"sample_ids":     sampleStrings(selection.IDs, 10),
			"cap": map[string]any{
				"activity_id_limit": maxBulkTagActivityIDs,
			},
		},
		Args: preparedRaw,
	}, nil
}

func summarizeAddGearComponent(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in addGearComponentArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Komponente %s (%s) zu Gear %s hinzufügen", in.Name, in.Type, gearName), nil
}

func summarizeUpdateGearComponentStatus(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in updateGearComponentStatusArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	name, err := ownedComponentName(ctx, deps, in.ComponentID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Komponentenstatus für %s auf %s setzen", name, in.Status), nil
}

func summarizeRecordGearServiceEvent(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in recordGearServiceEventArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	name, err := ownedComponentName(ctx, deps, in.ComponentID, userID)
	if err != nil {
		return "", err
	}
	description := "Service-Ereignis"
	if in.Description != nil && strings.TrimSpace(*in.Description) != "" {
		description = strings.TrimSpace(*in.Description)
	}
	return fmt.Sprintf("%s für Komponente %s am %s erfassen", description, name, in.PerformedAt), nil
}

func summarizeUpdateGearDefaultPredicates(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in updateGearDefaultPredicatesArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	predicates, err := gearpkg.NormalizeGearDefaultPredicates(in.Predicates)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Default-Prädikate für Gear %s ersetzen (%d Prädikate)", gearName, len(predicates)), nil
}

func summarizeBatchSetActivityGear(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in batchSetActivityGearArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	gearName, err := ownedGearName(ctx, deps, in.GearID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Gear %s für %d vorbereitete Aktivitäten als primary setzen", gearName, len(in.ActivityIDs)), nil
}

func requireOwnedComponent(ctx context.Context, deps *WriteExecutionDeps, componentID, userID string) (string, string, error) {
	var gearID, name string
	err := deps.Tx.QueryRow(ctx, `
		SELECT gc.gear_id::text, gc.name
		FROM gear_components gc
		JOIN gear g ON g.id = gc.gear_id
		WHERE gc.id = $1 AND g.user_id = $2`,
		componentID, userID,
	).Scan(&gearID, &name)
	if err != nil {
		return "", "", fmt.Errorf("FORBIDDEN")
	}
	return gearID, name, nil
}

func ownedComponentName(ctx context.Context, deps *Deps, componentID, userID string) (string, error) {
	if err := deps.DB.VerifyComponentOwnership(ctx, componentID, userID); err != nil {
		return "", fmt.Errorf("FORBIDDEN")
	}
	var name string
	if err := deps.DB.Pool.QueryRow(ctx, `SELECT name FROM gear_components WHERE id = $1`, componentID).Scan(&name); err != nil {
		return "", err
	}
	return name, nil
}

func createGearComponentTx(ctx context.Context, deps *WriteExecutionDeps, gearID string, create *model.GearComponentCreate) (*model.GearComponent, error) {
	var component model.GearComponent
	err := deps.Tx.QueryRow(ctx, `
		INSERT INTO gear_components (gear_id, parent_component_id, name, type, brand, model,
		  purchase_price, purchase_date, weight_g, serial_number, notes, preset_gear_type_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, gear_id, name, type, status, is_retired`,
		gearID, create.ParentComponentID, create.Name, create.Type, create.Brand, create.Model,
		create.PurchasePrice, create.PurchaseDate, create.WeightG, create.SerialNumber, create.Notes,
		create.PresetGearTypeKey,
	).Scan(&component.ID, &component.GearID, &component.Name, &component.Type, &component.Status, &component.IsRetired)
	if err != nil {
		return nil, fmt.Errorf("create component: %w", err)
	}
	return &component, nil
}

func updateGearComponentStatusTx(ctx context.Context, deps *WriteExecutionDeps, componentID, userID, status string) error {
	tag, err := deps.Tx.Exec(ctx, `
		UPDATE gear_components gc
		SET status = $2,
		    is_retired = $3,
		    removed_at = CASE
		        WHEN $2 IN ('stored', 'retired') THEN COALESCE(gc.removed_at, NOW())
		        WHEN $2 = 'active' THEN NULL
		        ELSE gc.removed_at
		    END
		FROM gear g
		WHERE gc.id = $1 AND gc.gear_id = g.id AND g.user_id = $4`,
		componentID, status, status == "retired", userID,
	)
	if err != nil {
		return fmt.Errorf("update component status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("FORBIDDEN")
	}
	if status == "stored" || status == "retired" {
		if _, err := deps.Tx.Exec(ctx, `
			UPDATE component_assignments
			SET removed_at = NOW()
			WHERE component_id = $1 AND removed_at IS NULL`,
			componentID,
		); err != nil {
			return fmt.Errorf("close component assignment: %w", err)
		}
	}
	if status == "active" {
		if _, err := deps.Tx.Exec(ctx, `
			INSERT INTO component_assignments (component_id, gear_id, installed_at)
			SELECT gc.id, gc.gear_id, NOW()
			FROM gear_components gc
			JOIN gear g ON g.id = gc.gear_id
			WHERE gc.id = $1 AND g.user_id = $2
			  AND NOT EXISTS (
			    SELECT 1 FROM component_assignments ca
			    WHERE ca.component_id = gc.id AND ca.removed_at IS NULL
			  )`,
			componentID, userID,
		); err != nil {
			return fmt.Errorf("open component assignment: %w", err)
		}
	}
	return nil
}

func createServiceEventTx(ctx context.Context, deps *WriteExecutionDeps, componentID string, create *database.ServiceEventCreate) (*model.GearServiceEvent, error) {
	var event model.GearServiceEvent
	var metricKey *string
	if create.IntervalID != nil {
		var key string
		if err := deps.Tx.QueryRow(ctx, `
			SELECT gsi.metric_key
			FROM gear_service_intervals gsi
			WHERE gsi.id = $1 AND gsi.component_id = $2`,
			*create.IntervalID, componentID,
		).Scan(&key); err != nil {
			return nil, fmt.Errorf("verify service interval: %w", err)
		}
		metricKey = &key
	}

	err := deps.Tx.QueryRow(ctx, `
		WITH snapshot AS (
		     SELECT jsonb_build_object(
		         'distance_m', COALESCE(total_distance_m, 0),
		         'moving_time_s', COALESCE(total_moving_time_s, 0),
		         'elapsed_time_s', COALESCE(total_time_s, 0),
		         'activity_count', COALESCE(total_activities, 0),
		         'elevation_gain_m', COALESCE(total_elevation_gain_m, 0),
		         'elevation_loss_m', COALESCE(total_elevation_loss_m, 0)
		     ) AS value
		     FROM gear_components
		     WHERE id = $1
		 )
		 INSERT INTO gear_service_events (component_id, interval_id, performed_at, distance_at_m, description, cost, notes, metric_snapshot)
		 SELECT $1, $2, $3, $4, $5, $6, $7, value FROM snapshot
		 RETURNING id, component_id, interval_id, performed_at::text, distance_at_m,
		   description, cost, notes, metric_snapshot, created_at::text`,
		componentID, create.IntervalID, create.PerformedAt, create.DistanceAtM,
		create.Description, create.Cost, create.Notes,
	).Scan(
		&event.ID, &event.ComponentID, &event.IntervalID, &event.PerformedAt, &event.DistanceAtM,
		&event.Description, &event.Cost, &event.Notes, &event.MetricSnapshot, &event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create service event: %w", err)
	}

	if metricKey != nil {
		value := event.MetricSnapshot[*metricKey]
		if _, err := deps.Tx.Exec(ctx, `
			UPDATE gear_service_intervals
			SET last_service_at = $1,
			    last_service_metric_value = $2,
			    last_service_distance_m = CASE WHEN metric_key = 'distance_m' THEN $2 ELSE last_service_distance_m END
			WHERE id = $3 AND component_id = $4`,
			create.PerformedAt, value, *create.IntervalID, componentID,
		); err != nil {
			return nil, fmt.Errorf("reset service interval: %w", err)
		}
	}
	return &event, nil
}
