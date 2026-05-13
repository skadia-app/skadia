package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

func TestChatCompletionsClientUsesResolvedBaseURLAndAPIKeyForStreaming(t *testing.T) {
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
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"Hallo"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	client := llmwire.NewChatCompletionsClient(server.URL+"/v1", "resolved-key", server.Client())

	stream := client.Stream(context.Background(), llmwire.ChatRequest{
		Model:    "test-model",
		Messages: []llmwire.ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	var acc string
	for stream.Next() {
		acc += stream.Current().Delta
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err: %v", err)
	}
	if gotAuth != "Bearer resolved-key" {
		t.Fatalf("Authorization = %q, want Bearer resolved-key", gotAuth)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotBody["stream"] != true {
		t.Fatalf("stream = %#v, want true", gotBody["stream"])
	}
	if got := acc; got != "Hallo" {
		t.Fatalf("accumulated content = %q, want Hallo", got)
	}
}

func TestNewLLMClientBlocksLoopbackBaseURL(t *testing.T) {
	client := NewLLMClient(LLMConfig{
		BaseURL: "http://127.0.0.1:9/v1",
		Model:   "test-model",
	})

	stream := client.Stream(context.Background(), llmwire.ChatRequest{
		Model:    "test-model",
		Messages: []llmwire.ChatMessage{{Role: "user", Content: "Hallo"}},
	})
	if stream.Next() {
		t.Fatal("Next() = true for blocked loopback api_base")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "blocked private IP") {
		t.Fatalf("Err() = %v, want blocked private IP error", err)
	}
}
