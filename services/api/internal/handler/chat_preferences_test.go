package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestChatPreferencesDefaultsAndPut(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	req := chatPrefsRequest(t, http.MethodGet, "/preferences", userID, nil)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", w.Code, w.Body.String())
	}
	var defaults chatPreferencesResponse
	if err := json.NewDecoder(w.Body).Decode(&defaults); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	if defaults.ChatExposeDocumentExtractedData || defaults.ChatWriteToolsEnabled || len(defaults.ChatAllowedWriteTools) != 0 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	body := []byte(`{
		"chat_expose_document_extracted_data": true,
		"chat_write_tools_enabled": true,
		"chat_allowed_write_tools": ["rename_activity", "set_activity_gear", "rename_activity"]
	}`)
	req = chatPrefsRequest(t, http.MethodPut, "/preferences", userID, body)
	w = httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", w.Code, w.Body.String())
	}
	var saved chatPreferencesResponse
	if err := json.NewDecoder(w.Body).Decode(&saved); err != nil {
		t.Fatalf("decode saved: %v", err)
	}
	if !saved.ChatExposeDocumentExtractedData || !saved.ChatWriteToolsEnabled {
		t.Fatalf("saved flags not set: %#v", saved)
	}
	if got := saved.ChatAllowedWriteTools; len(got) != 2 || got[0] != "rename_activity" || got[1] != "set_activity_gear" {
		t.Fatalf("saved allowed tools = %#v", got)
	}
}

func TestChatPreferencesRejectsUnknownWriteTool(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	body := []byte(`{
		"chat_write_tools_enabled": true,
		"chat_allowed_write_tools": ["rename_activity", "delete_activity"]
	}`)
	req := chatPrefsRequest(t, http.MethodPut, "/preferences", userID, body)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d body=%s, want 400", w.Code, w.Body.String())
	}
}

func TestChatPreferencesRequiresUser(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET status = %d body=%s, want 401", w.Code, w.Body.String())
	}
}

func TestConfirmChatToolCallResolvesPending(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)
	h.Confirm = agent.NewConfirmationManager(time.Minute)
	session, err := h.DB.CreateSession(context.Background(), userID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ch, err := h.Confirm.Register(context.Background(), &agent.PendingConfirm{
		SessionID:  session.ID,
		ToolCallID: "call-1",
		Tool:       "create_gear",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	req := chatPrefsRequest(t, http.MethodPost, "/sessions/"+session.ID+"/confirm", userID, []byte(`{"tool_call_id":"call-1","confirmed":true,"args_overrides":{"name":"Bike"}}`))
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("confirm status = %d body=%s", w.Code, w.Body.String())
	}
	resp := <-ch
	if !resp.Confirmed || resp.Source != "user" || string(resp.ArgsOverrides) != `{"name":"Bike"}` {
		t.Fatalf("confirm response = %#v", resp)
	}
}

func TestConfirmChatToolCallRejectsForeignSession(t *testing.T) {
	pool := fusionTestPool(t)
	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)
	h := newTestHandler(t, pool)
	h.Confirm = agent.NewConfirmationManager(time.Minute)
	session, err := h.DB.CreateSession(context.Background(), userA)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := chatPrefsRequest(t, http.MethodPost, "/sessions/"+session.ID+"/confirm", userB, []byte(`{"tool_call_id":"call-1","confirmed":true}`))
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("confirm status = %d body=%s, want 403", w.Code, w.Body.String())
	}
}

func TestConfirmChatToolCallUnknownReturns404(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)
	h.Confirm = agent.NewConfirmationManager(time.Minute)
	session, err := h.DB.CreateSession(context.Background(), userID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := chatPrefsRequest(t, http.MethodPost, "/sessions/"+session.ID+"/confirm", userID, []byte(`{"tool_call_id":"missing","confirmed":true}`))
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("confirm status = %d body=%s, want 404", w.Code, w.Body.String())
	}
}

func TestSendChatMessageUsesGoAgentAndPersistsAssistant(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.test/v1', 'model', true, 'passed', '{}'::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			enabled = EXCLUDED.enabled,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report
	`); err != nil {
		t.Fatalf("seed tool_agent config: %v", err)
	}
	h.Agent = agent.New(agent.Config{
		LLMResolver: agent.NewLLMResolver(h.DB, agent.LLMResolverConfig{BaseURL: "http://llm.test/v1", APIKey: "key", Model: "model"}),
		StreamChat: func(ctx context.Context, cfg agent.LLMConfig, params llmwire.ChatRequest) agent.ChatStream {
			return &handlerFakeStream{events: []llmwire.ChatStreamEvent{{
				Delta:        "Hallo.",
				FinishReason: "stop",
			}}}
		},
	})
	session, err := h.DB.CreateSession(context.Background(), userID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := chatPrefsRequest(t, http.MethodPost, "/sessions/"+session.ID+"/send", userID, []byte(`{"message_id":"msg-1","message":"analysiere training"}`))
	w := httptest.NewRecorder()
	h.ChatRoutes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("send status = %d body=%s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, `"type":"chat.chunk"`) || !strings.Contains(body, `"delta":"Hallo."`) {
		t.Fatalf("SSE body missing assistant chunk: %s", body)
	}

	messages, err := h.DB.GetMessages(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" || messages[1].Content != "Hallo." {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestChatHistoryPayloadDropsOnlyCurrentMessage(t *testing.T) {
	got := chatHistoryPayload([]*model.ChatMessage{
		{Role: "user", Content: "gleicher Text"},
		{Role: "assistant", Content: "alte Antwort"},
		{Role: "tool", Content: "ignorieren"},
		{Role: "user", Content: "gleicher Text"},
	}, "gleicher Text")

	if len(got) != 2 {
		t.Fatalf("history len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "gleicher Text" {
		t.Fatalf("first history item = %#v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "alte Antwort" {
		t.Fatalf("second history item = %#v", got[1])
	}
}

func chatPrefsRequest(t *testing.T, method, target, userID string, body []byte) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if userID != "" {
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
	}
	return req
}

type handlerFakeStream struct {
	events []llmwire.ChatStreamEvent
	idx    int
}

func (s *handlerFakeStream) Next() bool {
	return s.idx < len(s.events)
}

func (s *handlerFakeStream) Current() llmwire.ChatStreamEvent {
	event := s.events[s.idx]
	s.idx++
	return event
}

func (s *handlerFakeStream) Err() error { return nil }
