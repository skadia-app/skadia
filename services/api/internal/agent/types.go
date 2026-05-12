package agent

import (
	"encoding/json"
	"time"
)

type ChatRequest struct {
	UserID    string               `json:"user_id"`
	SessionID string               `json:"session_id"`
	MessageID string               `json:"message_id"`
	Message   string               `json:"message"`
	History   []ChatHistoryMessage `json:"history,omitempty"`
}

type ChatHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	EventThinking           = "chat.thinking"
	EventChunk              = "chat.chunk"
	EventToolCall           = "chat.tool_call"
	EventToolResult         = "chat.tool_result"
	EventToolConfirmRequest = "chat.tool_confirm_request"
	EventToolConfirmed      = "chat.tool_confirmed"
	EventToolDeclined       = "chat.tool_declined"
	EventDone               = "chat.done"
	EventError              = "chat.error"
)

type SSEEvent struct {
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	Step          string          `json:"step,omitempty"`
	Delta         string          `json:"delta,omitempty"`
	ToolCallID    string          `json:"tool_call_id,omitempty"`
	Tool          string          `json:"tool,omitempty"`
	Args          json.RawMessage `json:"args,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	ExpiresAt     *time.Time      `json:"expires_at_iso,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	Sources       []string        `json:"sources,omitempty"`
	Hops          int             `json:"hops,omitempty"`
	ContextWindow int             `json:"context_window,omitempty"`
	Usage         *TokenUsage     `json:"usage,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
