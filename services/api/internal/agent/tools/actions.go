package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	preparedPayload, err := e.prepareWritePayload(ctx, tool, req)
	if err != nil {
		return nil, err
	}
	actionReq := req
	actionReq.Args = preparedPayload.Args
	claimed, existing, err := e.audit().ClaimPending(ctx, auditRowFromAction(actionReq, tool.Name))
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("pending audit row missing")
	}
	actionID := formatActionID(existing.ID)
	if !claimed {
		if existing.ToolName != tool.Name || !jsonEqualRaw(existing.Args, preparedPayload.Args) {
			return nil, fmt.Errorf("audit slot already exists with different arguments or tool")
		}
		if existing.ResultStatus == string(ActionStatusPending) {
			if e.Deps == nil || e.Deps.Confirm == nil {
				return nil, fmt.Errorf("pending action exists without live confirmation")
			}
			pending, ok := e.Deps.Confirm.PendingAction(actionID)
			if !ok {
				return nil, fmt.Errorf("pending action exists without live confirmation")
			}
			if !jsonEqualRaw(pending.Args, preparedPayload.Args) {
				return nil, fmt.Errorf("pending action already exists with different arguments")
			}
			response, ok := e.Deps.Confirm.PendingActionResponse(actionID)
			if !ok {
				return nil, fmt.Errorf("pending action exists without response channel")
			}
			return &PreparedAction{
				ActionID:  actionID,
				ToolName:  tool.Name,
				Summary:   pending.Summary,
				Preview:   previewFromPreparedOrArgs(preparedPayload.Preview, tool.Name, pending.Args),
				ExpiresAt: pending.ExpiresAt,
				Status:    ActionStatusPending,
				Caller:    callerFromDetail(pending.Metadata),
				Response:  response,
			}, nil
		}
		return &PreparedAction{
			ActionID: actionID,
			ToolName: tool.Name,
			Summary:  preparedPayload.Summary,
			Preview:  preparedPayload.Preview,
			Status:   ActionStatus(existing.ResultStatus),
			Caller:   callerFromDetail(existing.ResultDetail),
		}, nil
	}
	if e.Deps == nil || e.Deps.Confirm == nil {
		_ = e.audit().CompletePending(ctx, req.UserID, existing.ID, string(ActionStatusError), actionDetailJSON(req.Caller, "", "confirmation manager is not configured", nil))
		return nil, fmt.Errorf("confirmation manager is not configured")
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	response, err := e.Deps.Confirm.Register(ctx, &agent.PendingConfirm{
		UserID:     req.UserID,
		ActionID:   actionID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		Tool:       tool.Name,
		Args:       preparedPayload.Args,
		Summary:    preparedPayload.Summary,
		ExpiresAt:  expiresAt,
		Metadata:   actionDetailJSON(req.Caller, "", "", nil),
	})
	if err != nil {
		_ = e.audit().CompletePending(ctx, req.UserID, existing.ID, string(ActionStatusError), actionDetailJSON(req.Caller, "", "confirmation registration failed", nil))
		return nil, err
	}
	return &PreparedAction{
		ActionID:  actionID,
		ToolName:  tool.Name,
		Summary:   preparedPayload.Summary,
		Preview:   preparedPayload.Preview,
		ExpiresAt: expiresAt,
		Status:    ActionStatusPending,
		Caller:    req.Caller,
		Response:  response,
	}, nil
}

func auditRowFromAction(req ConfirmableActionRequest, toolName string) database.AgentAuditRow {
	return database.AgentAuditRow{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   toolName,
		Args:       auditArgsJSON(req.Args),
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
	confirm, args, ok := e.Deps.Confirm.PeekConfirmedAction(actionID)
	if !ok || !confirm.Confirmed {
		return nil, fmt.Errorf("Skadia confirmation is required")
	}
	if len(args) == 0 {
		args = row.Args
	}
	req := ToolExecutionRequest{
		UserID:     row.UserID,
		SessionID:  row.SessionID,
		ToolCallID: row.ToolCallID,
		ToolName:   row.ToolName,
		Args:       args,
		AuditMode:  AuditWrite,
	}
	return e.executeConfirmedAuditRow(ctx, actionID, req, row, callerFromConfirmOrDetail(confirm, row.ResultDetail))
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
	if status == ActionStatusPending && expiresAt.IsZero() && !row.RequestedAt.IsZero() && row.RequestedAt.Add(5*time.Minute).Before(time.Now()) {
		_ = e.RecordDeclinedAction(ctx, actionID, userID, "timeout")
		status = ActionStatusTimeout
		expiresAt = row.RequestedAt.Add(5 * time.Minute)
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
	err = e.audit().CompletePending(ctx, userID, row.ID, string(status), actionDetailJSON(caller, reason, "", nil))
	if err == nil && e != nil && e.Deps != nil && e.Deps.Confirm != nil {
		e.Deps.Confirm.CancelAction(actionID)
	}
	return err
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

func (e *ExecutionEnvelope) executeConfirmedAuditRow(ctx context.Context, actionID string, req ToolExecutionRequest, row *database.AgentAuditRow, caller CallerContext) (any, error) {
	tool, err := e.writeTool(row.ToolName)
	if err != nil {
		_ = e.completePendingActionError(ctx, row, actionID, caller, err)
		return nil, err
	}
	if err := e.writeAllowed(ctx, row.UserID, tool.Name); err != nil {
		_ = e.completePendingActionError(ctx, row, actionID, caller, err)
		return nil, err
	}
	if err := validateArgs(tool.Name, tool.InputSchema, req.Args); err != nil {
		_ = e.completePendingActionError(ctx, row, actionID, caller, err)
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
	if e == nil || e.Deps == nil || e.Deps.Confirm == nil {
		return nil, fmt.Errorf("confirmation manager is not configured")
	}
	if _, _, ok := e.Deps.Confirm.ConsumeConfirmedAction(actionID); !ok {
		return nil, fmt.Errorf("Skadia confirmation is required")
	}
	var afterCommit []func(context.Context)
	result, execErr := tool.ExecuteWrite(ctx, &WriteExecutionDeps{Deps: e.Deps, Tx: tx, AfterCommit: &afterCommit}, row.UserID, req.Args)
	if execErr != nil {
		_ = tx.Rollback(ctx)
		_ = e.audit().CompletePending(ctx, row.UserID, row.ID, string(ActionStatusError), actionDetailJSON(caller, "", execErr.Error(), nil))
		return nil, execErr
	}
	if err := e.Deps.DB.CompletePendingAudit(ctx, tx, row.ID, string(ActionStatusExecuted), actionDetailJSON(caller, "", "", result)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, hook := range afterCommit {
		if hook != nil {
			go hook(context.Background())
		}
	}
	return result, nil
}

func (e *ExecutionEnvelope) completePendingActionError(ctx context.Context, row *database.AgentAuditRow, actionID string, caller CallerContext, err error) error {
	if row == nil {
		return nil
	}
	completeErr := e.audit().CompletePending(ctx, row.UserID, row.ID, string(ActionStatusError), actionDetailJSON(caller, "", err.Error(), nil))
	if e != nil && e.Deps != nil && e.Deps.Confirm != nil {
		e.Deps.Confirm.CancelAction(actionID)
	}
	return completeErr
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

func (e *ExecutionEnvelope) prepareWritePayload(ctx context.Context, tool *Tool, req ConfirmableActionRequest) (*PreparedWrite, error) {
	out := &PreparedWrite{Args: append(json.RawMessage(nil), req.Args...)}
	if tool.PrepareWrite != nil {
		prepared, err := tool.PrepareWrite(ctx, e.Deps, req.UserID, req.Args)
		if err != nil {
			return nil, err
		}
		if prepared != nil {
			if len(prepared.Args) > 0 {
				out.Args = append(json.RawMessage(nil), prepared.Args...)
				if err := validateArgs(tool.Name, tool.InputSchema, out.Args); err != nil {
					return nil, err
				}
			}
			out.Summary = prepared.Summary
			out.Preview = prepared.Preview
		}
	}
	if len(out.Args) == 0 {
		out.Args = json.RawMessage(`{}`)
	}
	if out.Summary == "" {
		summaryReq := req
		summaryReq.Args = out.Args
		summary, err := e.actionSummary(ctx, tool, summaryReq)
		if err != nil {
			return nil, err
		}
		out.Summary = summary
	}
	if out.Preview == nil {
		out.Preview = actionPreview(tool.Name, out.Args)
	}
	return out, nil
}

func previewFromPreparedOrArgs(preview any, toolName string, args json.RawMessage) any {
	if preview != nil {
		return preview
	}
	return actionPreview(toolName, args)
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
	raw, _ := json.Marshal(actionDetail{
		Caller: caller,
		Reason: reason,
		Error:  sanitizeAuditError(errText),
		Result: sanitizeAuditValue(result),
	})
	return raw
}

func auditArgsJSON(args json.RawMessage) json.RawMessage {
	var decoded any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &decoded)
	}
	raw, _ := json.Marshal(sanitizeAuditValue(decoded))
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func actionPreview(toolName string, args json.RawMessage) map[string]any {
	var decoded any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &decoded)
	}
	return map[string]any{
		"tool":      toolName,
		"arguments": sanitizeAuditValue(decoded),
	}
}

func sanitizeAuditValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if sensitivePreviewKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = sanitizeAuditValue(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = sanitizeAuditValue(child)
		}
		return out
	default:
		return value
	}
}

func sanitizeAuditError(errText string) string {
	if errText == "" {
		return ""
	}
	return "operation failed"
}

func sensitivePreviewKey(key string) bool {
	key = strings.ToLower(key)
	for _, needle := range []string{"api_key", "apikey", "password", "secret", "token"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func jsonEqualRaw(a, b json.RawMessage) bool {
	var left any
	var right any
	if len(a) == 0 {
		a = json.RawMessage(`{}`)
	}
	if len(b) == 0 {
		b = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(a, &left); err != nil {
		return string(a) == string(b)
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
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

func callerFromConfirmOrDetail(resp agent.ConfirmResponse, detail json.RawMessage) CallerContext {
	if len(resp.Metadata) > 0 {
		return callerFromDetail(resp.Metadata)
	}
	return callerFromDetail(detail)
}
