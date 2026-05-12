package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/openai/openai-go"
)

func TestAgentStreamExecutesToolCallThenStreamsFinalAnswer(t *testing.T) {
	runtime := &fakeToolRuntime{
		result: ToolRuntimeResult{Result: map[string]any{"ok": true}, Summary: `{"ok":true}`},
	}
	streams := []ChatStream{
		&fakeStream{chunks: []openai.ChatCompletionChunk{{
			Choices: []openai.ChatCompletionChunkChoice{{
				Delta: openai.ChatCompletionChunkChoiceDelta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
					Index: 0,
					ID:    "call-1",
					Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
						Name:      "query_activities",
						Arguments: `{"limit":1}`,
					},
				}}},
				FinishReason: "tool_calls",
			}},
		}}},
		&fakeStream{chunks: []openai.ChatCompletionChunk{{
			Choices: []openai.ChatCompletionChunkChoice{{
				Delta:        openai.ChatCompletionChunkChoiceDelta{Content: "Fertig."},
				FinishReason: "stop",
			}},
		}}},
	}
	agent := New(Config{
		LLMResolver: NewLLMResolver(fakeLLMConfigStore{}, LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		Tools:       runtime,
		StreamChat: func(ctx context.Context, cfg LLMConfig, params openai.ChatCompletionNewParams) ChatStream {
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
		StreamChat: func(ctx context.Context, cfg LLMConfig, params openai.ChatCompletionNewParams) ChatStream {
			return &fakeStream{chunks: []openai.ChatCompletionChunk{{
				Choices: []openai.ChatCompletionChunkChoice{{
					Delta: openai.ChatCompletionChunkChoiceDelta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
						Index: 0,
						ID:    "call-tag",
						Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
							Name:      "tag_activity",
							Arguments: `{"activity_id":"a1","tag_name":"Gravel"}`,
						},
					}}},
					FinishReason: "tool_calls",
				}},
			}}}
		},
	})
	sink := &captureSink{}

	if err := agent.Stream(context.Background(), ChatRequest{UserID: "user-1", SessionID: "session-1", MessageID: "message-1", Message: "tagge aktivität"}, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	assertEventOrder(t, sink.events, []string{EventThinking, EventToolCall, EventUserQuestion})
}

type fakeToolRuntime struct {
	result ToolRuntimeResult
	calls  []ToolRuntimeRequest
}

func (r *fakeToolRuntime) AddEnabledWriteTools(filter []string, prefs *ChatPreferences) []string {
	return filter
}

func (r *fakeToolRuntime) Definitions(filter []string, prefs *ChatPreferences) []openai.ChatCompletionToolParam {
	return nil
}

func (r *fakeToolRuntime) Execute(ctx context.Context, req ToolRuntimeRequest, sink SSESink) (ToolRuntimeResult, error) {
	r.calls = append(r.calls, req)
	return r.result, nil
}

type fakeStream struct {
	chunks []openai.ChatCompletionChunk
	idx    int
}

func (s *fakeStream) Next() bool {
	return s.idx < len(s.chunks)
}

func (s *fakeStream) Current() openai.ChatCompletionChunk {
	chunk := s.chunks[s.idx]
	s.idx++
	return chunk
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
	return nil, nil
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
