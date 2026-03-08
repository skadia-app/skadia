package model

import (
	"encoding/json"
	"time"
)

type ChatSession struct {
	ID        string  `json:"id" db:"id"`
	UserID    string  `json:"user_id" db:"user_id"`
	Title     *string `json:"title" db:"title"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type ChatMessage struct {
	ID        string          `json:"id" db:"id"`
	SessionID string          `json:"session_id" db:"session_id"`
	Role      string          `json:"role" db:"role"` // user, assistant, tool
	Content   string          `json:"content" db:"content"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty" db:"tool_calls"`
	Sources   json.RawMessage `json:"sources,omitempty" db:"sources"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}

// WebSocket message types
type WSChatMessage struct {
	Type      string `json:"type"`                 // chat, chat.cancel
	ID        string `json:"id"`                   // message UUID
	SessionID string `json:"session_id"`           // session UUID
	Message   string `json:"message,omitempty"`
}

type WSChatResponse struct {
	Type    string          `json:"type"`             // chat.thinking, chat.chunk, chat.tool_call, chat.tool_result, chat.done, chat.error
	ID      string          `json:"id"`               // message UUID
	Delta   string          `json:"delta,omitempty"`
	Step    string          `json:"step,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Summary string          `json:"summary,omitempty"`
	Sources []string        `json:"sources,omitempty"`
	Error   string          `json:"error,omitempty"`
}
