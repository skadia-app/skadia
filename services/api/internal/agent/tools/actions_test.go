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
	"github.com/jackc/pgx/v5"
)

func TestPrepareActionCreatesPendingAuditWithoutMutationAndRetainsCallerContext(t *testing.T) {
	calls := 0
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
		calls++
		return map[string]any{"updated": true}, nil
	}))
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, actionDeps(db, userID))
	sessionID := uuid.NewString()

	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  sessionID,
		ToolCallID: "call-prepare",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Caller:     CallerContext{Transport: "local_mcp", ClientID: "codex-cli", HostApprovalID: "host-ok"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if calls != 0 {
		t.Fatalf("write calls before confirmation = %d, want 0", calls)
	}
	if prepared.ActionID == "" || prepared.Status != ActionStatusPending || prepared.ConfirmationURL == "" {
		t.Fatalf("prepared action = %#v", prepared)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusPending || status.Caller.Transport != "local_mcp" || status.Caller.HostApprovalID != "host-ok" {
		t.Fatalf("status = %#v, want pending caller metadata", status)
	}
}

func TestExecuteConfirmedActionRequiresSkadiaConfirmation(t *testing.T) {
	calls := 0
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
		calls++
		return map[string]any{"updated": true}, nil
	}))
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(reg, actionDepsWithConfirm(db, userID, confirm))
	sessionID := uuid.NewString()
	toolCallID := "call-confirm"

	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  sessionID,
		ToolCallID: toolCallID,
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Caller:     CallerContext{Transport: "local_mcp", HostApprovalID: "approved-by-host"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if _, err := envelope.ExecuteConfirmedAction(context.Background(), prepared.ActionID, userID); err == nil {
		t.Fatal("ExecuteConfirmedAction succeeded without Skadia confirmation")
	}
	if calls != 0 {
		t.Fatalf("write calls without confirmation = %d, want 0", calls)
	}
	if !confirm.Resolve(sessionID, toolCallID, agent.ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	result, err := envelope.ExecuteConfirmedAction(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("ExecuteConfirmedAction after confirmation: %v", err)
	}
	if calls != 1 || result.(map[string]any)["updated"] != true {
		t.Fatalf("calls=%d result=%#v, want one update", calls, result)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus executed: %v", err)
	}
	if status.Status != ActionStatusExecuted {
		t.Fatalf("status = %#v, want executed", status)
	}
}

func TestCallerContextCannotBypassWriteAllowList(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(time.Minute),
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity"},
		}},
	})

	_, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-disallowed",
		ToolName:   "delete_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp", HostApprovalID: "approved-by-host", Scopes: []string{"write:*"}},
	})
	if err == nil {
		t.Fatal("PrepareAction allowed caller context to bypass per-tool allow-list")
	}
}

func TestExecuteConfirmedActionRejectsForgedAndForeignActionIDs(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))

	if _, err := envelope.ExecuteConfirmedAction(context.Background(), "audit:999999999", userID); err == nil {
		t.Fatal("forged action id succeeded")
	}
	otherUser := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		otherUser, "foreign-action-"+otherUser[:8], "foreign-action-"+otherUser[:8]+"@example.com"); err != nil {
		t.Fatalf("insert foreign user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, otherUser) })

	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-foreign",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if _, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, otherUser); err == nil {
		t.Fatal("foreign user read action status")
	}
}

func TestRecordDeclinedActionStoresCallerMetadata(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-decline-action",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp", RequestID: "req-1"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := envelope.RecordDeclinedAction(context.Background(), prepared.ActionID, userID, "declined"); err != nil {
		t.Fatalf("RecordDeclinedAction: %v", err)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusDeclined || status.Caller.RequestID != "req-1" {
		t.Fatalf("status = %#v, want declined caller metadata", status)
	}
}

func actionDeps(db *database.DB, userID string) *Deps {
	return actionDepsWithConfirm(db, userID, agent.NewConfirmationManager(time.Minute))
}

func actionDepsWithConfirm(db *database.DB, userID string, confirm *agent.ConfirmationManager) *Deps {
	return &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: confirm,
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity", "update_activity_title"},
		}},
	}
}

func assertAuditDetailContains(t *testing.T, db *database.DB, userID, actionID, needle string) {
	t.Helper()
	id, err := parseActionID(actionID)
	if err != nil {
		t.Fatalf("parse action id: %v", err)
	}
	err = db.WithUserTx(context.Background(), userID, func(tx pgx.Tx) error {
		var detail string
		if err := tx.QueryRow(context.Background(), `SELECT COALESCE(result_detail::text, '') FROM agent_audit_log WHERE id = $1`, id).Scan(&detail); err != nil {
			return err
		}
		if !strings.Contains(detail, needle) {
			t.Fatalf("audit detail = %s, want containing %q", detail, needle)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read audit detail: %v", err)
	}
}
