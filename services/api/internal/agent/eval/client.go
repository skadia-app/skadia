package eval

import (
	"context"
	"fmt"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

type WireClient struct {
	HTTPClient *http.Client
}

const maxEvalToolCallIndex = 32

func (c WireClient) RunCase(ctx context.Context, candidate Candidate, tc Case) (Transcript, error) {
	client := llmwire.NewChatCompletionsClient(candidate.APIBase, candidate.APIKey, c.HTTPClient)
	message := llmwire.ChatMessage{
		Role:    "user",
		Content: tc.Prompt,
	}
	if tc.ImageDataURL != "" {
		message.Content = ""
		message.ContentParts = []llmwire.ChatContentPart{
			{Type: "text", Text: tc.Prompt},
			{Type: "image_url", ImageURL: &llmwire.ChatImageURL{URL: tc.ImageDataURL}},
		}
	}
	stream := client.Stream(ctx, llmwire.ChatRequest{
		Model:     candidate.ModelName,
		Messages:  []llmwire.ChatMessage{message},
		Tools:     evalToolDefinitions(tc),
		MaxTokens: intPtr(512),
	})
	var out Transcript
	for stream.Next() {
		event := stream.Current()
		if event.Error != nil {
			return out, fmt.Errorf("provider stream error: %s", event.Error.Message)
		}
		out.Content += event.Delta
		if event.FinishReason != "" {
			out.FinishReason = event.FinishReason
		}
		for _, call := range event.ToolCalls {
			if call.Index < 0 || call.Index > maxEvalToolCallIndex {
				return out, fmt.Errorf("provider tool call index %d is outside eval bounds", call.Index)
			}
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
		InputSchema: evalToolSchema(tc),
	}}
}

func evalToolSchema(tc Case) []byte {
	if len(tc.Expectations.ToolArgsSchema) == 0 {
		return []byte(`{"type":"object","additionalProperties":true}`)
	}
	return tc.Expectations.ToolArgsSchema
}

func intPtr(v int) *int { return &v }
