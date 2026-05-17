package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/database"
)

type AgentToolRuntime struct {
	Registry *Registry
	Deps     *Deps
	Envelope *ExecutionEnvelope
	Limiter  *ToolQuotaLimiter
}

func NewAgentToolRuntime(reg *Registry, deps *Deps, envelope *ExecutionEnvelope) *AgentToolRuntime {
	if envelope == nil {
		envelope = NewExecutionEnvelope(reg, deps)
	}
	return &AgentToolRuntime{Registry: reg, Deps: deps, Envelope: envelope, Limiter: NewToolQuotaLimiter()}
}

func (r *AgentToolRuntime) AddEnabledWriteTools(filter []string, prefs *agent.ChatPreferences) []string {
	if r == nil || r.Registry == nil {
		return filter
	}
	return r.Registry.AddEnabledWriteTools(filter, prefs)
}

func (r *AgentToolRuntime) Definitions(filter []string, prefs *agent.ChatPreferences) []llmwire.ToolDefinition {
	if r == nil || r.Registry == nil {
		return nil
	}
	return r.Registry.Definitions(filter, prefs)
}

func (r *AgentToolRuntime) Execute(ctx context.Context, req agent.ToolRuntimeRequest, sink agent.SSESink) (agent.ToolRuntimeResult, error) {
	if r == nil || r.Registry == nil {
		return agent.ToolRuntimeResult{}, fmt.Errorf("tool registry is not configured")
	}
	tool, ok := r.Registry.Get(req.ToolName)
	if !ok {
		return agent.ToolRuntimeResult{}, fmt.Errorf("unknown tool: %s", req.ToolName)
	}
	if req.EnforceTools && !stringInList(req.AllowedTools, req.ToolName) {
		return agent.ToolRuntimeResult{}, fmt.Errorf("tool is not available for this turn: %s", req.ToolName)
	}
	if tool.Permission != PermissionRead && req.EnforceTools && !req.WriteIntent {
		return agent.ToolRuntimeResult{}, fmt.Errorf("write tool is not available for this turn: %s", req.ToolName)
	}

	switch {
	case tool.AuditMode == AuditPreview && tool.ExecutePreview != nil:
		result, err := r.executeGuardedRead(ctx, req, tool, req.Args, func(execCtx context.Context, args json.RawMessage) (any, error) {
			execReq := toExecutionRequest(req, tool.AuditMode)
			execReq.Args = args
			return r.Envelope.ExecutePreview(execCtx, execReq)
		})
		return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
	case tool.AuditMode == AuditRead && tool.Permission == PermissionRead:
		result, err := r.executeGuardedRead(ctx, req, tool, req.Args, func(execCtx context.Context, args json.RawMessage) (any, error) {
			execReq := req
			execReq.Args = args
			return r.executeAuditedRead(execCtx, execReq, tool)
		})
		return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
	case tool.Permission == PermissionRead:
		result, err := r.executeGuardedRead(ctx, req, tool, req.Args, func(execCtx context.Context, args json.RawMessage) (any, error) {
			return r.Registry.ExecuteRead(execCtx, r.Deps, req.UserID, req.ToolName, args)
		})
		return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
	default:
		return r.executeConfirmedWrite(ctx, req, sink, tool)
	}
}

func stringInList(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (r *AgentToolRuntime) executeGuardedRead(ctx context.Context, req agent.ToolRuntimeRequest, tool *Tool, args json.RawMessage, exec func(context.Context, json.RawMessage) (any, error)) (any, error) {
	cleanArgs := stripExternalReadConsentArgs(args)
	if consentResult, ok, err := r.requireExternalReadConsent(ctx, req, tool, cleanArgs, args); ok || err != nil {
		return consentResult, err
	}

	var quota *ToolQuotaState
	if tool.RateLimitPerHour > 0 {
		state := r.toolQuotaLimiter().Allow(req.UserID, tool.Name, tool.RateLimitPerHour)
		quota = &state
		if !state.Allowed {
			return map[string]any{
				"status":          "rate_limited",
				"blocking_reason": "rate_limited",
				"rate_limit":      state,
			}, nil
		}
	}

	result, err := r.executeReadWithTimeout(ctx, tool, cleanArgs, exec)
	if status := contextBlockStatus(ctx, err); status != "" {
		return map[string]any{
			"status":          status,
			"blocking_reason": status,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if quota != nil {
		return withToolQuotaState(result, *quota), nil
	}
	return result, nil
}

func (r *AgentToolRuntime) requireExternalReadConsent(ctx context.Context, req agent.ToolRuntimeRequest, tool *Tool, cleanArgs, originalArgs json.RawMessage) (any, bool, error) {
	if tool.ExternalRead == nil || tool.ExternalRead.Policy != ExternalReadPolicyManualConfirm {
		return nil, false, nil
	}
	manager := r.externalConsentManager()
	consentReq := agentExternalReadConsentRequest{
		UserID:    req.UserID,
		SessionID: req.SessionID,
		ToolName:  tool.Name,
		Purpose:   tool.ExternalRead.Purpose,
		TTL:       tool.ExternalRead.TTL,
		Args:      cleanArgs,
	}
	token := externalReadConsentToken(originalArgs)
	if token != "" && manager.Consume(ctx, consentReq, token) {
		return nil, false, nil
	}
	request, err := manager.Request(ctx, consentReq)
	if err != nil {
		return nil, true, err
	}
	return map[string]any{
		"status":                            "requires_user_confirmation",
		"external_status":                   "requires_user_confirmation",
		"blocking_reason":                   "requires_user_confirmation",
		"consent_request":                   request,
		"confirmed_followup_requires_token": true,
	}, true, nil
}

func (r *AgentToolRuntime) executeReadWithTimeout(ctx context.Context, tool *Tool, args json.RawMessage, exec func(context.Context, json.RawMessage) (any, error)) (any, error) {
	execCtx := ctx
	cancel := func() {}
	if tool.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, tool.Timeout)
	}
	defer cancel()

	type readResult struct {
		result any
		err    error
	}
	done := make(chan readResult, 1)
	go func() {
		result, err := exec(execCtx, args)
		done <- readResult{result: result, err: err}
	}()

	select {
	case out := <-done:
		return out.result, out.err
	case <-execCtx.Done():
		return nil, execCtx.Err()
	}
}

func contextBlockStatus(parent context.Context, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if parent != nil && errors.Is(parent.Err(), context.Canceled) {
			return "cancelled"
		}
		return "timed_out"
	}
	return ""
}

func (r *AgentToolRuntime) PublicToolCallArgs(toolName string, args json.RawMessage) (json.RawMessage, bool) {
	if r == nil || r.Registry == nil {
		return nil, false
	}
	tool, ok := r.Registry.Get(toolName)
	if !ok {
		return nil, false
	}
	if tool.Permission == PermissionRead {
		return args, true
	}
	return nil, false
}

func (r *AgentToolRuntime) executeAuditedRead(ctx context.Context, req agent.ToolRuntimeRequest, tool *Tool) (any, error) {
	if r == nil || r.Deps == nil || r.Deps.DB == nil || r.Deps.Audit == nil {
		return nil, fmt.Errorf("audited read tool requires audit storage")
	}
	args := req.Args
	var tables []string
	if tool.Name == "run_sql" {
		args, tables = runSQLAuditArgs(req.Args)
	}
	row, err := claimReadAuditSlot(ctx, r.Deps.Audit, databaseAgentAuditRow(req, tool.Name, args))
	if err != nil {
		return nil, err
	}
	result, execErr := r.Registry.ExecuteRead(ctx, r.Deps, req.UserID, req.ToolName, req.Args)
	status := "executed"
	if execErr != nil {
		status = "error"
	}
	detail := json.RawMessage(nil)
	if tool.Name == "run_sql" {
		detail = runSQLAuditDetail(tables, runSQLResultRowCount(result), execErr)
	}
	if err := completeReadAudit(ctx, r.Deps.DB, req.UserID, row.ID, status, detail); err != nil {
		return nil, err
	}
	return result, execErr
}

func (r *AgentToolRuntime) executeConfirmedWrite(ctx context.Context, req agent.ToolRuntimeRequest, sink agent.SSESink, tool *Tool) (agent.ToolRuntimeResult, error) {
	prepared, err := r.Envelope.PrepareAction(ctx, ConfirmableActionRequest{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   req.ToolName,
		Args:       req.Args,
		Caller:     CallerContext{Transport: "internal_chat"},
	})
	if err != nil {
		if strings.Contains(err.Error(), "TAG_NOT_FOUND") {
			return agent.ToolRuntimeResult{AskUser: true, Question: tagNotFoundQuestion(req.Args)}, nil
		}
		return agent.ToolRuntimeResult{}, err
	}
	if prepared.Status != ActionStatusPending {
		status, err := r.Envelope.GetActionStatus(ctx, prepared.ActionID, req.UserID)
		if err != nil {
			return agent.ToolRuntimeResult{}, err
		}
		var cached any
		_ = json.Unmarshal(status.Detail, &cached)
		return agent.ToolRuntimeResult{Result: cached, Summary: string(status.Detail)}, nil
	}

	expiresAt := prepared.ExpiresAt
	if err := sink.Send(agent.SSEEvent{
		Type:       agent.EventToolConfirmRequest,
		ID:         req.MessageID,
		ToolCallID: req.ToolCallID,
		Tool:       req.ToolName,
		Summary:    prepared.Summary,
		Preview:    prepared.Preview,
		ExpiresAt:  &expiresAt,
	}); err != nil {
		return agent.ToolRuntimeResult{}, err
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = r.Envelope.RecordDeclined(context.Background(), toExecutionRequest(req, AuditWrite), "cancelled")
			return agent.ToolRuntimeResult{}, ctx.Err()
		case <-ticker.C:
			_ = sink.WriteRaw(": waiting_for_confirmation\n\n")
		case resp := <-prepared.Response:
			if resp.Confirmed {
				if err := sink.Send(agent.SSEEvent{Type: agent.EventToolConfirmed, ID: req.MessageID, ToolCallID: req.ToolCallID, Tool: req.ToolName}); err != nil {
					return agent.ToolRuntimeResult{}, err
				}
				result, err := r.Envelope.ExecuteConfirmedAction(ctx, prepared.ActionID, req.UserID)
				return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
			}
			reason := resp.Source
			if reason == "" {
				reason = "declined"
			}
			if err := r.Envelope.RecordDeclinedAction(ctx, prepared.ActionID, req.UserID, reason); err != nil {
				return agent.ToolRuntimeResult{}, err
			}
			if err := sink.Send(agent.SSEEvent{Type: agent.EventToolDeclined, ID: req.MessageID, ToolCallID: req.ToolCallID, Tool: req.ToolName, Reason: reason}); err != nil {
				return agent.ToolRuntimeResult{}, err
			}
			return agent.ToolRuntimeResult{Result: map[string]string{"status": "USER_DECLINED", "reason": reason}, Summary: "USER_DECLINED: " + reason}, nil
		}
	}
}

func databaseAgentAuditRow(req agent.ToolRuntimeRequest, toolName string, args json.RawMessage) database.AgentAuditRow {
	return database.AgentAuditRow{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   toolName,
		Args:       args,
	}
}

func toExecutionRequest(req agent.ToolRuntimeRequest, mode AuditMode) ToolExecutionRequest {
	return ToolExecutionRequest{
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   req.ToolName,
		Args:       req.Args,
		AuditMode:  mode,
	}
}

func marshalToolResult(result any) string {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprint(result)
	}
	return string(raw)
}

func tagNotFoundQuestion(args json.RawMessage) string {
	var in struct {
		TagName string `json:"tag_name"`
	}
	_ = json.Unmarshal(args, &in)
	name := strings.TrimSpace(in.TagName)
	if name == "" {
		return "Der Tag existiert noch nicht. Soll ich ihn zuerst erstellen?"
	}
	return fmt.Sprintf("Der Tag %q existiert noch nicht. Soll ich ihn zuerst erstellen?", name)
}

type ToolQuotaState struct {
	Allowed      bool      `json:"allowed"`
	LimitPerHour int       `json:"limit_per_hour"`
	Remaining    int       `json:"remaining"`
	ResetAt      time.Time `json:"reset_at"`
}

type ToolQuotaLimiter struct {
	mu      sync.Mutex
	windows map[string]toolQuotaWindow
}

type toolQuotaWindow struct {
	count   int
	resetAt time.Time
}

func NewToolQuotaLimiter() *ToolQuotaLimiter {
	return &ToolQuotaLimiter{windows: make(map[string]toolQuotaWindow)}
}

func (l *ToolQuotaLimiter) Allow(userID, toolName string, limitPerHour int) ToolQuotaState {
	if limitPerHour <= 0 {
		return ToolQuotaState{Allowed: true}
	}
	if l == nil {
		l = NewToolQuotaLimiter()
	}
	now := time.Now().UTC()
	key := userID + "\x00" + toolName
	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.windows[key]
	if window.resetAt.IsZero() || !now.Before(window.resetAt) {
		window = toolQuotaWindow{resetAt: now.Add(time.Hour)}
	}
	if window.count >= limitPerHour {
		l.windows[key] = window
		return ToolQuotaState{
			Allowed:      false,
			LimitPerHour: limitPerHour,
			Remaining:    0,
			ResetAt:      window.resetAt,
		}
	}
	window.count++
	l.windows[key] = window
	return ToolQuotaState{
		Allowed:      true,
		LimitPerHour: limitPerHour,
		Remaining:    limitPerHour - window.count,
		ResetAt:      window.resetAt,
	}
}

func (r *AgentToolRuntime) toolQuotaLimiter() *ToolQuotaLimiter {
	if r.Limiter == nil {
		r.Limiter = NewToolQuotaLimiter()
	}
	return r.Limiter
}

func (r *AgentToolRuntime) externalConsentManager() *ExternalReadConsentManager {
	if r.Deps == nil {
		r.Deps = &Deps{}
	}
	if r.Deps.ExternalConsent == nil {
		r.Deps.ExternalConsent = NewExternalReadConsentManager(0)
	}
	return r.Deps.ExternalConsent
}

func withToolQuotaState(result any, quota ToolQuotaState) any {
	raw, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"status": "ok", "result": result, "rate_limit": quota}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return map[string]any{"status": "ok", "result": result, "rate_limit": quota}
	}
	obj["rate_limit"] = quota
	return obj
}
