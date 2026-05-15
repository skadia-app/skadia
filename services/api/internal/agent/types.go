package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
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
	EventToolsLoaded        = "chat.tools_loaded"
	EventToolCall           = "chat.tool_call"
	EventToolResult         = "chat.tool_result"
	EventToolConfirmRequest = "chat.tool_confirm_request"
	EventToolConfirmed      = "chat.tool_confirmed"
	EventToolDeclined       = "chat.tool_declined"
	EventUserQuestion       = "chat.user_question"
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
	Tools         []string        `json:"tools,omitempty"`
	Args          json.RawMessage `json:"args,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	Preview       any             `json:"preview,omitempty"`
	Question      string          `json:"question,omitempty"`
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

type ToolRuntime interface {
	AddEnabledWriteTools(filter []string, prefs *ChatPreferences) []string
	Definitions(filter []string, prefs *ChatPreferences) []llmwire.ToolDefinition
	Execute(ctx context.Context, req ToolRuntimeRequest, sink SSESink) (ToolRuntimeResult, error)
}

type ToolRuntimeRequest struct {
	UserID     string
	SessionID  string
	MessageID  string
	ToolCallID string
	ToolName   string
	Args       json.RawMessage
}

type ToolRuntimeResult struct {
	Result   any
	Summary  string
	AskUser  bool
	Question string
}

type ChatStream interface {
	Next() bool
	Current() llmwire.ChatStreamEvent
	Err() error
}

type ChatStreamFunc func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream
