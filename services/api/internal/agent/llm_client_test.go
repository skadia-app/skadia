package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go"
)

func TestNewLLMClientUsesResolvedBaseURLAndAPIKeyForStreaming(t *testing.T) {
	var gotAuth string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":0,"model":"test-model","choices":[{"index":0,"delta":{"content":"Hallo"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":0,"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	client := NewLLMClient(LLMConfig{
		BaseURL: server.URL + "/v1",
		APIKey:  "resolved-key",
		Model:   "test-model",
	})

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String("Hallo"),
				},
			},
		}},
		Model: "test-model",
	})
	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		if !acc.AddChunk(stream.Current()) {
			t.Fatal("accumulator rejected chunk")
		}
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
	if got := acc.Choices[0].Message.Content; got != "Hallo" {
		t.Fatalf("accumulated content = %q, want Hallo", got)
	}
}
