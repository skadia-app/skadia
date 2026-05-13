package llmwire

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatCompletionsClientPostsSkadiaRequestAsChatCompletionsStream(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hallo\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	client := NewChatCompletionsClient(server.URL+"/v1", "resolved-key", nil)
	stream := client.Stream(context.Background(), ChatRequest{
		Model: "test-model",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: "Hallo",
		}},
		Tools: []ToolDefinition{{
			Name:        "query_activities",
			Description: "Query activities",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})

	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	if got := stream.Current().Delta; got != "Hallo" {
		t.Fatalf("Delta = %q, want Hallo", got)
	}
	if stream.Next() {
		t.Fatal("Next() after [DONE] = true")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}

	if gotAuth != "Bearer resolved-key" {
		t.Fatalf("Authorization = %q, want Bearer resolved-key", gotAuth)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if _, ok := gotBody["stream_options"]; ok {
		t.Fatalf("stream_options was sent by default: %#v", gotBody["stream_options"])
	}
	if _, ok := gotBody["parallel_tool_calls"]; ok {
		t.Fatalf("parallel_tool_calls was sent by default: %#v", gotBody["parallel_tool_calls"])
	}
	tools := gotBody["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "query_activities" || fn["description"] != "Query activities" {
		t.Fatalf("function definition = %#v", fn)
	}
	if _, ok := fn["input_schema"]; ok {
		t.Fatalf("provider request leaked Skadia input_schema field: %#v", fn)
	}
	if _, ok := fn["parameters"]; !ok {
		t.Fatalf("function definition missing parameters: %#v", fn)
	}
}

func TestChatCompletionsClientSendsOptionalProviderParamsOnlyWhenRequested(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hallo\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	stream := NewChatCompletionsClient(server.URL, "key", nil).Stream(context.Background(), ChatRequest{
		Model:                    "test-model",
		Messages:                 []ChatMessage{{Role: "user", Content: "Hallo"}},
		IncludeUsage:             true,
		DisableParallelToolCalls: true,
	})
	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	if stream.Next() {
		t.Fatal("Next() after [DONE] = true")
	}
	streamOptions := gotBody["stream_options"].(map[string]any)
	if streamOptions["include_usage"] != true {
		t.Fatalf("stream_options.include_usage = %#v, want true", streamOptions["include_usage"])
	}
	if gotBody["parallel_tool_calls"] != false {
		t.Fatalf("parallel_tool_calls = %#v, want false", gotBody["parallel_tool_calls"])
	}
}

func TestChatCompletionsClientRetriesOnlyBeforeFirstStreamEvent(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	stream := NewChatCompletionsClient(server.URL, "key", nil).Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	if got := stream.Current().Delta; got != "ok" {
		t.Fatalf("Delta = %q, want ok", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestChatCompletionsClientDoesNotRetryAfterFirstStreamEvent(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {not-json}\n\n")
	}))
	t.Cleanup(server.Close)

	stream := NewChatCompletionsClient(server.URL, "key", nil).Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if !stream.Next() {
		t.Fatalf("first Next() = false, err=%v", stream.Err())
	}
	if stream.Next() {
		t.Fatal("second Next() = true after malformed event")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "parse chat stream event") {
		t.Fatalf("Err() = %v, want parse error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want no retry after first event", attempts)
	}
}

func TestChatCompletionsClientRetriesEOFBeforeFirstStreamEvent(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	stream := NewChatCompletionsClient(server.URL, "key", nil).Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	if got := stream.Current().Delta; got != "ok" {
		t.Fatalf("Delta = %q, want ok", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want retry before first stream event", attempts)
	}
}

func TestChatCompletionsClientFailsOnProviderStreamErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"quota exceeded\",\"type\":\"rate_limit\",\"code\":\"429\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	stream := NewChatCompletionsClient(server.URL, "key", nil).Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if stream.Next() {
		t.Fatal("Next() = true for provider stream error")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("Err() = %v, want provider stream error", err)
	}
}

func TestSSRFProtectedHTTPClientBlocksLoopbackResolution(t *testing.T) {
	client := NewChatCompletionsClient("http://127.0.0.1:9/v1", "", NewSSRFProtectedHTTPClient(0, nil))
	stream := client.Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if stream.Next() {
		t.Fatal("Next() = true for blocked loopback api_base")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "blocked private IP") {
		t.Fatalf("Err() = %v, want blocked private IP error", err)
	}
}

func TestSSRFProtectedHTTPClientBlocksAllowedHostOnUnexpectedPort(t *testing.T) {
	client := NewChatCompletionsClient("http://host.containers.internal:5432/v1", "", NewSSRFProtectedHTTPClient(0, nil))
	stream := client.Stream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if stream.Next() {
		t.Fatal("Next() = true for host.containers.internal on blocked port")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("Err() = %v, want port allowlist error", err)
	}
}
