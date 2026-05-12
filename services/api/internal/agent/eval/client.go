package eval

import (
	"context"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

type WireClient struct {
	HTTPClient *http.Client
}

func (c WireClient) RunCase(ctx context.Context, candidate Candidate, tc Case) (Transcript, error) {
	client := llmwire.NewChatCompletionsClient(candidate.APIBase, candidate.APIKey, c.HTTPClient)
	stream := client.Stream(ctx, llmwire.ChatRequest{
		Model: candidate.ModelName,
		Messages: []llmwire.ChatMessage{{
			Role:    "user",
			Content: tc.Prompt,
		}},
		Tools:     evalToolDefinitions(tc),
		MaxTokens: intPtr(512),
	})
	var out Transcript
	for stream.Next() {
		event := stream.Current()
		out.Content += event.Delta
		if event.FinishReason != "" {
			out.FinishReason = event.FinishReason
		}
		for _, call := range event.ToolCalls {
			index := int(call.Index)
			for len(out.ToolCalls) <= index {
				out.ToolCalls = append(out.ToolCalls, ToolCall{})
			}
			if call.Name != "" {
				out.ToolCalls[index].Name += call.Name
			}
			if call.Args != "" {
				out.ToolCalls[index].Arguments += call.Args
			}
		}
	}
	return out, stream.Err()
}

func evalToolDefinitions(tc Case) []llmwire.ToolDefinition {
	if tc.Expectations.RequireToolCall == "" {
		return nil
	}
	return []llmwire.ToolDefinition{{
		Name:        tc.Expectations.RequireToolCall,
		Description: "Evaluation probe tool",
		InputSchema: []byte(`{"type":"object","additionalProperties":true}`),
	}}
}

func intPtr(v int) *int { return &v }
