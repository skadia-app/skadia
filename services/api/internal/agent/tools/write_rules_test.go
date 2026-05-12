package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWriteRuleToolsMetadata(t *testing.T) {
	reg := NewRegistry()
	RegisterRuleWriteTools(reg)
	for _, name := range []string{"create_automation_rule", "update_automation_rule", "delete_automation_rule", "enable_automation_rule", "disable_automation_rule"} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("write tool %q was not registered", name)
		}
		if tool.Permission != PermissionWriteSafe || !tool.RequiresConfirmation || !tool.IsUserOwned {
			t.Fatalf("unexpected write metadata for %s: %#v", name, tool)
		}
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("rule tool schemas must validate: %v", err)
	}
}

func TestCreateAutomationRuleWriteToolCreatesRule(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	tagID := insertActivityToolTag(t, pool, userID, "Favorit")
	envelope := ruleWriteEnvelope(t, pool, userID, "create_automation_rule")
	sessionID := uuid.NewString()
	args := []byte(`{"name":"Favoritenregel","trigger_event":"on_gear_usage_update","conditions":[],"actions":[{"action":"add_tag","params":{"tag_id":"` + tagID + `"}}]}`)

	prepared, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-create-rule", ToolName: "create_automation_rule", Args: args,
	})
	if err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if !strings.Contains(prepared.Summary, "on_gear_usage_update") {
		t.Fatalf("summary = %q, want trigger event", prepared.Summary)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-create-rule")
	result, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-create-rule", ToolName: "create_automation_rule", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	ruleID := result.(map[string]any)["rule_id"].(string)
	var name, triggerEvent string
	var actions json.RawMessage
	if err := pool.QueryRow(context.Background(),
		`SELECT name, trigger_event, actions FROM automation_rules WHERE id = $1 AND user_id = $2`,
		ruleID, userID,
	).Scan(&name, &triggerEvent, &actions); err != nil {
		t.Fatalf("select rule: %v", err)
	}
	if name != "Favoritenregel" || triggerEvent != "on_gear_usage_update" || !strings.Contains(string(actions), tagID) {
		t.Fatalf("unexpected rule row: name=%q trigger=%q actions=%s", name, triggerEvent, actions)
	}
}

func TestCreateAutomationRuleWriteToolRejectsDescriptionAndUnknownAction(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	envelope := ruleWriteEnvelope(t, pool, userID, "create_automation_rule")

	_, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-rule-description", ToolName: "create_automation_rule", Args: []byte(`{"name":"Bad","description":"nope","trigger_event":"on_import","conditions":[],"actions":[]}`),
	})
	if err == nil {
		t.Fatal("PrepareConfirmation accepted unsupported description field")
	}

	_, err = envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-rule-action", ToolName: "create_automation_rule", Args: []byte(`{"name":"Bad","trigger_event":"on_import","conditions":[],"actions":[{"action":"teleport","params":{}}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("PrepareConfirmation err = %v, want unknown action", err)
	}
}

func TestUpdateAutomationRuleWriteToolUpdatesActiveAndRejectsDescription(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	db := &database.DB{Pool: pool}
	rule := insertAutomationRuleToolRule(t, db, userID, "Alt")
	envelope := ruleWriteEnvelope(t, pool, userID, "update_automation_rule")
	sessionID := uuid.NewString()

	bad := []byte(`{"rule_id":"` + rule.ID + `","fields":{"description":"nope"}}`)
	if _, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-update-rule-description", ToolName: "update_automation_rule", Args: bad,
	}); err == nil {
		t.Fatal("PrepareConfirmation accepted unsupported description field")
	}

	args := []byte(`{"rule_id":"` + rule.ID + `","fields":{"name":"Neu","is_active":false}}`)
	if _, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-update-rule", ToolName: "update_automation_rule", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-update-rule")
	if _, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-update-rule", ToolName: "update_automation_rule", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var name string
	var active bool
	if err := pool.QueryRow(context.Background(), `SELECT name, is_active FROM automation_rules WHERE id = $1`, rule.ID).Scan(&name, &active); err != nil {
		t.Fatalf("select rule: %v", err)
	}
	if name != "Neu" || active {
		t.Fatalf("rule = (%q, active=%v), want Neu false", name, active)
	}
}

func TestAutomationRuleWriteToolsRejectCrossUserAndToggle(t *testing.T) {
	pool := testdb.Open(t)
	userA := insertActivityToolUser(t, pool)
	userB := insertActivityToolUser(t, pool)
	db := &database.DB{Pool: pool}
	ruleB := insertAutomationRuleToolRule(t, db, userB, "Fremd")
	envelopeA := ruleWriteEnvelope(t, pool, userA, "update_automation_rule")

	_, err := envelopeA.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userA, SessionID: uuid.NewString(), ToolCallID: "call-cross-rule", ToolName: "update_automation_rule", Args: []byte(`{"rule_id":"` + ruleB.ID + `","fields":{"name":"Nope"}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("PrepareConfirmation err = %v, want FORBIDDEN", err)
	}

	ruleA := insertAutomationRuleToolRule(t, db, userA, "Toggle")
	envelopeToggle := ruleWriteEnvelope(t, pool, userA, "disable_automation_rule", "enable_automation_rule")
	disableArgs := []byte(`{"rule_id":"` + ruleA.ID + `"}`)
	sessionID := uuid.NewString()
	if _, err := envelopeToggle.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userA, SessionID: sessionID, ToolCallID: "call-disable-rule", ToolName: "disable_automation_rule", Args: disableArgs,
	}); err != nil {
		t.Fatalf("PrepareConfirmation disable: %v", err)
	}
	confirmEnvelopeTool(t, envelopeToggle, sessionID, "call-disable-rule")
	if _, err := envelopeToggle.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userA, SessionID: sessionID, ToolCallID: "call-disable-rule", ToolName: "disable_automation_rule", Args: disableArgs,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed disable: %v", err)
	}
	var active bool
	if err := pool.QueryRow(context.Background(), `SELECT is_active FROM automation_rules WHERE id = $1`, ruleA.ID).Scan(&active); err != nil {
		t.Fatalf("select disabled: %v", err)
	}
	if active {
		t.Fatal("disable_automation_rule left rule active")
	}
}

func TestAutomationRuleTriggerEventAllowlistMatchesConstraint(t *testing.T) {
	pool := testdb.Open(t)
	var definition string
	if err := pool.QueryRow(context.Background(), `
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conrelid = 'automation_rules'::regclass AND conname = 'chk_trigger_event'`,
	).Scan(&definition); err != nil {
		t.Fatalf("query chk_trigger_event: %v", err)
	}
	for event := range automationTriggerEvents {
		if !strings.Contains(definition, event) {
			t.Fatalf("trigger event %q missing from DB constraint %s", event, definition)
		}
	}
}

func insertAutomationRuleToolRule(t *testing.T, db *database.DB, userID, name string) *model.AutomationRule {
	t.Helper()
	rule, err := db.CreateRule(context.Background(), userID, &model.AutomationRuleCreate{
		Name:         name,
		Conditions:   json.RawMessage(`[]`),
		Actions:      json.RawMessage(`[]`),
		TriggerEvent: "on_import",
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	return rule
}

func ruleWriteEnvelope(t *testing.T, pool *pgxpool.Pool, userID string, allowedTools ...string) *ExecutionEnvelope {
	t.Helper()
	db := &database.DB{Pool: pool}
	if _, err := db.UpsertChatPrefs(context.Background(), userID, database.ChatUserPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: allowedTools,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	reg := NewRegistry()
	for _, tool := range WriteRuleTools() {
		reg.Register(tool)
	}
	return NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(5 * time.Minute),
		Preferences: &agent.PreferenceLoader{LoadFunc: func(ctx context.Context, userID string) (*agent.ChatPreferences, error) {
			prefs, err := db.GetChatPrefs(ctx, userID)
			if err != nil {
				return nil, err
			}
			return &agent.ChatPreferences{
				ChatWriteToolsEnabled: prefs.ChatWriteToolsEnabled,
				ChatAllowedWriteTools: prefs.ChatAllowedWriteTools,
			}, nil
		}},
	})
}
