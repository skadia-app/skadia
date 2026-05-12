package llmwire

import "encoding/json"

type ChatRequest struct {
	Model           string           `json:"model"`
	Messages        []ChatMessage    `json:"messages"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	TopP            *float64         `json:"top_p,omitempty"`
	PresencePenalty *float64         `json:"presence_penalty,omitempty"`
	MaxTokens       *int             `json:"max_tokens,omitempty"`
}

type ChatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
}

type ChatToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"arguments"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolCallDelta struct {
	Index int64  `json:"index"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Args  string `json:"arguments,omitempty"`
}

type ChatStreamEvent struct {
	Delta        string          `json:"delta,omitempty"`
	ToolCalls    []ToolCallDelta `json:"tool_calls,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	Usage        *ChatUsage      `json:"usage,omitempty"`
	Error        *ChatError      `json:"error,omitempty"`
	Done         bool            `json:"done,omitempty"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

type ChatStream interface {
	Next() bool
	Current() ChatStreamEvent
	Err() error
}
