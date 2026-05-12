package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

var dryRunRuleInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "rule_id": {"type": "string", "minLength": 1},
    "rule_body": {"type": "object"},
    "sample_size": {"type": "integer", "minimum": 1, "maximum": 500},
    "filter": {
      "type": "object",
      "properties": {
        "sport_type": {"type": "string"},
        "since": {"type": "string", "format": "date"},
        "until": {"type": "string", "format": "date"}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`)

func DryRunRuleTool() *Tool {
	return &Tool{
		Name:           "dry_run_rule",
		Description:    "Preview which owned activities an automation rule would match without mutating user data.",
		InputSchema:    dryRunRuleInputSchema,
		Permission:     PermissionRead,
		AuditMode:      AuditPreview,
		IsUserOwned:    true,
		ExecutePreview: executeDryRunRule,
	}
}

func RegisterRulePreviewTools(reg *Registry) {
	reg.Register(DryRunRuleTool())
}

type dryRunRuleArgs struct {
	RuleID     *string                     `json:"rule_id"`
	RuleBody   *model.AutomationRuleCreate `json:"rule_body"`
	SampleSize *int                        `json:"sample_size"`
	Filter     *dryRunRuleFilter           `json:"filter"`
}

type dryRunRuleFilter struct {
	SportType string `json:"sport_type"`
	Since     string `json:"since"`
	Until     string `json:"until"`
}

func executeDryRunRule(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error) {
	in, err := decodeDryRunRuleArgs(args)
	if err != nil {
		return nil, err
	}
	if deps == nil || deps.Readonly == nil {
		return nil, fmt.Errorf("readonly pool is required")
	}
	db := &database.DB{Pool: deps.Readonly}
	rule, err := loadDryRunRule(ctx, db, userID, in, deps)
	if err != nil {
		return nil, err
	}
	activityIDs, err := listDryRunActivityIDs(ctx, db, userID, in)
	if err != nil {
		return nil, err
	}
	matches, err := automation.PreviewRuleMatches(ctx, db, userID, *rule, activityIDs)
	if err != nil {
		return nil, err
	}
	matchedIDs := make([]string, 0, len(matches))
	byActivity := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		matchedIDs = append(matchedIDs, match.ActivityID)
		row := map[string]any{
			"activity_id": match.ActivityID,
			"actions":     match.Actions,
		}
		if match.BranchIndex != nil {
			row["branch_index"] = *match.BranchIndex
		}
		byActivity = append(byActivity, row)
	}
	return map[string]any{
		"rule_id":                     rule.ID,
		"matched_activity_ids":        matchedIDs,
		"matched_actions_by_activity": byActivity,
		"total_evaluated":             len(activityIDs),
		"total_matched":               len(matches),
	}, nil
}

func decodeDryRunRuleArgs(args json.RawMessage) (dryRunRuleArgs, error) {
	var in dryRunRuleArgs
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return in, err
	}
	hasRuleID := in.RuleID != nil && strings.TrimSpace(*in.RuleID) != ""
	hasRuleBody := in.RuleBody != nil
	if hasRuleID == hasRuleBody {
		return in, fmt.Errorf("exactly one of rule_id or rule_body is required")
	}
	if in.Filter != nil {
		if _, err := parseDryRunDate(in.Filter.Since); err != nil {
			return in, err
		}
		if _, err := parseDryRunDate(in.Filter.Until); err != nil {
			return in, err
		}
	}
	return in, nil
}

func loadDryRunRule(ctx context.Context, db *database.DB, userID string, in dryRunRuleArgs, deps *PreviewExecutionDeps) (*model.AutomationRule, error) {
	if in.RuleID != nil {
		rule, err := database.LoadRuleForPreview(ctx, deps.Readonly, userID, strings.TrimSpace(*in.RuleID))
		if err != nil {
			return nil, err
		}
		if rule == nil {
			return nil, fmt.Errorf("rule not found")
		}
		return rule, nil
	}
	create := *in.RuleBody
	if create.TriggerEvent == "" {
		create.TriggerEvent = "on_import"
	}
	fullDeps := &Deps{DB: db}
	normalized, err := decodeCreateAutomationRuleArgs(ctx, fullDeps, userID, mustMarshalRaw(create))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &model.AutomationRule{
		ID:               "adhoc",
		UserID:           userID,
		Name:             normalized.Name,
		IsActive:         true,
		Priority:         normalized.Priority,
		Conditions:       normalized.Conditions,
		ConditionsLogic:  normalized.ConditionsLogic,
		ConditionGroups:  normalized.ConditionGroups,
		Actions:          normalized.Actions,
		TriggerEvent:     normalized.TriggerEvent,
		StopOnMatch:      normalized.StopOnMatch,
		ApplyRetroactive: normalized.ApplyRetroactive,
		Branches:         normalized.Branches,
		CreatedAt:        now,
	}, nil
}

func listDryRunActivityIDs(ctx context.Context, db *database.DB, userID string, in dryRunRuleArgs) ([]string, error) {
	limit := 50
	if in.SampleSize != nil {
		limit = *in.SampleSize
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	where := []string{"user_id = $1"}
	params := []any{userID}
	if in.Filter != nil {
		if strings.TrimSpace(in.Filter.SportType) != "" {
			params = append(params, strings.TrimSpace(in.Filter.SportType))
			where = append(where, fmt.Sprintf("sport_type = $%d", len(params)))
		}
		if since, _ := parseDryRunDate(in.Filter.Since); since != nil {
			params = append(params, *since)
			where = append(where, fmt.Sprintf("started_at::date >= $%d", len(params)))
		}
		if until, _ := parseDryRunDate(in.Filter.Until); until != nil {
			params = append(params, *until)
			where = append(where, fmt.Sprintf("started_at::date <= $%d", len(params)))
		}
	}
	params = append(params, limit)
	query := fmt.Sprintf(`SELECT id::text FROM activities WHERE %s ORDER BY started_at DESC NULLS LAST, created_at DESC LIMIT $%d`, strings.Join(where, " AND "), len(params))
	rows, err := db.Pool.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("list dry-run activity ids: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dry-run activity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dry-run activity ids: %w", err)
	}
	return ids, nil
}

func parseDryRunDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q", value)
	}
	return &parsed, nil
}

func mustMarshalRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
