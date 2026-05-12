package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

type AgentToolRuntime struct {
	Registry *Registry
	Deps     *Deps
	Envelope *ExecutionEnvelope
}

func NewAgentToolRuntime(reg *Registry, deps *Deps, envelope *ExecutionEnvelope) *AgentToolRuntime {
	if envelope == nil {
		envelope = NewExecutionEnvelope(reg, deps)
	}
	return &AgentToolRuntime{Registry: reg, Deps: deps, Envelope: envelope}
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

	switch {
	case tool.AuditMode == AuditPreview && tool.ExecutePreview != nil:
		result, err := r.Envelope.ExecutePreview(ctx, toExecutionRequest(req, tool.AuditMode))
		return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
	case tool.Permission == PermissionRead:
		result, err := r.Registry.ExecuteRead(ctx, r.Deps, req.UserID, req.ToolName, req.Args)
		return agent.ToolRuntimeResult{Result: result, Summary: marshalToolResult(result)}, err
	default:
		return r.executeConfirmedWrite(ctx, req, sink, tool)
	}
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
		Args:       req.Args,
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
