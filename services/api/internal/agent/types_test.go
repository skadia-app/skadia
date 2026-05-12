package agent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSSEToolConfirmRequestMarshalsStableSchema(t *testing.T) {
	expires := time.Date(2026, 5, 12, 14, 30, 0, 0, time.UTC)
	evt := SSEEvent{
		Type:       EventToolConfirmRequest,
		ID:         "msg-1",
		ToolCallID: "call-1",
		Tool:       "create_gear",
		Args:       json.RawMessage(`{"name":"Kette"}`),
		Summary:    "Erstellt Ausruestung Kette.",
		Preview:    map[string]any{"arguments": map[string]any{"name": "Kette"}, "tool": "create_gear"},
		ExpiresAt:  &expires,
	}

	got, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"type":"chat.tool_confirm_request","id":"msg-1","tool_call_id":"call-1","tool":"create_gear","args":{"name":"Kette"},"summary":"Erstellt Ausruestung Kette.","preview":{"arguments":{"name":"Kette"},"tool":"create_gear"},"expires_at_iso":"2026-05-12T14:30:00Z"}`
	if string(got) != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
}

func TestChatRequestMirrorsPythonPayload(t *testing.T) {
	raw := []byte(`{"user_id":"u1","session_id":"s1","message_id":"m1","message":"Hallo","history":[{"role":"user","content":"Alt"}]}`)
	var req ChatRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if req.UserID != "u1" || req.SessionID != "s1" || req.MessageID != "m1" || req.Message != "Hallo" {
		t.Fatalf("unexpected request: %+v", req)
	}
	if len(req.History) != 1 || req.History[0].Role != "user" || req.History[0].Content != "Alt" {
		t.Fatalf("unexpected history: %+v", req.History)
	}
}
