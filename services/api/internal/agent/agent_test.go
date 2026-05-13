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

func TestAgentStreamStopsBeforeLLMWhenChatProfileDisabled(t *testing.T) {
	streamCalls := 0
	agent := New(Config{
		LLMResolver:  NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		ProfileStore: fakeProfileStore{value: []byte(`{"enabled": false, "base_profile": "small"}`)},
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalls++
			return &fakeStream{}
		},
	})
	sink := &captureSink{}

	err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "analysiere mein training",
	}, sink)
	if err == nil {
		t.Fatal("Stream returned nil for disabled chat profile")
	}
	if streamCalls != 0 {
		t.Fatalf("StreamChat calls = %d, want 0", streamCalls)
	}
	assertEventOrder(t, sink.events, []string{EventError})
}

func TestAgentChatParamsUsesProviderSpecificOptionalFlags(t *testing.T) {
	agent := New(Config{})
	profile := defaultProfiles["small"]
	messages := []conversationMessage{{Role: "user", Content: "Hallo"}}

	openAIParams := agent.chatParams(LLMConfig{Model: "model", BackendType: model.LLMBackendOpenAI}, &profile, messages, []llmwire.ToolDefinition{{Name: "query_activities"}}, true, false)
	if !openAIParams.IncludeUsage {
		t.Fatal("OpenAI chat params did not request streaming usage")
	}
	if !openAIParams.DisableParallelToolCalls {
		t.Fatal("OpenAI chat params did not disable parallel tool calls when tools are allowed")
	}

	llamaParams := agent.chatParams(LLMConfig{Model: "model", BackendType: model.LLMBackendLlamaServer}, &profile, messages, []llmwire.ToolDefinition{{Name: "query_activities"}}, true, false)
	if llamaParams.IncludeUsage {
		t.Fatal("llama_server chat params requested provider-specific streaming usage")
	}
	if llamaParams.DisableParallelToolCalls {
		t.Fatal("llama_server chat params sent provider-specific parallel tool control")
	}
}

func TestAgentChatParamsCanRequireToolChoice(t *testing.T) {
	agent := New(Config{})
	profile := defaultProfiles["small"]
	params := agent.chatParams(
		LLMConfig{Model: "model", BackendType: model.LLMBackendLlamaServer},
		&profile,
		[]conversationMessage{{Role: "user", Content: "Wann war ich zuletzt schwimmen?"}},
		[]llmwire.ToolDefinition{{Name: "query_activities"}},
		true,
		true,
	)
	if params.ToolChoice != "required" {
		t.Fatalf("ToolChoice = %q, want required", params.ToolChoice)
	}
}

func TestShouldRequireToolForDataQuestions(t *testing.T) {
	tools := []llmwire.ToolDefinition{{Name: "query_activities"}}
	if !shouldRequireTool("Wann war ich zuletzt schwimmen?", []string{"activities"}, tools) {
		t.Fatal("shouldRequireTool returned false for activity data question")
	}
	if !shouldRequireTool("When was my last swim?", []string{"activities"}, tools) {
		t.Fatal("shouldRequireTool returned false for English activity data question")
	}
	if shouldRequireTool("Hallo", []string{"activities"}, tools) {
		t.Fatal("shouldRequireTool returned true for greeting")
	}
	if shouldRequireTool("Wann war ich zuletzt schwimmen?", []string{"activities"}, nil) {
		t.Fatal("shouldRequireTool returned true without tools")
	}
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

func TestCollectLLMStreamDoesNotExecuteTextFallbackToolEnvelope(t *testing.T) {
	stream := &fakeStream{events: []llmwire.ChatStreamEvent{{
		Delta:        `{"name":"run_sql","arguments":{"sql":"SELECT * FROM activities"}}`,
		FinishReason: "stop",
	}}}
	sink := &captureSink{}

	calls, finishReason, streamed, err := collectLLMStream("message-1", stream, sink, &TokenUsage{})
	if err != nil {
		t.Fatalf("collectLLMStream: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %#v, want no free-text fallback tool execution", calls)
	}
	if finishReason != "stop" {
		t.Fatalf("finishReason = %q, want stop", finishReason)
	}
	if !streamed || len(sink.events) != 1 || sink.events[0].Type != EventChunk {
		t.Fatalf("streamed=%v events=%#v, want plain assistant text streamed", streamed, sink.events)
	}
}

func TestCollectLLMStreamDoesNotDuplicateRepeatedFullToolNames(t *testing.T) {
	stream := &fakeStream{events: []llmwire.ChatStreamEvent{
		{ToolCalls: []llmwire.ToolCallDelta{{Index: 0, ID: "call-1", Name: "run_sql", Args: `{"sql":`}}},
		{ToolCalls: []llmwire.ToolCallDelta{{Index: 0, Name: "run_sql", Args: `"SELECT 1"}`}}, FinishReason: "tool_calls"},
	}}
	calls, _, _, err := collectLLMStream("message-1", stream, &captureSink{}, &TokenUsage{})
	if err != nil {
		t.Fatalf("collectLLMStream: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "run_sql" {
		t.Fatalf("calls = %#v, want one run_sql call", calls)
	}
}

func TestAgentStreamOmitsToolCallArgsWhenRuntimeMarksThemPrivate(t *testing.T) {
	runtime := &fakeToolRuntime{
		result:          ToolRuntimeResult{Result: map[string]any{"ok": true}, Summary: `{"ok":true}`},
		publicArgsAllow: false,
	}
	streams := []ChatStream{
		&fakeStream{events: []llmwire.ChatStreamEvent{{
			ToolCalls: []llmwire.ToolCallDelta{{
				Index: 0,
				ID:    "call-write",
				Name:  "create_gear",
				Args:  `{"name":"Secret Bike","api_key":"secret"}`,
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
		Message:   "lege gear an",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	for _, event := range sink.events {
		if event.Type == EventToolCall && len(event.Args) > 0 {
			t.Fatalf("tool_call event leaked args: %s", event.Args)
		}
	}
}

func TestAgentStreamOmitsToolCallArgsWhenRuntimeHasNoPublicArgsGate(t *testing.T) {
	runtime := &fakeToolRuntimeNoPublicArgs{result: ToolRuntimeResult{Result: map[string]any{"ok": true}, Summary: `{"ok":true}`}}
	streams := []ChatStream{
		&fakeStream{events: []llmwire.ChatStreamEvent{{
			ToolCalls: []llmwire.ToolCallDelta{{
				Index: 0,
				ID:    "call-write",
				Name:  "create_gear",
				Args:  `{"name":"Secret Bike"}`,
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
		Message:   "lege gear an",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for _, event := range sink.events {
		if event.Type == EventToolCall && len(event.Args) > 0 {
			t.Fatalf("tool_call event leaked args: %s", event.Args)
		}
	}
}

type fakeToolRuntime struct {
	result          ToolRuntimeResult
	calls           []ToolRuntimeRequest
	publicArgsAllow bool
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

func (r *fakeToolRuntime) PublicToolCallArgs(toolName string, args json.RawMessage) (json.RawMessage, bool) {
	if r.publicArgsAllow {
		return args, true
	}
	return nil, false
}

type fakeToolRuntimeNoPublicArgs struct {
	result ToolRuntimeResult
}

func (r *fakeToolRuntimeNoPublicArgs) AddEnabledWriteTools(filter []string, prefs *ChatPreferences) []string {
	return filter
}

func (r *fakeToolRuntimeNoPublicArgs) Definitions(filter []string, prefs *ChatPreferences) []llmwire.ToolDefinition {
	return nil
}

func (r *fakeToolRuntimeNoPublicArgs) Execute(ctx context.Context, req ToolRuntimeRequest, sink SSESink) (ToolRuntimeResult, error) {
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
			"suite_id":      model.LLMEvalSuiteID,
			"suite_hash":    model.LLMEvalSuiteHash,
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
