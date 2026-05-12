package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestAgentStreamExecutesToolCallThenStreamsFinalAnswer(t *testing.T) {
	runtime := &fakeToolRuntime{
		result: ToolRuntimeResult{Result: map[string]any{"ok": true}, Summary: `{"ok":true}`},
	}
	streams := []ChatStream{
		&fakeStream{events: []llmwire.ChatStreamEvent{{
			ToolCalls: []llmwire.ToolCallDelta{{
				Index: 0,
				ID:    "call-1",
				Name:  "query_activities",
				Args:  `{"limit":1}`,
			}},
			FinishReason: "tool_calls",
		}}},
		&fakeStream{events: []llmwire.ChatStreamEvent{{
			Delta:        "Fertig.",
			FinishReason: "stop",
		}}},
	}
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			if len(streams) == 0 {
				t.Fatal("unexpected extra stream call")
			}
			next := streams[0]
			streams = streams[1:]
			return next
		},
	})
	sink := &captureSink{}

	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "analysiere mein training",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	assertEventOrder(t, sink.events, []string{EventThinking, EventToolCall, EventToolResult, EventThinking, EventChunk, EventDone})
	if got := runtime.calls[0].ToolCallID; got != "call-1" {
		t.Fatalf("tool_call_id = %q, want call-1", got)
	}
}

func TestAgentStreamUserQuestionHardStopsAfterToolResult(t *testing.T) {
	runtime := &fakeToolRuntime{result: ToolRuntimeResult{AskUser: true, Question: "Tag anlegen?"}}
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			return &fakeStream{events: []llmwire.ChatStreamEvent{{
				ToolCalls: []llmwire.ToolCallDelta{{
					Index: 0,
					ID:    "call-tag",
					Name:  "tag_activity",
					Args:  `{"activity_id":"a1","tag_name":"Gravel"}`,
				}},
				FinishReason: "tool_calls",
			}}}
		},
	})
	sink := &captureSink{}

	if err := agent.Stream(context.Background(), ChatRequest{UserID: "user-1", SessionID: "session-1", MessageID: "message-1", Message: "tagge aktivität"}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	assertEventOrder(t, sink.events, []string{EventThinking, EventToolCall, EventUserQuestion})
}

func TestAgentStreamUserQuestionSkipsRemainingSameTurnToolCalls(t *testing.T) {
	runtime := &fakeToolRuntime{result: ToolRuntimeResult{AskUser: true, Question: "Tag anlegen?"}}
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalls++
			if streamCalls > 1 {
				t.Fatal("unexpected follow-up model stream after user question")
			}
			return &fakeStream{events: []llmwire.ChatStreamEvent{{
				ToolCalls: []llmwire.ToolCallDelta{
					{Index: 0, ID: "call-tag", Name: "tag_activity", Args: `{"activity_id":"a1","tag_name":"Gravel"}`},
					{Index: 1, ID: "call-route", Name: "suggest_route", Args: `{"sport_type":"cycling"}`},
				},
				FinishReason: "tool_calls",
			}}}
		},
	})
	sink := &captureSink{}

	if err := agent.Stream(context.Background(), ChatRequest{UserID: "user-1", SessionID: "session-1", MessageID: "message-1", Message: "tagge und plane"}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0].ToolName != "tag_activity" {
		t.Fatalf("runtime calls = %#v, want only first ask_user tool call", runtime.calls)
	}
	assertEventOrder(t, sink.events, []string{EventThinking, EventToolCall, EventUserQuestion})
}

func TestCollectLLMStreamAccumulatesSplitToolDeltasAndGeneratesMissingIDs(t *testing.T) {
	stream := &fakeStream{events: []llmwire.ChatStreamEvent{
		{ToolCalls: []llmwire.ToolCallDelta{
			{Index: 1, Name: "tag_", Args: `{"activity_`},
			{Index: 0, Name: "query_", Args: `{"limit":`},
		}},
		{ToolCalls: []llmwire.ToolCallDelta{
			{Index: 0, ID: "late-call-0", Name: "activities", Args: `1}`},
			{Index: 1, Name: "activity", Args: `id":"a1"}`},
		}, FinishReason: "tool_calls"},
	}}

	calls, finishReason, streamed, err := collectLLMStream("message-1", stream, &captureSink{}, &TokenUsage{})
	if err != nil {
		t.Fatalf("collectLLMStream: %v", err)
	}
	if streamed {
		t.Fatal("streamed = true, want false for tool-only chunks")
	}
	if finishReason != "tool_calls" {
		t.Fatalf("finishReason = %q, want tool_calls", finishReason)
	}
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2: %#v", len(calls), calls)
	}
	if calls[0].ID != "late-call-0" || calls[0].Name != "query_activities" || calls[0].Args != `{"limit":1}` {
		t.Fatalf("call[0] = %#v", calls[0])
	}
	if calls[1].Name != "tag_activity" || calls[1].Args != `{"activity_id":"a1"}` {
		t.Fatalf("call[1] = %#v", calls[1])
	}
	if _, err := uuid.Parse(calls[1].ID); err != nil {
		t.Fatalf("call[1].ID = %q, want generated UUID: %v", calls[1].ID, err)
	}
}

type fakeToolRuntime struct {
	result ToolRuntimeResult
	calls  []ToolRuntimeRequest
}

func (r *fakeToolRuntime) AddEnabledWriteTools(filter []string, prefs *ChatPreferences) []string {
	return filter
}

func (r *fakeToolRuntime) Definitions(filter []string, prefs *ChatPreferences) []llmwire.ToolDefinition {
	return nil
}

func (r *fakeToolRuntime) Execute(ctx context.Context, req ToolRuntimeRequest, sink SSESink) (ToolRuntimeResult, error) {
	r.calls = append(r.calls, req)
	return r.result, nil
}

type fakeStream struct {
	events []llmwire.ChatStreamEvent
	idx    int
}

func (s *fakeStream) Next() bool {
	return s.idx < len(s.events)
}

func (s *fakeStream) Current() llmwire.ChatStreamEvent {
	event := s.events[s.idx]
	s.idx++
	return event
}

func (s *fakeStream) Err() error { return nil }

type captureSink struct {
	events []SSEEvent
	raw    []string
	taps   []func(string)
}

func (s *captureSink) Send(event SSEEvent) error {
	s.events = append(s.events, event)
	if event.Type == EventChunk {
		for _, tap := range s.taps {
			tap(event.Delta)
		}
	}
	return nil
}

func (s *captureSink) WriteRaw(frame string) error {
	s.raw = append(s.raw, frame)
	return nil
}

func (s *captureSink) OnChunk(fn func(delta string)) {
	s.taps = append(s.taps, fn)
}

type fakeLLMConfigStore struct{}

func (fakeLLMConfigStore) GetLLMSystemConfig(ctx context.Context) ([]model.LLMSystemConfig, error) {
	baseURL := "http://llm.test/v1"
	modelName := "model"
	return []model.LLMSystemConfig{{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &baseURL,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusPassed,
		EvalReport: map[string]any{"identity": map[string]any{
			"role":          model.LLMRoleToolAgent,
			"backend_type":  "llama_server",
			"api_base_host": "llm.test",
			"model_name":    "model",
		}},
	}}, nil
}

func (fakeLLMConfigStore) GetLLMAPIKeyEnc(ctx context.Context, role string) (string, error) {
	return "", nil
}

func (fakeLLMConfigStore) GetLLMUserConfig(ctx context.Context, userID string) ([]model.LLMUserConfig, error) {
	return nil, nil
}

func assertEventOrder(t *testing.T, events []SSEEvent, want []string) {
	t.Helper()
	got := make([]string, len(events))
	for idx, event := range events {
		got[idx] = event.Type
	}
	rawGot, _ := json.Marshal(got)
	rawWant, _ := json.Marshal(want)
	if string(rawGot) != string(rawWant) {
		t.Fatalf("events = %s, want %s", rawGot, rawWant)
	}
}
