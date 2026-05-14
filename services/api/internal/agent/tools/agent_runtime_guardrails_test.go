package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
)

func TestAgentToolRuntimeRateLimitPerUserToolPerHour(t *testing.T) {
	reg := NewRegistry()
	limitedCalls := 0
	reg.Register(&Tool{
		Name:             "limited_external_read",
		Description:      "Limited external read",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 2,
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			limitedCalls++
			return map[string]any{"status": "ok"}, nil
		},
	})
	reg.Register(&Tool{
		Name:             "other_limited_external_read",
		Description:      "Other limited external read",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 1,
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			return map[string]any{"status": "ok"}, nil
		},
	})
	reg.Register(&Tool{
		Name:        "local_read",
		Description: "Local read",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:  PermissionRead,
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			return map[string]any{"status": "ok"}, nil
		},
	})
	runtime := NewAgentToolRuntime(reg, nil, nil)

	first := executeGuardrailTool(t, runtime, "user-a", "limited_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, first, "ok")
	assertRateLimitState(t, first, true, 2, 1)

	second := executeGuardrailTool(t, runtime, "user-a", "limited_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, second, "ok")
	assertRateLimitState(t, second, true, 2, 0)

	blocked := executeGuardrailTool(t, runtime, "user-a", "limited_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, blocked, "rate_limited")
	if got := toolResultMap(t, blocked)["blocking_reason"]; got != "rate_limited" {
		t.Fatalf("blocking_reason = %#v, want rate_limited", got)
	}
	assertRateLimitState(t, blocked, false, 2, 0)
	if limitedCalls != 2 {
		t.Fatalf("limited tool executed %d times, want 2", limitedCalls)
	}

	otherUser := executeGuardrailTool(t, runtime, "user-b", "limited_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, otherUser, "ok")
	assertRateLimitState(t, otherUser, true, 2, 1)

	otherTool := executeGuardrailTool(t, runtime, "user-a", "other_limited_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, otherTool, "ok")
	assertRateLimitState(t, otherTool, true, 1, 0)

	for i := 0; i < 3; i++ {
		local := executeGuardrailTool(t, runtime, "user-a", "local_read", json.RawMessage(`{}`))
		assertToolStatus(t, local, "ok")
		if _, ok := toolResultMap(t, local)["rate_limit"]; ok {
			t.Fatalf("local non-limited tool returned rate_limit metadata: %#v", local.Result)
		}
	}
}

func TestAgentToolRuntimeTimeoutReturnsStructuredResult(t *testing.T) {
	reg := NewRegistry()
	started := make(chan struct{})
	reg.Register(&Tool{
		Name:        "slow_external_read",
		Description: "Slow external read",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:  PermissionRead,
		Timeout:     10 * time.Millisecond,
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	runtime := NewAgentToolRuntime(reg, nil, nil)

	result := executeGuardrailTool(t, runtime, "user-a", "slow_external_read", json.RawMessage(`{}`))
	assertToolStatus(t, result, "timed_out")
	if got := toolResultMap(t, result)["blocking_reason"]; got != "timed_out" {
		t.Fatalf("blocking_reason = %#v, want timed_out", got)
	}
	select {
	case <-started:
	default:
		t.Fatal("slow tool did not start")
	}
}

func TestAgentToolRuntimeCancelReturnsStructuredResult(t *testing.T) {
	reg := NewRegistry()
	started := make(chan struct{})
	reg.Register(&Tool{
		Name:        "cancellable_external_read",
		Description: "Cancellable external read",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:  PermissionRead,
		Timeout:     time.Second,
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	runtime := NewAgentToolRuntime(reg, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan agent.ToolRuntimeResult, 1)
	go func() {
		result, err := runtime.Execute(ctx, agent.ToolRuntimeRequest{
			UserID:     "user-a",
			SessionID:  "session-a",
			MessageID:  "msg-a",
			ToolCallID: "call-a",
			ToolName:   "cancellable_external_read",
			Args:       json.RawMessage(`{}`),
		}, nil)
		if err != nil {
			t.Errorf("runtime Execute returned error: %v", err)
		}
		done <- result
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tool start")
	}
	cancel()

	select {
	case result := <-done:
		assertToolStatus(t, result, "cancelled")
		if got := toolResultMap(t, result)["blocking_reason"]; got != "cancelled" {
			t.Fatalf("blocking_reason = %#v, want cancelled", got)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not return after cancellation")
	}
}

func executeGuardrailTool(t *testing.T, runtime *AgentToolRuntime, userID, toolName string, args json.RawMessage) agent.ToolRuntimeResult {
	t.Helper()
	result, err := runtime.Execute(context.Background(), agent.ToolRuntimeRequest{
		UserID:     userID,
		SessionID:  "session-a",
		MessageID:  "msg-a",
		ToolCallID: "call-a",
		ToolName:   toolName,
		Args:       args,
	}, nil)
	if err != nil {
		t.Fatalf("runtime Execute(%s): %v", toolName, err)
	}
	return result
}

func assertToolStatus(t *testing.T, result agent.ToolRuntimeResult, want string) {
	t.Helper()
	if got := toolResultMap(t, result)["status"]; got != want {
		t.Fatalf("status = %#v, want %s; result=%#v", got, want, result.Result)
	}
}

func assertRateLimitState(t *testing.T, result agent.ToolRuntimeResult, allowed bool, limit, remaining int) {
	t.Helper()
	state, ok := toolResultMap(t, result)["rate_limit"].(map[string]any)
	if !ok {
		t.Fatalf("rate_limit missing or wrong type: %#v", result.Result)
	}
	if got := state["allowed"]; got != allowed {
		t.Fatalf("rate_limit.allowed = %#v, want %v", got, allowed)
	}
	if got := int(state["limit_per_hour"].(float64)); got != limit {
		t.Fatalf("rate_limit.limit_per_hour = %d, want %d", got, limit)
	}
	if got := int(state["remaining"].(float64)); got != remaining {
		t.Fatalf("rate_limit.remaining = %d, want %d", got, remaining)
	}
	if _, ok := state["reset_at"].(string); !ok {
		t.Fatalf("rate_limit.reset_at missing or wrong type: %#v", state)
	}
}

func toolResultMap(t *testing.T, result agent.ToolRuntimeResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(result.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal result: %v; raw=%s", err, raw)
	}
	return out
}
