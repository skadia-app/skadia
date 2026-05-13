package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

var bulkTagActivitiesInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "tag_name": {"type": "string", "minLength": 1},
    "mode": {"type": "string", "enum": ["add", "remove"]},
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
    },
    "prepared": {"type": "boolean"},
    "prepared_tag_id": {"type": "string", "format": "uuid"},
    "prepared_activity_ids": {"type": "array", "items": {"type": "string", "format": "uuid"}, "maxItems": 5000},
    "prepared_count": {"type": "integer", "minimum": 0},
    "prepared_criteria": {"type": "object"}
  },
  "required": ["tag_name", "mode"],
  "additionalProperties": false
}`)

type bulkTagActivitiesArgs struct {
	TagName             string                    `json:"tag_name"`
	Mode                string                    `json:"mode"`
	Selection           activitySelectionCriteria `json:"selection,omitempty"`
	Prepared            bool                      `json:"prepared,omitempty"`
	PreparedTagID       string                    `json:"prepared_tag_id,omitempty"`
	PreparedActivityIDs []string                  `json:"prepared_activity_ids,omitempty"`
	PreparedCount       int                       `json:"prepared_count,omitempty"`
	PreparedCriteria    map[string]any            `json:"prepared_criteria,omitempty"`
}

func BulkTagActivitiesTool() *Tool {
	return &Tool{
		Name:                 "bulk_tag_activities",
		Description:          "Add or remove an existing tag on a prepared activity selection after user confirmation.",
		InputSchema:          bulkTagActivitiesInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		PrepareWrite:         prepareBulkTagActivities,
		SummaryForUser:       summarizeBulkTagActivities,
		ExecuteWrite:         executeBulkTagActivities,
	}
}

func prepareBulkTagActivities(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (*PreparedWrite, error) {
	in, err := decodeBulkTagActivitiesArgs(raw)
	if err != nil {
		return nil, err
	}
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	if deps == nil || deps.DB == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	tag, err := deps.DB.FindTagByNameForAgent(ctx, userID, in.TagName)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, fmt.Errorf("TAG_NOT_FOUND")
	}
	selection, err := resolveActivitySelectionIDs(ctx, pool, userID, in.Selection, time.Now().UTC(), maxBulkTagActivityIDs)
	if err != nil {
		return nil, err
	}
	if selection.Truncated {
		return nil, fmt.Errorf("selection matches %d activities; refine below %d before bulk tagging", selection.Count, maxBulkTagActivityIDs)
	}
	prepared := bulkTagActivitiesArgs{
		TagName:             in.TagName,
		Mode:                in.Mode,
		Selection:           in.Selection,
		Prepared:            true,
		PreparedTagID:       tag.ID,
		PreparedActivityIDs: selection.IDs,
		PreparedCount:       selection.Count,
		PreparedCriteria:    selection.Criteria,
	}
	preparedRaw, err := json.Marshal(prepared)
	if err != nil {
		return nil, err
	}
	preview := map[string]any{
		"tag_name": in.TagName,
		"mode":     in.Mode,
		"count":    selection.Count,
		"criteria": selection.Criteria,
		"cap": map[string]any{
			"activity_id_limit": maxBulkTagActivityIDs,
		},
		"truncated": false,
	}
	return &PreparedWrite{
		Summary: bulkTagSummary(in.Mode, in.TagName, selection.Count),
		Preview: preview,
		Args:    preparedRaw,
	}, nil
}

func executeBulkTagActivities(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	in, err := decodeBulkTagActivitiesArgs(raw)
	if err != nil {
		return nil, err
	}
	if !in.Prepared {
		return nil, fmt.Errorf("bulk_tag_activities requires prepared activity_ids")
	}
	if len(in.PreparedActivityIDs) > maxBulkTagActivityIDs {
		return nil, fmt.Errorf("prepared activity_ids exceed limit %d", maxBulkTagActivityIDs)
	}
	if deps == nil || deps.DB == nil || deps.Tx == nil {
		return nil, fmt.Errorf("database transaction is not configured")
	}
	var currentTagName string
	if err := deps.Tx.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`,
		in.PreparedTagID, userID,
	).Scan(&currentTagName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("resolve prepared tag: %w", err)
	}
	affected := make([]string, 0, len(in.PreparedActivityIDs))
	for _, activityID := range in.PreparedActivityIDs {
		if err := deps.DB.AssignActivityToTagTx(ctx, deps.Tx, in.PreparedTagID, userID, activityID, in.Mode); err != nil {
			return nil, err
		}
		affected = append(affected, activityID)
	}
	return map[string]any{
		"status":       "ok",
		"tag_id":       in.PreparedTagID,
		"tag_name":     currentTagName,
		"mode":         in.Mode,
		"affected":     len(affected),
		"affected_ids": affected,
	}, nil
}

func summarizeBulkTagActivities(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (string, error) {
	in, err := decodeBulkTagActivitiesArgs(raw)
	if err != nil {
		return "", err
	}
	if in.Prepared {
		return bulkTagSummary(in.Mode, in.TagName, len(in.PreparedActivityIDs)), nil
	}
	prepared, err := prepareBulkTagActivities(ctx, deps, userID, raw)
	if err != nil {
		return "", err
	}
	return prepared.Summary + " (prepared-args hook required)", nil
}

func decodeBulkTagActivitiesArgs(raw json.RawMessage) (bulkTagActivitiesArgs, error) {
	var in bulkTagActivitiesArgs
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, err
	}
	in.TagName = strings.TrimSpace(in.TagName)
	in.Mode = strings.TrimSpace(in.Mode)
	in.PreparedTagID = strings.TrimSpace(in.PreparedTagID)
	if in.TagName == "" {
		return in, fmt.Errorf("tag_name is required")
	}
	switch in.Mode {
	case "add", "remove":
	default:
		return in, fmt.Errorf("INVALID_MODE")
	}
	if in.Prepared {
		normalizedIDs, err := normalizePreparedActivityIDs(in.PreparedActivityIDs)
		if err != nil {
			return in, err
		}
		in.PreparedActivityIDs = normalizedIDs
		if in.PreparedTagID == "" {
			return in, fmt.Errorf("prepared_tag_id is required")
		}
	}
	return in, nil
}

func normalizePreparedActivityIDs(activityIDs []string) ([]string, error) {
	criteria, err := normalizeActivitySelectionCriteria(activitySelectionCriteria{ActivityIDs: activityIDs})
	if err != nil {
		return nil, err
	}
	return criteria.ActivityIDs, nil
}

func bulkTagSummary(mode, tagName string, count int) string {
	verb := "hinzufügen zu"
	if mode == "remove" {
		verb = "entfernen von"
	}
	return fmt.Sprintf("Tag '%s' %s %d Aktivitäten", tagName, verb, count)
}
