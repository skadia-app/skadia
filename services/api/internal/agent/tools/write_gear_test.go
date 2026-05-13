package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/google/uuid"
)

func TestCreateGearWriteToolAuditAndIdempotency(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	envelope := gearWriteEnvelope(t, fx, "create_gear")
	sessionID := uuid.NewString()
	toolCallID := "call-create-gear"
	args := json.RawMessage(`{"name":"Agent Bike","gear_type":"bike","brand":"Acme"}`)

	prepared, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: toolCallID, ToolName: "create_gear", Args: args,
	})
	if err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if prepared == nil || !strings.Contains(prepared.Summary, "Agent Bike") {
		t.Fatalf("prepared summary = %#v", prepared)
	}

	confirmEnvelopeTool(t, envelope, sessionID, toolCallID)
	first, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: toolCallID, ToolName: "create_gear", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecuteConfirmed first: %v", err)
	}
	second, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: toolCallID, ToolName: "create_gear", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecuteConfirmed replay: %v", err)
	}
	if first.(map[string]any)["gear_id"] != second.(map[string]any)["gear_id"] {
		t.Fatalf("replay result = %#v, first = %#v", second, first)
	}

	var gearCount int
	if err := fx.pool.QueryRow(ctx,
		`SELECT count(*) FROM gear WHERE user_id = $1 AND name = 'Agent Bike'`,
		fx.userID,
	).Scan(&gearCount); err != nil {
		t.Fatalf("count gear: %v", err)
	}
	if gearCount != 1 {
		t.Fatalf("gear count = %d, want 1", gearCount)
	}

	var auditStatus string
	if err := fx.pool.QueryRow(ctx,
		`SELECT result_status FROM agent_audit_log WHERE user_id = $1 AND session_id = $2 AND tool_call_id = $3`,
		fx.userID, sessionID, toolCallID,
	).Scan(&auditStatus); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if auditStatus != "executed" {
		t.Fatalf("audit status = %s, want executed", auditStatus)
	}
}

func TestSetActivityGearRejectsForeignGear(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 1200, 600, 10, 10, 0, 0, 0)
	foreignUserID := fx.insertUser(t)
	foreignGearID := fx.insertGearForUser(t, foreignUserID, "Foreign Bike", "bike", "bike", false)
	envelope := gearWriteEnvelope(t, fx, "set_activity_gear")
	args := json.RawMessage(`{"activity_id":"` + activityID + `","gear_id":"` + foreignGearID + `","role_key":"primary"}`)

	_, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: uuid.NewString(), ToolCallID: "call-foreign-gear", ToolName: "set_activity_gear", Args: args,
	})
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("PrepareConfirmation err = %v, want FORBIDDEN", err)
	}
}

func TestSetActivityGearStoresUserSource(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	activityID := fx.insertActivity(t, 1200, 600, 10, 10, 0, 0, 0)
	envelope := gearWriteEnvelope(t, fx, "set_activity_gear")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"activity_id":"` + activityID + `","gear_id":"` + fx.gearID + `","role_key":"primary"}`)

	if _, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-set-gear", ToolName: "set_activity_gear", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-set-gear")
	if _, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-set-gear", ToolName: "set_activity_gear", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}

	var source string
	if err := fx.pool.QueryRow(ctx,
		`SELECT source FROM activity_gear_assignments WHERE activity_id = $1 AND role_key = 'primary'`,
		activityID,
	).Scan(&source); err != nil {
		t.Fatalf("select assignment source: %v", err)
	}
	if source != "user" {
		t.Fatalf("assignment source = %q, want user", source)
	}
}

func TestRetireGearWriteToolUpdatesStatus(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	envelope := gearWriteEnvelope(t, fx, "retire_gear")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"gear_id":"` + fx.gearID + `","retirement_date_iso":"2026-05-12"}`)

	if _, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-retire", ToolName: "retire_gear", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-retire")
	if _, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-retire", ToolName: "retire_gear", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}

	var status string
	var retired bool
	if err := fx.pool.QueryRow(ctx, `SELECT status, is_retired FROM gear WHERE id = $1`, fx.gearID).Scan(&status, &retired); err != nil {
		t.Fatalf("select gear status: %v", err)
	}
	if status != "retired" || !retired {
		t.Fatalf("status=%s retired=%v, want retired/true", status, retired)
	}
}

func TestAddGearComponentRejectsForeignParentComponent(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	foreignUserID := fx.insertUser(t)
	foreignGearID := fx.insertGearForUser(t, foreignUserID, "Foreign Bike", "bike", "bike", false)
	foreignComponentID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO gear_components (id, gear_id, name, type, status, is_retired)
		VALUES ($1, $2, 'Foreign Chain', 'chain', 'active', false)`,
		foreignComponentID, foreignGearID,
	); err != nil {
		t.Fatalf("insert foreign component: %v", err)
	}
	envelope := gearWriteEnvelope(t, fx, "add_gear_component")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"gear_id":"` + fx.gearID + `","name":"Child Chain","type":"chain","parent_component_id":"` + foreignComponentID + `"}`)

	if _, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-add-component", ToolName: "add_gear_component", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-add-component")
	_, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-add-component", ToolName: "add_gear_component", Args: args,
	})
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("ExecuteConfirmed err = %v, want FORBIDDEN", err)
	}
}

func TestRecordGearServiceEventRejectsForeignComponentAtPrepare(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	foreignUserID := fx.insertUser(t)
	foreignGearID := fx.insertGearForUser(t, foreignUserID, "Foreign Bike", "bike", "bike", false)
	foreignComponentID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO gear_components (id, gear_id, name, type, status, is_retired)
		VALUES ($1, $2, 'Foreign Chain', 'chain', 'active', false)`,
		foreignComponentID, foreignGearID,
	); err != nil {
		t.Fatalf("insert foreign component: %v", err)
	}
	envelope := gearWriteEnvelope(t, fx, "record_gear_service_event")
	args := json.RawMessage(`{"component_id":"` + foreignComponentID + `","performed_at":"2026-05-13","description":"Cleaned"}`)

	_, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: uuid.NewString(), ToolCallID: "call-service", ToolName: "record_gear_service_event", Args: args,
	})
	if err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("PrepareConfirmation err = %v, want FORBIDDEN", err)
	}
}

func TestUpdateGearDefaultPredicatesRejectsUnknownSport(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	envelope := gearWriteEnvelope(t, fx, "update_gear_default_predicates")
	args := json.RawMessage(`{"gear_id":"` + fx.gearID + `","predicates":[{"sport_type":"not_a_sport"}]}`)

	_, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: uuid.NewString(), ToolCallID: "call-predicates", ToolName: "update_gear_default_predicates", Args: args,
	})
	if err == nil {
		t.Fatal("PrepareConfirmation accepted unknown sport predicate")
	}
}

func TestBatchSetActivityGearRequiresPreparedActivityIDs(t *testing.T) {
	ctx := context.Background()
	args := json.RawMessage(`{"gear_id":"` + uuid.NewString() + `","role_key":"primary","selection":{"sport_type":"cycling"}}`)

	_, err := BatchSetActivityGearTool().ExecuteWrite(ctx, nil, uuid.NewString(), args)
	if err == nil || !strings.Contains(err.Error(), "UNSAFE_SELECTION") {
		t.Fatalf("ExecuteConfirmed err = %v, want UNSAFE_SELECTION", err)
	}
}

func TestBatchSetActivityGearSetsConcreteOwnedActivities(t *testing.T) {
	fx := newGearToolFixture(t)
	ctx := context.Background()
	activityA := fx.insertActivity(t, 1200, 600, 10, 10, 0, 0, 0)
	activityB := fx.insertActivity(t, 1300, 700, 20, 20, 0, 0, 0)
	envelope := gearWriteEnvelope(t, fx, "batch_set_activity_gear")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"gear_id":"` + fx.gearID + `","role_key":"primary","activity_ids":["` + activityA + `","` + activityB + `"]}`)

	if _, err := envelope.PrepareConfirmation(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-batch-concrete", ToolName: "batch_set_activity_gear", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-batch-concrete")
	result, err := envelope.ExecuteConfirmed(ctx, ToolExecutionRequest{
		UserID: fx.userID, SessionID: sessionID, ToolCallID: "call-batch-concrete", ToolName: "batch_set_activity_gear", Args: args,
	})
	if err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	if result.(map[string]any)["affected_count"] != 2 {
		t.Fatalf("result = %#v, want affected_count 2", result)
	}

	var count int
	if err := fx.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM activity_gear_assignments
		WHERE activity_id IN ($1, $2) AND gear_id = $3 AND role_key = 'primary' AND source = 'user'`,
		activityA, activityB, fx.gearID,
	).Scan(&count); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if count != 2 {
		t.Fatalf("assignment count = %d, want 2", count)
	}
}

func gearWriteEnvelope(t *testing.T, fx gearToolFixture, allowedTools ...string) *ExecutionEnvelope {
	t.Helper()
	if _, err := fx.db.UpsertChatPrefs(context.Background(), fx.userID, database.ChatUserPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: allowedTools,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	reg := NewRegistry()
	for _, tool := range WriteGearTools() {
		reg.Register(tool)
	}
	return NewExecutionEnvelope(reg, &Deps{
		DB:      fx.db,
		Audit:   agent.NewAuditWriter(fx.db),
		Confirm: agent.NewConfirmationManager(5 * time.Minute),
		Preferences: &agent.PreferenceLoader{LoadFunc: func(ctx context.Context, userID string) (*agent.ChatPreferences, error) {
			prefs, err := fx.db.GetChatPrefs(ctx, userID)
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
