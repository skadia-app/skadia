package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDryRunRuleToolPreviewsExistingRuleAndAudits(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Regelrunde")
	tagID := insertActivityToolTag(t, pool, userID, "Preview")
	db := &database.DB{Pool: pool}
	rule, err := db.CreateRule(context.Background(), userID, &model.AutomationRuleCreate{
		Name:         "Preview Rule",
		Conditions:   json.RawMessage(`[{"field":"sport_type","op":"eq","value":"running"}]`),
		Actions:      json.RawMessage(`[{"action":"add_tag","params":{"tag_id":"` + tagID + `"}}]`),
		TriggerEvent: "on_import",
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	envelope := dryRunRuleEnvelope(pool)
	args := json.RawMessage(`{"rule_id":"` + rule.ID + `","sample_size":10}`)
	result, err := envelope.ExecutePreview(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-dry-run", ToolName: "dry_run_rule", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecutePreview: %v", err)
	}
	out := result.(map[string]any)
	if int(out["total_matched"].(float64)) != 1 {
		t.Fatalf("total_matched = %#v, want 1 in %#v", out["total_matched"], out)
	}
	matchedIDs := out["matched_activity_ids"].([]any)
	if len(matchedIDs) != 1 || matchedIDs[0] != activityID {
		t.Fatalf("matched_activity_ids = %#v, want %s", matchedIDs, activityID)
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT result_status FROM agent_audit_log WHERE user_id = $1 AND tool_call_id = 'call-dry-run'`, userID).Scan(&status); err != nil {
		t.Fatalf("select audit: %v", err)
	}
	if status != "executed" {
		t.Fatalf("audit status = %q, want executed", status)
	}
}

func TestDryRunRuleToolReturnsFirstMatchedBranchActions(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Branchrunde")
	envelope := dryRunRuleEnvelope(pool)
	args := json.RawMessage(`{
		"rule_body": {
			"name": "Branch Preview",
			"trigger_event": "on_import",
			"branches": [
				{"type":"if","condition_groups":[{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"cycling"}]}],"actions":[{"action":"set_name","params":{"template":"cycling"}}]},
				{"type":"else","actions":[{"action":"set_name","params":{"template":"fallback"}}]}
			]
		},
		"sample_size": 10
	}`)
	result, err := envelope.ExecutePreview(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-dry-run-branch", ToolName: "dry_run_rule", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecutePreview: %v", err)
	}
	out := result.(map[string]any)
	rows := out["matched_actions_by_activity"].([]any)
	if len(rows) != 1 {
		t.Fatalf("matched_actions_by_activity len = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0].(map[string]any)
	if row["activity_id"] != activityID || int(row["branch_index"].(float64)) != 1 {
		t.Fatalf("branch row = %#v, want activity %s branch 1", row, activityID)
	}
	encoded, _ := json.Marshal(row["actions"])
	if !strings.Contains(string(encoded), "fallback") || strings.Contains(string(encoded), "cycling") {
		t.Fatalf("actions = %s, want fallback branch only", encoded)
	}
}

func TestDryRunRuleToolRequiresExactlyOneRuleSource(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	envelope := dryRunRuleEnvelope(pool)
	_, err := envelope.ExecutePreview(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-dry-run-invalid", ToolName: "dry_run_rule", Args: json.RawMessage(`{}`),
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("ExecutePreview err = %v, want exactly one rule source", err)
	}
}

func dryRunRuleEnvelope(pool *pgxpool.Pool) *ExecutionEnvelope {
	db := &database.DB{Pool: pool}
	reg := NewRegistry()
	reg.Register(DryRunRuleTool())
	return NewExecutionEnvelope(reg, &Deps{
		DB:       db,
		Readonly: pool,
		Audit:    agent.NewAuditWriter(db),
	})
}
