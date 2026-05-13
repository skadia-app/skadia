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
	if prepared.ActionID == "" || prepared.Status != ActionStatusPending || prepared.ConfirmationURL != "" {
		t.Fatalf("prepared action = %#v", prepared)
	}
	preview, ok := prepared.Preview.(map[string]any)
	if !ok || preview["tool"] != "update_activity_title" {
		t.Fatalf("preview = %#v, want tool metadata", prepared.Preview)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusPending || status.Caller.Transport != "local_mcp" || status.Caller.HostApprovalID != "host-ok" {
		t.Fatalf("status = %#v, want pending caller metadata", status)
	}
}

func TestPrepareActionPreviewRedactsSecrets(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-secret-preview",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{"title":"Morning","api_key":"secret","nested":{"access_token":"token"}}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	raw, err := json.Marshal(prepared.Preview)
	if err != nil {
		t.Fatalf("Marshal preview: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, `"api_key":"secret"`) || strings.Contains(got, `"access_token":"token"`) {
		t.Fatalf("preview leaked sensitive value: %s", got)
	}
	if !strings.Contains(got, "Morning") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("preview = %s, want visible safe field and redaction", got)
	}
}

func TestPrepareActionAuditArgsRedactSecrets(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-secret-audit",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{"title":"Morning","api_key":"secret","nested":{"access_token":"token"}}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	id, err := parseActionID(prepared.ActionID)
	if err != nil {
		t.Fatalf("parse action id: %v", err)
	}
	err = db.WithUserTx(context.Background(), userID, func(tx pgx.Tx) error {
		var argsText string
		if err := tx.QueryRow(context.Background(), `SELECT args::text FROM agent_audit_log WHERE id = $1`, id).Scan(&argsText); err != nil {
			return err
		}
		if strings.Contains(argsText, `"secret"`) || strings.Contains(argsText, `:"token"`) {
			t.Fatalf("audit args leaked secret: %s", argsText)
		}
		if !strings.Contains(argsText, "[redacted]") || !strings.Contains(argsText, "Morning") {
			t.Fatalf("audit args = %s, want redacted secret and safe title", argsText)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read audit args: %v", err)
	}
}

func TestExecutePreviewAuditRedactsArgsAndDetail(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "dry_run_secret_preview",
		Description: "Preview secret",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"api_key":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		Permission:  PermissionRead,
		AuditMode:   AuditPreview,
		ExecutePreview: func(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return map[string]any{"status": "ok", "access_token": "result-token"}, nil
		},
	})
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, &Deps{DB: db, Audit: agent.NewAuditWriter(db)})
	req := ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-preview-redaction",
		ToolName:   "dry_run_secret_preview",
		Args:       json.RawMessage(`{"id":"a1","api_key":"secret"}`),
	}
	result, err := envelope.ExecutePreview(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecutePreview: %v", err)
	}
	if got := result.(map[string]any)["access_token"]; got != "[redacted]" {
		t.Fatalf("preview result access_token = %#v, want redacted", got)
	}
	err = db.WithUserTx(context.Background(), userID, func(tx pgx.Tx) error {
		var argsText, detailText string
		if err := tx.QueryRow(context.Background(), `
			SELECT args::text, COALESCE(result_detail::text, '')
			FROM agent_audit_log
			WHERE user_id = $1 AND session_id = $2 AND tool_call_id = $3
		`, userID, req.SessionID, req.ToolCallID).Scan(&argsText, &detailText); err != nil {
			return err
		}
		for _, text := range []string{argsText, detailText} {
			if strings.Contains(text, `"secret"`) || strings.Contains(text, `"result-token"`) {
				t.Fatalf("preview audit leaked sensitive value: args=%s detail=%s", argsText, detailText)
			}
		}
		if !strings.Contains(argsText, "[redacted]") || !strings.Contains(detailText, "[redacted]") {
			t.Fatalf("preview audit missing redaction: args=%s detail=%s", argsText, detailText)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read audit row: %v", err)
	}
}

func TestPrepareActionDuplicatePendingIsIdempotent(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDepsWithConfirm(db, userID, confirm))
	req := ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-duplicate-prepare",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	}
	first, err := envelope.PrepareAction(context.Background(), req)
	if err != nil {
		t.Fatalf("first PrepareAction: %v", err)
	}
	second, err := envelope.PrepareAction(context.Background(), req)
	if err != nil {
		t.Fatalf("second PrepareAction: %v", err)
	}
	if first.ActionID != second.ActionID || second.Status != ActionStatusPending || second.Response == nil {
		t.Fatalf("duplicate prepare = %#v, first = %#v", second, first)
	}
	status, err := envelope.GetActionStatus(context.Background(), first.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusPending {
		t.Fatalf("status = %s, want pending", status.Status)
	}
}

func TestPrepareActionDuplicatePendingRejectsDifferentArgs(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))
	req := ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-duplicate-prepare-mismatch",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	}
	if _, err := envelope.PrepareAction(context.Background(), req); err != nil {
		t.Fatalf("first PrepareAction: %v", err)
	}
	req.Args = json.RawMessage(`{"id":"a2"}`)
	if _, err := envelope.PrepareAction(context.Background(), req); err == nil || !strings.Contains(err.Error(), "different arguments") {
		t.Fatalf("second PrepareAction error = %v, want argument mismatch", err)
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

func TestExecuteConfirmedActionPreflightFailureCompletesAuditAndCancelsConfirmation(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDepsWithConfirm(db, userID, confirm))
	sessionID := uuid.NewString()
	toolCallID := "call-confirm-preflight-error"
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  sessionID,
		ToolCallID: toolCallID,
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if !confirm.Resolve(sessionID, toolCallID, agent.ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	envelope.Deps.Preferences = &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{},
	}}
	if _, err := envelope.ExecuteConfirmedAction(context.Background(), prepared.ActionID, userID); err == nil {
		t.Fatal("ExecuteConfirmedAction succeeded after write tool was disallowed")
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusError {
		t.Fatalf("status = %#v, want error", status)
	}
	if _, _, ok := confirm.ConsumeConfirmedAction(prepared.ActionID); ok {
		t.Fatal("confirmation remained consumable after preflight failure")
	}
}

func TestRecordDeclinedActionDoesNotOverwriteExecutedAction(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDepsWithConfirm(db, userID, confirm))
	sessionID := uuid.NewString()
	toolCallID := "call-decline-after-execute"
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  sessionID,
		ToolCallID: toolCallID,
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if !confirm.Resolve(sessionID, toolCallID, agent.ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	if _, err := envelope.ExecuteConfirmedAction(context.Background(), prepared.ActionID, userID); err != nil {
		t.Fatalf("ExecuteConfirmedAction: %v", err)
	}
	if err := envelope.RecordDeclinedAction(context.Background(), prepared.ActionID, userID, "declined"); err != nil {
		t.Fatalf("RecordDeclinedAction: %v", err)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusExecuted {
		t.Fatalf("status = %#v, want executed", status)
	}
}

func TestRecordDeclinedActionCancelsPendingConfirmation(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDepsWithConfirm(db, userID, confirm))
	sessionID := uuid.NewString()
	toolCallID := "call-decline-cancel"
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  sessionID,
		ToolCallID: toolCallID,
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := envelope.RecordDeclinedAction(context.Background(), prepared.ActionID, userID, "cancelled"); err != nil {
		t.Fatalf("RecordDeclinedAction: %v", err)
	}
	if confirm.Resolve(sessionID, toolCallID, agent.ConfirmResponse{Confirmed: true, Source: "late-click"}) {
		t.Fatal("late confirmation resolved after cancellation")
	}
}

func TestGetActionStatusExpiresAbandonedPendingAuditRow(t *testing.T) {
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(exportTestRegistry(), actionDeps(db, userID))
	prepared, err := envelope.PrepareAction(context.Background(), ConfirmableActionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-abandoned-expiry",
		ToolName:   "update_activity",
		Args:       json.RawMessage(`{}`),
		Caller:     CallerContext{Transport: "local_mcp"},
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	envelope.Deps.Confirm = agent.NewConfirmationManager(time.Minute)
	id, err := parseActionID(prepared.ActionID)
	if err != nil {
		t.Fatalf("parse action id: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE agent_audit_log SET requested_at = now() - interval '6 minutes' WHERE id = $1`, id); err != nil {
		t.Fatalf("age audit row: %v", err)
	}
	status, err := envelope.GetActionStatus(context.Background(), prepared.ActionID, userID)
	if err != nil {
		t.Fatalf("GetActionStatus: %v", err)
	}
	if status.Status != ActionStatusTimeout {
		t.Fatalf("status = %#v, want timeout", status)
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
