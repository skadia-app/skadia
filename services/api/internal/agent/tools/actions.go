package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

type ActionStatus string

const (
	ActionStatusPending  ActionStatus = "pending"
	ActionStatusExecuted ActionStatus = "executed"
	ActionStatusDeclined ActionStatus = "declined"
	ActionStatusTimeout  ActionStatus = "timeout"
	ActionStatusError    ActionStatus = "error"
)

type CallerContext struct {
	Transport      string   `json:"transport,omitempty"`
	ClientID       string   `json:"client_id,omitempty"`
	AuthSubject    string   `json:"auth_subject,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	RequestID      string   `json:"request_id,omitempty"`
	HostApprovalID string   `json:"host_approval_id,omitempty"`
}

type ConfirmableActionRequest struct {
	UserID     string
	SessionID  string
	ToolCallID string
	ToolName   string
	Args       json.RawMessage
	Caller     CallerContext
}

type PreparedAction struct {
	ActionID        string
	ToolName        string
	Summary         string
	Preview         any
	ConfirmationURL string
	ExpiresAt       time.Time
	Status          ActionStatus
	Caller          CallerContext
	Response        <-chan agent.ConfirmResponse
}

type ActionStatusResult struct {
	ActionID  string
	ToolName  string
	Status    ActionStatus
	Detail    json.RawMessage
	ExpiresAt time.Time
	Caller    CallerContext
}

type actionDetail struct {
	Caller CallerContext `json:"caller,omitempty"`
	Reason string        `json:"reason,omitempty"`
	Error  string        `json:"error,omitempty"`
	Result any           `json:"result,omitempty"`
}

func (e *ExecutionEnvelope) PrepareAction(ctx context.Context, req ConfirmableActionRequest) (*PreparedAction, error) {
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
	summary, err := e.actionSummary(ctx, tool, req)
	if err != nil {
		return nil, err
	}
	claimed, existing, err := e.audit().ClaimPending(ctx, auditRowFromAction(req, tool.Name))
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("pending audit row missing")
	}
	actionID := formatActionID(existing.ID)
	if !claimed && existing.ResultStatus != string(ActionStatusPending) {
		return &PreparedAction{
			ActionID: actionID,
			ToolName: tool.Name,
			Summary:  summary,
			Status:   ActionStatus(existing.ResultStatus),
			Caller:   callerFromDetail(existing.ResultDetail),
		}, nil
	}
	if e.Deps == nil || e.Deps.Confirm == nil {
		_ = e.audit().Complete(ctx, req.UserID, existing.ID, string(ActionStatusError), actionDetailJSON(req.Caller, "", "confirmation manager is not configured", nil))
		return nil, fmt.Errorf("confirmation manager is not configured")
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	response, err := e.Deps.Confirm.Register(ctx, &agent.PendingConfirm{
		UserID:     req.UserID,
		ActionID:   actionID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		Tool:       tool.Name,
		Args:       req.Args,
		Summary:    summary,
		ExpiresAt:  expiresAt,
		Metadata:   actionDetailJSON(req.Caller, "", "", nil),
	})
	if err != nil {
		_ = e.audit().Complete(ctx, req.UserID, existing.ID, string(ActionStatusError), actionDetailJSON(req.Caller, "", "confirmation registration failed", nil))
		return nil, err
	}
	return &PreparedAction{
		ActionID:        actionID,
		ToolName:        tool.Name,
		Summary:         summary,
		ConfirmationURL: "/agent/actions/" + actionID,
		ExpiresAt:       expiresAt,
		Status:          ActionStatusPending,
		Caller:          req.Caller,
		Response:        response,
	}, nil
}

func auditRowFromAction(req ConfirmableActionRequest, toolName string) database.AgentAuditRow {
	return database.AgentAuditRow{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   toolName,
		Args:       req.Args,
	}
}

func (e *ExecutionEnvelope) ExecuteConfirmedAction(ctx context.Context, actionID, userID string) (any, error) {
	id, err := parseActionID(actionID)
	if err != nil {
		return nil, err
	}
	row, err := e.loadActionAuditRow(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if row.ResultStatus != string(ActionStatusPending) {
		return decodeAuditResult(row)
	}
	if e == nil || e.Deps == nil || e.Deps.Confirm == nil {
		return nil, fmt.Errorf("confirmation manager is not configured")
	}
	confirm, ok := e.Deps.Confirm.ConfirmedAction(actionID)
	if !ok || !confirm.Confirmed {
		return nil, fmt.Errorf("Skadia confirmation is required")
	}
	req := ToolExecutionRequest{
		UserID:     row.UserID,
		SessionID:  row.SessionID,
		ToolCallID: row.ToolCallID,
		ToolName:   row.ToolName,
		Args:       row.Args,
		AuditMode:  AuditWrite,
	}
	if len(confirm.ArgsOverrides) > 0 && strings.TrimSpace(string(confirm.ArgsOverrides)) != "null" {
		req.Args = confirm.ArgsOverrides
	}
	return e.executeConfirmedAuditRow(ctx, req, row, callerFromPendingOrDetail(e.Deps.Confirm, actionID, row.ResultDetail))
}

func (e *ExecutionEnvelope) GetActionStatus(ctx context.Context, actionID, userID string) (*ActionStatusResult, error) {
	id, err := parseActionID(actionID)
	if err != nil {
		return nil, err
	}
	row, err := e.loadActionAuditRow(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	status := ActionStatus(row.ResultStatus)
	caller := callerFromPendingOrDetail(nil, "", row.ResultDetail)
	expiresAt := time.Time{}
	if e != nil && e.Deps != nil && e.Deps.Confirm != nil {
		if pending, ok := e.Deps.Confirm.PendingAction(actionID); ok {
			caller = callerFromDetail(pending.Metadata)
			expiresAt = pending.ExpiresAt
			if status == ActionStatusPending && !pending.ExpiresAt.IsZero() && !pending.ExpiresAt.After(time.Now()) {
				_ = e.RecordDeclinedAction(ctx, actionID, userID, "timeout")
				status = ActionStatusTimeout
			}
		}
	}
	return &ActionStatusResult{
		ActionID:  actionID,
		ToolName:  row.ToolName,
		Status:    status,
		Detail:    row.ResultDetail,
		ExpiresAt: expiresAt,
		Caller:    caller,
	}, nil
}

func (e *ExecutionEnvelope) RecordDeclinedAction(ctx context.Context, actionID, userID, reason string) error {
	id, err := parseActionID(actionID)
	if err != nil {
		return err
	}
	row, err := e.loadActionAuditRow(ctx, id, userID)
	if err != nil {
		return err
	}
	if row.ResultStatus != string(ActionStatusPending) {
		return nil
	}
	caller := callerFromPendingOrDetail(nil, "", row.ResultDetail)
	if e != nil && e.Deps != nil && e.Deps.Confirm != nil {
		caller = callerFromPendingOrDetail(e.Deps.Confirm, actionID, row.ResultDetail)
	}
	status := ActionStatusDeclined
	if reason == string(ActionStatusTimeout) || reason == "timeout" {
		status = ActionStatusTimeout
	}
	return e.audit().Complete(ctx, userID, row.ID, string(status), actionDetailJSON(caller, reason, "", nil))
}

func (e *ExecutionEnvelope) loadActionAuditRow(ctx context.Context, id int64, userID string) (*database.AgentAuditRow, error) {
	if e == nil || e.Deps == nil || e.Deps.DB == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	var out *database.AgentAuditRow
	err := e.Deps.DB.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		row, err := scanActionAuditRow(tx.QueryRow(ctx, `
			SELECT id, user_id::text, session_id::text, tool_call_id, tool_name, args, result_status,
			       result_detail, requested_at, completed_at
			FROM agent_audit_log
			WHERE id = $1 AND user_id = $2
		`, id, userID))
		if err != nil {
			return err
		}
		out = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scanActionAuditRow(row pgx.Row) (*database.AgentAuditRow, error) {
	var r database.AgentAuditRow
	if err := row.Scan(
		&r.ID,
		&r.UserID,
		&r.SessionID,
		&r.ToolCallID,
		&r.ToolName,
		&r.Args,
		&r.ResultStatus,
		&r.ResultDetail,
		&r.RequestedAt,
		&r.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func (e *ExecutionEnvelope) executeConfirmedAuditRow(ctx context.Context, req ToolExecutionRequest, row *database.AgentAuditRow, caller CallerContext) (any, error) {
	tool, err := e.writeTool(row.ToolName)
	if err != nil {
		return nil, err
	}
	if err := e.writeAllowed(ctx, row.UserID, tool.Name); err != nil {
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
	result, execErr := tool.ExecuteWrite(ctx, &WriteExecutionDeps{Deps: e.Deps, Tx: tx}, row.UserID, req.Args)
	if execErr != nil {
		_ = tx.Rollback(ctx)
		_ = e.audit().Complete(ctx, row.UserID, row.ID, string(ActionStatusError), actionDetailJSON(caller, "", execErr.Error(), nil))
		return nil, execErr
	}
	if err := e.Deps.DB.CompleteAudit(ctx, tx, row.ID, string(ActionStatusExecuted), actionDetailJSON(caller, "", "", result)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *ExecutionEnvelope) actionSummary(ctx context.Context, tool *Tool, req ConfirmableActionRequest) (string, error) {
	summary := tool.Description
	if tool.SummaryForUser != nil {
		got, err := tool.SummaryForUser(ctx, e.Deps, req.UserID, req.Args)
		if err != nil {
			return "", err
		}
		if got != "" {
			summary = got
		}
	}
	return summary, nil
}

func formatActionID(id int64) string {
	return "audit:" + strconv.FormatInt(id, 10)
}

func parseActionID(actionID string) (int64, error) {
	raw, ok := strings.CutPrefix(actionID, "audit:")
	if !ok || raw == "" {
		return 0, fmt.Errorf("invalid action id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid action id")
	}
	return id, nil
}

func actionDetailJSON(caller CallerContext, reason, errText string, result any) json.RawMessage {
	raw, _ := json.Marshal(actionDetail{Caller: caller, Reason: reason, Error: errText, Result: result})
	return raw
}

func callerFromDetail(raw json.RawMessage) CallerContext {
	var detail actionDetail
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &detail)
	}
	return detail.Caller
}

func callerFromPendingOrDetail(confirm *agent.ConfirmationManager, actionID string, detail json.RawMessage) CallerContext {
	if confirm != nil {
		if pending, ok := confirm.PendingAction(actionID); ok {
			return callerFromDetail(pending.Metadata)
		}
		if resolved, ok := confirm.ResolvedAction(actionID); ok {
			return callerFromDetail(resolved.Metadata)
		}
	}
	return callerFromDetail(detail)
}
