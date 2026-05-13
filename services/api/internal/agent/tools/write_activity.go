package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

var updateActivityMetadataInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "minLength": 1},
    "fields": {
      "type": "object",
      "properties": {
        "title": {"type": "string"},
        "description": {"type": "string"},
        "sport_type": {"type": "string"},
        "sub_type": {"type": "string"}
      },
      "additionalProperties": false,
      "minProperties": 1
    }
  },
  "required": ["activity_id", "fields"],
  "additionalProperties": false
}`)

var renameActivityInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "minLength": 1},
    "title": {"type": "string", "minLength": 1}
  },
  "required": ["activity_id", "title"],
  "additionalProperties": false
}`)

var tagActivityInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "minLength": 1},
    "tag_name": {"type": "string", "minLength": 1},
    "mode": {"type": "string", "enum": ["add", "remove"]}
  },
  "required": ["activity_id", "tag_name", "mode"],
  "additionalProperties": false
}`)

func WriteActivityTools() []*Tool {
	return []*Tool{
		UpdateActivityMetadataTool(),
		RenameActivityTool(),
		TagActivityTool(),
		BulkTagActivitiesTool(),
	}
}

func UpdateActivityMetadataTool() *Tool {
	return &Tool{
		Name:                 "update_activity_metadata",
		Description:          "Update safe metadata fields on an owned activity after user confirmation.",
		InputSchema:          updateActivityMetadataInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeUpdateActivityMetadata,
		ExecuteWrite:         executeUpdateActivityMetadata,
	}
}

func RenameActivityTool() *Tool {
	return &Tool{
		Name:                 "rename_activity",
		Description:          "Rename an owned activity after user confirmation.",
		InputSchema:          renameActivityInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeRenameActivity,
		ExecuteWrite:         executeRenameActivity,
	}
}

func TagActivityTool() *Tool {
	return &Tool{
		Name:                 "tag_activity",
		Description:          "Add or remove an existing tag on an owned activity after user confirmation.",
		InputSchema:          tagActivityInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeTagActivity,
		ExecuteWrite:         executeTagActivity,
	}
}

type updateActivityMetadataArgs struct {
	ActivityID string `json:"activity_id"`
	Fields     struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		SportType   *string `json:"sport_type"`
		SubType     *string `json:"sub_type"`
	} `json:"fields"`
}

type renameActivityArgs struct {
	ActivityID string `json:"activity_id"`
	Title      string `json:"title"`
}

type tagActivityArgs struct {
	ActivityID string `json:"activity_id"`
	TagName    string `json:"tag_name"`
	Mode       string `json:"mode"`
}

func executeUpdateActivityMetadata(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("update_activity_metadata", updateActivityMetadataInputSchema, args); err != nil {
		return nil, err
	}
	var in updateActivityMetadataArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedActivity(ctx, deps, in.ActivityID, userID); err != nil {
		return nil, err
	}
	activity, err := deps.DB.UpdateActivityMetadataTx(ctx, deps.Tx, in.ActivityID, userID, &model.ActivityUpdate{
		Title:       trimPtr(in.Fields.Title),
		Description: in.Fields.Description,
		SportType:   trimPtr(in.Fields.SportType),
		SubType:     trimPtr(in.Fields.SubType),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": activity.ID, "activity_id": activity.ID, "status": "ok"}, nil
}

func executeRenameActivity(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("rename_activity", renameActivityInputSchema, args); err != nil {
		return nil, err
	}
	var in renameActivityArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedActivity(ctx, deps, in.ActivityID, userID); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	activity, err := deps.DB.UpdateActivityMetadataTx(ctx, deps.Tx, in.ActivityID, userID, &model.ActivityUpdate{Title: &title})
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": activity.ID, "activity_id": activity.ID, "status": "ok"}, nil
}

func executeTagActivity(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
	if err := validateArgs("tag_activity", tagActivityInputSchema, args); err != nil {
		return nil, err
	}
	var in tagActivityArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := requireOwnedActivity(ctx, deps, in.ActivityID, userID); err != nil {
		return nil, err
	}
	tag, err := deps.DB.FindTagByNameForAgent(ctx, userID, strings.TrimSpace(in.TagName))
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return map[string]any{
			"code":           "TAG_NOT_FOUND",
			"ask_user":       true,
			"question":       fmt.Sprintf("Der Tag '%s' existiert noch nicht. Soll ich ihn anlegen?", in.TagName),
			"next_tool":      "create_tag",
			"suggested_args": map[string]any{"name": in.TagName},
		}, fmt.Errorf("TAG_NOT_FOUND")
	}
	if err := deps.DB.AssignActivityToTagTx(ctx, deps.Tx, tag.ID, userID, in.ActivityID, in.Mode); err != nil {
		return nil, err
	}
	return map[string]any{"affected_id": in.ActivityID, "activity_id": in.ActivityID, "tag_id": tag.ID, "status": "ok"}, nil
}

func summarizeUpdateActivityMetadata(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in updateActivityMetadataArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	name, err := ownedActivityName(ctx, deps, in.ActivityID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Aktivität '%s' anpassen: %s", name, strings.Join(activityFieldNames(in), ", ")), nil
}

func summarizeRenameActivity(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in renameActivityArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	name, err := ownedActivityName(ctx, deps, in.ActivityID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Aktivität '%s' umbenennen zu '%s'", name, in.Title), nil
}

func summarizeTagActivity(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error) {
	var in tagActivityArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	tag, err := deps.DB.FindTagByNameForAgent(ctx, userID, strings.TrimSpace(in.TagName))
	if err != nil {
		return "", err
	}
	if tag == nil {
		return "", fmt.Errorf("TAG_NOT_FOUND")
	}
	name, err := ownedActivityName(ctx, deps, in.ActivityID, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Tags der Aktivität '%s': %s %s", name, in.Mode, in.TagName), nil
}

func activityFieldNames(in updateActivityMetadataArgs) []string {
	var fields []string
	if in.Fields.Title != nil {
		fields = append(fields, "title")
	}
	if in.Fields.Description != nil {
		fields = append(fields, "description")
	}
	if in.Fields.SportType != nil {
		fields = append(fields, "sport_type")
	}
	if in.Fields.SubType != nil {
		fields = append(fields, "sub_type")
	}
	return fields
}
