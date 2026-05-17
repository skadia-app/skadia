package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

	assertEventOrder(t, sink.events, []string{EventThinking, EventToolsLoaded, EventToolCall, EventToolResult, EventThinking, EventChunk, EventDone})
	if got := runtime.calls[0].ToolCallID; got != "call-1" {
		t.Fatalf("tool_call_id = %q, want call-1", got)
	}
}

func TestAgentStreamUserQuestionHardStopsAfterToolResult(t *testing.T) {
	runtime := &fakeToolRuntime{result: ToolRuntimeResult{AskUser: true, Question: "Tag anlegen?"}}
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		Preferences: &PreferenceLoader{Defaults: ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"tag_activity"},
		}},
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

	assertEventOrder(t, sink.events, []string{EventThinking, EventToolsLoaded, EventToolCall, EventUserQuestion})
}

func TestAgentStreamUserQuestionSkipsRemainingSameTurnToolCalls(t *testing.T) {
	runtime := &fakeToolRuntime{result: ToolRuntimeResult{AskUser: true, Question: "Tag anlegen?"}}
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		Preferences: &PreferenceLoader{Defaults: ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"tag_activity"},
		}},
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
	assertEventOrder(t, sink.events, []string{EventThinking, EventToolsLoaded, EventToolCall, EventUserQuestion})
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

func TestAgentStreamEmitsLoadedToolsBeforeModelCall(t *testing.T) {
	runtime := &fakeToolRuntime{
		defs: []llmwire.ToolDefinition{
			{Name: "geocode_place", Description: "Orte suchen.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "analyze_route_points", Description: "Distanz messen.", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	streamCalled := false
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalled = true
			return &fakeStream{events: []llmwire.ChatStreamEvent{{
				Delta:        "Fertig.",
				FinishReason: "stop",
			}}}
		},
	})
	sink := &captureSink{}

	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "wie weit ist Beispielort A von Beispielort B entfernt?",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !streamCalled {
		t.Fatal("StreamChat was not called")
	}
	var loaded *SSEEvent
	for idx := range sink.events {
		if sink.events[idx].Type == EventToolsLoaded {
			loaded = &sink.events[idx]
			break
		}
	}
	if loaded == nil {
		t.Fatalf("events = %#v, want tools_loaded", sink.events)
	}
	if got := strings.Join(loaded.Tools, ","); got != "geocode_place,analyze_route_points" {
		t.Fatalf("loaded tools = %#v", loaded.Tools)
	}
}

func TestAgentStreamEmitsLoadedToolsAfterContextFit(t *testing.T) {
	runtime := &fakeToolRuntime{
		defsByName: map[string]llmwire.ToolDefinition{
			"get_agent_service_status": {Name: "get_agent_service_status", Description: strings.Repeat("status ", 20), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"geocode_place":            {Name: "geocode_place", Description: strings.Repeat("geocode ", 20), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"resolve_user_location":    {Name: "resolve_user_location", Description: strings.Repeat("location ", 20), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"resolve_route_place":      {Name: "resolve_route_place", Description: strings.Repeat("place ", 20), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"analyze_route_points":     {Name: "analyze_route_points", Description: strings.Repeat("analysis ", 20), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"query_geo_features":       {Name: "query_geo_features", Description: strings.Repeat("optional ", 900), InputSchema: json.RawMessage(`{"type":"object"}`)},
			"search_routes":            {Name: "search_routes", Description: strings.Repeat("optional ", 900), InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	var captured llmwire.ChatRequest
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		ProfileStore: fakeProfileStore{value: []byte(`{
			"base_profile": "small",
			"overrides": {
				"max_context_tokens": 3300,
				"max_response_tokens": 750,
				"max_tools": 7
			}
		}`)},
		Tools: runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			captured = params
			return &fakeStream{events: []llmwire.ChatStreamEvent{{Delta: "Fertig.", FinishReason: "stop"}}}
		},
	})
	sink := &captureSink{}
	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "Wie weit ist synthetischer Ort A von synthetischem Ort B entfernt?",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var loaded *SSEEvent
	for idx := range sink.events {
		if sink.events[idx].Type == EventToolsLoaded {
			loaded = &sink.events[idx]
			break
		}
	}
	if loaded == nil {
		t.Fatalf("events = %#v, want tools_loaded", sink.events)
	}
	for _, name := range loaded.Tools {
		if !hasLLMToolDefinition(captured.Tools, name) {
			t.Fatalf("tools_loaded included %q but request tools were %#v", name, captured.Tools)
		}
	}
	for _, want := range []string{"get_agent_service_status", "geocode_place", "resolve_user_location", "resolve_route_place", "analyze_route_points"} {
		if !containsToolName(loaded.Tools, want) {
			t.Fatalf("loaded tools = %#v, want required tool %q", loaded.Tools, want)
		}
	}
}

func TestAgentStreamRejectsUnplannedToolCallBeforeRuntime(t *testing.T) {
	runtime := &fakeToolRuntime{
		defsByName: map[string]llmwire.ToolDefinition{
			"query_activities": {Name: "query_activities", Description: "Activities.", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalls++
			if streamCalls == 1 {
				return &fakeStream{events: []llmwire.ChatStreamEvent{{
					ToolCalls:    []llmwire.ToolCallDelta{{Index: 0, ID: "call-write", Name: "tag_activity", Args: `{"activity_id":"a1","tag_name":"x"}`}},
					FinishReason: "tool_calls",
				}}}
			}
			return &fakeStream{events: []llmwire.ChatStreamEvent{{Delta: "Fertig.", FinishReason: "stop"}}}
		},
	})
	sink := &captureSink{}
	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "zeige meine letzte aktivität",
	}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
	if !eventSummaryContains(sink.events, EventToolResult, "not available for this turn") {
		t.Fatalf("events = %#v, want rejected tool result", sink.events)
	}
}

func TestToolPlanExpansionUsesStructuredNeeds(t *testing.T) {
	runtime := &fakeToolRuntime{
		defsByName: map[string]llmwire.ToolDefinition{
			"query_activities":            {Name: "query_activities", Description: "Activities.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"get_agent_service_status":    {Name: "get_agent_service_status", Description: "Status.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"get_routing_coverage":        {Name: "get_routing_coverage", Description: "Coverage.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"query_coverage":              {Name: "query_coverage", Description: "Coverage.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"list_routing_profiles":       {Name: "list_routing_profiles", Description: "Profiles.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"plan_route_between":          {Name: "plan_route_between", Description: "Plan route.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"query_weather_at_point_time": {Name: "query_weather_at_point_time", Description: "Weather.", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	agent := New(Config{Tools: runtime})
	plan := ToolPlan{
		RequiredTools: []string{"query_activities"},
		OptionalTools: []string{"get_routing_coverage", "list_routing_profiles", "query_weather_at_point_time"},
		Tools:         []string{"query_activities"},
		OmittedTools: []ToolOmission{
			{Name: "get_routing_coverage", Reason: "profile_tool_limit"},
			{Name: "list_routing_profiles", Reason: "profile_tool_limit"},
			{Name: "query_weather_at_point_time", Reason: "profile_tool_limit"},
		},
	}
	expanded, defs := agent.expandToolPlanAfterResults(plan, &ChatPreferences{}, runtime.Definitions(plan.Tools, nil), []conversationMessage{{
		Role:    "tool",
		Content: `{"status":"insufficient_capability","needs":["coverage","routing_profiles"]}`,
	}})
	if expanded.Intent.ExpansionLevel != 1 {
		t.Fatalf("expansion level = %d, want 1", expanded.Intent.ExpansionLevel)
	}
	if !containsToolName(expanded.Tools, "get_routing_coverage") || !containsToolName(expanded.Tools, "list_routing_profiles") {
		t.Fatalf("expanded tools = %#v, want coverage and profiles", expanded.Tools)
	}
	if containsToolName(expanded.Tools, "query_weather_at_point_time") {
		t.Fatalf("expanded tools = %#v, did not expect unrelated weather expansion", expanded.Tools)
	}
	if len(defs) != len(expanded.Tools) {
		t.Fatalf("defs = %#v, want one per expanded tool %#v", defs, expanded.Tools)
	}
}

func TestToolLoadedEventIncludesDebugMetadata(t *testing.T) {
	event := toolLoadedEvent("message-1", []llmwire.ToolDefinition{{Name: "query_activities"}}, ToolPlan{
		Intent: AgentIntent{
			Operation: IntentRead,
			Domains:   []string{"activities"},
			Entities:  []IntentEntity{EntityActivity},
		},
		Tools: []string{"query_activities", "get_activity_detail"},
		OmittedTools: []ToolOmission{
			{Name: "create_tag", Reason: "write_intent_not_requested"},
		},
		Reason: "intent-router",
	})
	if event.Intent != "read:activities:activity" {
		t.Fatalf("intent = %q, want debug label", event.Intent)
	}
	if event.ExpansionLevel == nil || *event.ExpansionLevel != 0 {
		t.Fatalf("expansion level = %#v, want 0", event.ExpansionLevel)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if !strings.Contains(string(raw), `"expansion_level":0`) {
		t.Fatalf("event JSON missing expansion_level 0: %s", raw)
	}
	if !strings.Contains(string(raw), `"write_intent_not_requested"`) || !strings.Contains(string(raw), `"context_budget"`) {
		t.Fatalf("event JSON missing omission reasons: %s", raw)
	}
}

func TestAgentStreamExpandsToolsFromStructuredNeeds(t *testing.T) {
	runtime := &fakeToolRuntime{
		result: ToolRuntimeResult{Summary: `{"status":"insufficient_capability","needs":["routing_planning"]}`},
		defsByName: map[string]llmwire.ToolDefinition{
			"get_agent_service_status": {Name: "get_agent_service_status", Description: "Status.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"geocode_place":            {Name: "geocode_place", Description: "Geocode.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"resolve_user_location":    {Name: "resolve_user_location", Description: "User location.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"resolve_route_place":      {Name: "resolve_route_place", Description: "Place.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"analyze_route_points":     {Name: "analyze_route_points", Description: "Analyze.", InputSchema: json.RawMessage(`{"type":"object"}`)},
			"plan_route_between":       {Name: "plan_route_between", Description: "Plan route.", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	requests := []llmwire.ChatRequest{}
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			requests = append(requests, params)
			streamCalls++
			if streamCalls == 1 {
				return &fakeStream{events: []llmwire.ChatStreamEvent{{
					ToolCalls:    []llmwire.ToolCallDelta{{Index: 0, ID: "call-analyze", Name: "analyze_route_points", Args: `{"points":[{"lat":47,"lng":11},{"lat":48,"lng":12}]}`}},
					FinishReason: "tool_calls",
				}}}
			}
			return &fakeStream{events: []llmwire.ChatStreamEvent{{Delta: "Ich kann eine Route planen.", FinishReason: "stop"}}}
		},
	})
	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "Wie weit ist Beispielstadt Nord von Beispielstadt Sued entfernt?",
	}, &captureSink{}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(requests) < 2 {
		t.Fatalf("requests len = %d, want at least 2", len(requests))
	}
	if hasLLMToolDefinition(requests[0].Tools, "plan_route_between") {
		t.Fatalf("initial tools unexpectedly included plan_route_between: %#v", requests[0].Tools)
	}
	if !hasLLMToolDefinition(requests[1].Tools, "plan_route_between") {
		t.Fatalf("expanded tools = %#v, want plan_route_between", requests[1].Tools)
	}
}

func TestAgentChatParamsUsesProviderSpecificOptionalFlags(t *testing.T) {
	agent := New(Config{})
	profile := defaultProfiles["small"]
	messages := []conversationMessage{{Role: "user", Content: "Hallo"}}

	openAIParams := agent.chatParams(LLMConfig{Model: "model", BackendType: model.LLMBackendOpenAI}, &profile, messages, []llmwire.ToolDefinition{{Name: "query_activities"}}, true, false, profile.MaxResponseTokens)
	if !openAIParams.IncludeUsage {
		t.Fatal("OpenAI chat params did not request streaming usage")
	}
	if !openAIParams.DisableParallelToolCalls {
		t.Fatal("OpenAI chat params did not disable parallel tool calls when tools are allowed")
	}

	llamaParams := agent.chatParams(LLMConfig{Model: "model", BackendType: model.LLMBackendLlamaServer}, &profile, messages, []llmwire.ToolDefinition{{Name: "query_activities"}}, true, false, profile.MaxResponseTokens)
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
		profile.MaxResponseTokens,
	)
	if params.ToolChoice != "required" {
		t.Fatalf("ToolChoice = %q, want required", params.ToolChoice)
	}
}

func TestNewLLMClientUsesLongTimeoutForLocalStreams(t *testing.T) {
	client := NewLLMClient(LLMConfig{BaseURL: "http://llm.test/v1"})
	if client.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
	if client.HTTPClient.Timeout < 10*time.Minute {
		t.Fatalf("HTTPClient.Timeout = %s, want at least 10m for local LLM streams", client.HTTPClient.Timeout)
	}
}

func TestAgentStreamFitsPromptToConfiguredContextWindow(t *testing.T) {
	runtime := &fakeToolRuntime{
		defs: []llmwire.ToolDefinition{{
			Name:        "query_activities",
			Description: "Aktivitäten lesen.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}}}`),
		}},
	}
	var captured llmwire.ChatRequest
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		ProfileStore: fakeProfileStore{value: []byte(`{
			"base_profile": "small",
			"overrides": {
				"max_context_tokens": 3500,
				"history_messages": 20,
				"max_response_tokens": 1500
			}
		}`)},
		Tools: runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			captured = params
			return &fakeStream{events: []llmwire.ChatStreamEvent{{
				Delta:        "Fertig.",
				FinishReason: "stop",
			}}}
		},
	})
	history := make([]ChatHistoryMessage, 0, 20)
	for range 20 {
		history = append(history, ChatHistoryMessage{
			Role:    "user",
			Content: "old-marker " + strings.Repeat("lange Historie ", 120),
		})
	}

	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "Wann war ich zuletzt Radfahren?",
		History:   history,
	}, &captureSink{}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := estimateLLMRequestTokens(captured); got > 3500 {
		t.Fatalf("estimated request tokens = %d, want <= 3500", got)
	}
	if captured.MaxTokens == nil || *captured.MaxTokens >= 1500 {
		t.Fatalf("max_tokens = %v, want capped below profile response tokens", captured.MaxTokens)
	}
	oldHistoryMessages := 0
	for _, message := range captured.Messages {
		if strings.Contains(message.Content, "old-marker") {
			oldHistoryMessages++
		}
	}
	if oldHistoryMessages >= len(history) {
		t.Fatalf("captured all old history despite tight context budget: %#v", captured.Messages)
	}
}

func TestAgentStreamFitsPromptAfterToolResults(t *testing.T) {
	runtime := &fakeToolRuntime{
		defs: []llmwire.ToolDefinition{{
			Name:        "query_activities",
			Description: "Aktivitäten lesen.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}}}`),
		}},
		result: ToolRuntimeResult{Summary: strings.Repeat("sehr langes Tool-Ergebnis ", 180)},
	}
	var secondCall llmwire.ChatRequest
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		ProfileStore: fakeProfileStore{value: []byte(`{
			"base_profile": "small",
			"overrides": {
				"max_context_tokens": 2600,
				"max_response_tokens": 1200,
				"tool_result_limit": 40
			}
		}`)},
		Tools: runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalls++
			if streamCalls == 2 {
				secondCall = params
				return &fakeStream{events: []llmwire.ChatStreamEvent{{Delta: "Fertig.", FinishReason: "stop"}}}
			}
			return &fakeStream{events: []llmwire.ChatStreamEvent{{
				ToolCalls:    []llmwire.ToolCallDelta{{Index: 0, ID: "call-1", Name: "query_activities", Args: `{"limit":10}`}},
				FinishReason: "tool_calls",
			}}}
		},
	})

	if err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   "Wann war ich zuletzt Radfahren?",
	}, &captureSink{}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if streamCalls != 2 {
		t.Fatalf("stream calls = %d, want 2", streamCalls)
	}
	if got := estimateLLMRequestTokens(secondCall); got > 2600 {
		t.Fatalf("estimated second request tokens = %d, want <= 2600", got)
	}
}

func TestAgentStreamStopsWhenBaseRequestCannotFitContext(t *testing.T) {
	streamCalls := 0
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		ProfileStore: fakeProfileStore{value: []byte(`{
			"base_profile": "small",
			"overrides": {
				"max_context_tokens": 1024,
				"max_response_tokens": 1000
			}
		}`)},
		StreamChat: func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			streamCalls++
			return &fakeStream{}
		},
	})

	err := agent.Stream(context.Background(), ChatRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		MessageID: "message-1",
		Message:   strings.Repeat("lange aktuelle Frage ", 600),
	}, &captureSink{})
	if err == nil {
		t.Fatal("Stream returned nil, want context budget error")
	}
	if streamCalls != 0 {
		t.Fatalf("stream calls = %d, want 0", streamCalls)
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
	defs            []llmwire.ToolDefinition
	defsByName      map[string]llmwire.ToolDefinition
}

func (r *fakeToolRuntime) AddEnabledWriteTools(filter []string, prefs *ChatPreferences) []string {
	return filter
}

func (r *fakeToolRuntime) Definitions(filter []string, prefs *ChatPreferences) []llmwire.ToolDefinition {
	if r.defsByName != nil {
		out := make([]llmwire.ToolDefinition, 0, len(filter))
		for _, name := range filter {
			if def, ok := r.defsByName[name]; ok {
				out = append(out, def)
			}
		}
		return out
	}
	if len(r.defs) == 0 && len(filter) > 0 {
		out := make([]llmwire.ToolDefinition, 0, len(filter))
		for _, name := range filter {
			out = append(out, llmwire.ToolDefinition{Name: name, Description: name, InputSchema: json.RawMessage(`{"type":"object"}`)})
		}
		return out
	}
	return r.defs
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

func eventSummaryContains(events []SSEEvent, eventType, needle string) bool {
	for _, event := range events {
		if event.Type == eventType && strings.Contains(event.Summary, needle) {
			return true
		}
	}
	return false
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

func estimateLLMRequestTokens(req llmwire.ChatRequest) int {
	raw, _ := json.Marshal(struct {
		Messages []llmwire.ChatMessage    `json:"messages"`
		Tools    []llmwire.ToolDefinition `json:"tools,omitempty"`
	}{
		Messages: req.Messages,
		Tools:    req.Tools,
	})
	total := (len(raw) + 2) / 3
	if req.MaxTokens != nil {
		total += *req.MaxTokens
	}
	return total
}

func hasLLMToolDefinition(tools []llmwire.ToolDefinition, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
