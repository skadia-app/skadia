package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type ToolExecutionRequest struct {
	UserID     string
	SessionID  string
	ToolCallID string
	ToolName   string
	Args       json.RawMessage
	Confirmed  bool
	AuditMode  AuditMode
}

type PreparedConfirmation struct {
	Summary   string
	ExpiresAt time.Time
	Cached    *json.RawMessage
	Response  <-chan agent.ConfirmResponse
}

type ExecutionEnvelope struct {
	Registry *Registry
	Deps     *Deps
}

func NewExecutionEnvelope(reg *Registry, deps *Deps) *ExecutionEnvelope {
	return &ExecutionEnvelope{Registry: reg, Deps: deps}
}

func (e *ExecutionEnvelope) PrepareConfirmation(ctx context.Context, req ToolExecutionRequest) (*PreparedConfirmation, error) {
	tool, err := e.writeTool(req.ToolName)
	if err != nil {
		return nil, err
	}
	if err := e.writeAllowed(ctx, req.UserID, tool.Name); err != nil {
		return nil, err
	}
	if err := validateArgs(tool.Name, tool.InputSchema, req.Args); err != nil {
		return nil, err
	}
	summary := tool.Description
	if tool.SummaryForUser != nil {
		got, err := tool.SummaryForUser(ctx, e.Deps, req.UserID, req.Args)
		if err != nil {
			return nil, err
		}
		if got != "" {
			summary = got
		}
	}
	claimed, existing, err := e.audit().ClaimPending(ctx, database.AgentAuditRow{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   tool.Name,
		Args:       req.Args,
	})
	if err != nil {
		return nil, err
	}
	if !claimed && existing != nil && existing.ResultStatus != "pending" {
		cached := append(json.RawMessage(nil), existing.ResultDetail...)
		return &PreparedConfirmation{Cached: &cached}, nil
	}
	if e.Deps == nil || e.Deps.Confirm == nil {
		if existing != nil {
			_ = e.audit().Complete(ctx, req.UserID, existing.ID, "error", json.RawMessage(`{"error":"confirmation manager is not configured"}`))
		}
		return nil, fmt.Errorf("confirmation manager is not configured")
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	response, err := e.Deps.Confirm.Register(ctx, &agent.PendingConfirm{
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		Tool:       tool.Name,
		Args:       req.Args,
		Summary:    summary,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		if existing != nil {
			_ = e.audit().Complete(ctx, req.UserID, existing.ID, "error", json.RawMessage(`{"error":"confirmation registration failed"}`))
		}
		return nil, err
	}
	return &PreparedConfirmation{Summary: summary, ExpiresAt: expiresAt, Response: response}, nil
}

func (e *ExecutionEnvelope) ExecutePreview(ctx context.Context, req ToolExecutionRequest) (any, error) {
	tool, ok := e.Registry.Get(req.ToolName)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", req.ToolName)
	}
	if tool.AuditMode != AuditPreview || tool.ExecutePreview == nil {
		return nil, fmt.Errorf("tool is not a preview tool: %s", req.ToolName)
	}
	if err := validateArgs(tool.Name, tool.InputSchema, req.Args); err != nil {
		return nil, err
	}
	detail, _, err := e.audit().WithAuditSlot(ctx, database.AgentAuditRow{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   tool.Name,
		Args:       req.Args,
	}, func(ctx context.Context, tx pgx.Tx) (json.RawMessage, error) {
		result, err := tool.ExecutePreview(ctx, &PreviewExecutionDeps{
			Readonly: e.Deps.Readonly,
			SQLAgent: e.Deps.SQLAgent,
		}, req.UserID, req.Args)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return nil, err
	}
	var out any
	if len(detail) > 0 {
		if err := json.Unmarshal(detail, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (e *ExecutionEnvelope) ExecuteConfirmed(ctx context.Context, req ToolExecutionRequest) (any, error) {
	tool, err := e.writeTool(req.ToolName)
	if err != nil {
		return nil, err
	}
	if err := e.writeAllowed(ctx, req.UserID, tool.Name); err != nil {
		return nil, err
	}
	if err := validateArgs(tool.Name, tool.InputSchema, req.Args); err != nil {
		return nil, err
	}
	tx, auditRow, err := e.beginPendingAuditTx(ctx, req)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return decodeAuditResult(auditRow)
	}
	defer tx.Rollback(ctx)

	result, execErr := tool.ExecuteWrite(ctx, &WriteExecutionDeps{Deps: e.Deps, Tx: tx}, req.UserID, req.Args)
	if execErr != nil {
		_ = tx.Rollback(ctx)
		detail, _ := json.Marshal(map[string]string{"error": execErr.Error()})
		_ = e.audit().Complete(ctx, req.UserID, auditRow.ID, "error", detail)
		return nil, execErr
	}
	detail, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if err := e.Deps.DB.CompleteAudit(ctx, tx, auditRow.ID, "executed", detail); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *ExecutionEnvelope) RecordDeclined(ctx context.Context, req ToolExecutionRequest, reason string) error {
	status := "declined"
	if reason == "timeout" {
		status = "timeout"
	}
	tx, auditRow, err := e.beginPendingAuditTx(ctx, req)
	if err != nil {
		return err
	}
	if tx == nil {
		return nil
	}
	defer tx.Rollback(ctx)
	detail, _ := json.Marshal(map[string]string{"reason": reason})
	if err := e.Deps.DB.CompleteAudit(ctx, tx, auditRow.ID, status, detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e *ExecutionEnvelope) writeTool(name string) (*Tool, error) {
	if e == nil || e.Registry == nil {
		return nil, fmt.Errorf("registry is not configured")
	}
	tool, ok := e.Registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	if tool.Permission == PermissionRead {
		return nil, fmt.Errorf("tool is not a write tool: %s", name)
	}
	if tool.ExecuteWrite == nil {
		return nil, fmt.Errorf("write tool has no executor: %s", name)
	}
	return tool, nil
}

func (e *ExecutionEnvelope) writeAllowed(ctx context.Context, userID, toolName string) error {
	if e == nil || e.Deps == nil || e.Deps.Preferences == nil {
		return fmt.Errorf("write tools are disabled")
	}
	prefs, err := e.Deps.Preferences.LoadChatPreferences(ctx, userID)
	if err != nil {
		return err
	}
	if prefs == nil || !prefs.ChatWriteToolsEnabled {
		return fmt.Errorf("write tools are disabled")
	}
	for _, allowed := range prefs.ChatAllowedWriteTools {
		if allowed == toolName {
			return nil
		}
	}
	return fmt.Errorf("write tool is not allowed: %s", toolName)
}

func (e *ExecutionEnvelope) audit() *agent.AuditWriter {
	if e == nil || e.Deps == nil {
		return nil
	}
	return e.Deps.Audit
}

func (e *ExecutionEnvelope) beginPendingAuditTx(ctx context.Context, req ToolExecutionRequest) (pgx.Tx, *database.AgentAuditRow, error) {
	if e == nil || e.Deps == nil || e.Deps.DB == nil || e.Deps.DB.Pool == nil {
		return nil, nil, fmt.Errorf("database is not configured")
	}
	tx, err := e.Deps.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", req.UserID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", req.UserID+":"+req.SessionID+":"+req.ToolCallID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	row, err := loadAuditSlot(ctx, tx, req.UserID, req.SessionID, req.ToolCallID)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	if row.ResultStatus != "pending" {
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return nil, nil, err
		}
		return nil, row, nil
	}
	return tx, row, nil
}

func decodeAuditResult(row *database.AgentAuditRow) (any, error) {
	if row == nil {
		return nil, fmt.Errorf("audit row is missing")
	}
	if row.ResultStatus == "error" {
		return nil, fmt.Errorf("cached write tool error: %s", string(row.ResultDetail))
	}
	var out any
	if len(row.ResultDetail) > 0 {
		if err := json.Unmarshal(row.ResultDetail, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func loadAuditSlot(ctx context.Context, tx pgx.Tx, userID, sessionID, toolCallID string) (*database.AgentAuditRow, error) {
	var row database.AgentAuditRow
	err := tx.QueryRow(ctx, `
		SELECT id, user_id::text, session_id::text, tool_call_id, tool_name, args, result_status,
		       result_detail, requested_at, completed_at
		FROM agent_audit_log
		WHERE user_id = $1 AND session_id = $2 AND tool_call_id = $3
	`, userID, sessionID, toolCallID).Scan(
		&row.ID,
		&row.UserID,
		&row.SessionID,
		&row.ToolCallID,
		&row.ToolName,
		&row.Args,
		&row.ResultStatus,
		&row.ResultDetail,
		&row.RequestedAt,
		&row.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("pending audit row not found")
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func validateArgs(toolName string, schema json.RawMessage, args json.RawMessage) error {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := validateSchema(toolName, schema); err != nil {
		return err
	}
	var schemaDoc any
	if err := json.Unmarshal(schema, &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	url := fmt.Sprintf("agent/tools/%s.args.schema.json", toolName)
	if err := compiler.AddResource(url, schemaDoc); err != nil {
		return err
	}
	compiled, err := compiler.Compile(url)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(args))
	if err != nil {
		return err
	}
	return compiled.Validate(instance)
}
