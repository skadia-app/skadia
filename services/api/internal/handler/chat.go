package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type chatPreferencesResponse struct {
	ChatExposeDocumentExtractedData bool     `json:"chat_expose_document_extracted_data"`
	ChatWriteToolsEnabled           bool     `json:"chat_write_tools_enabled"`
	ChatAllowedWriteTools           []string `json:"chat_allowed_write_tools"`
}

type updateChatPreferencesRequest struct {
	ChatExposeDocumentExtractedData bool     `json:"chat_expose_document_extracted_data"`
	ChatWriteToolsEnabled           bool     `json:"chat_write_tools_enabled"`
	ChatAllowedWriteTools           []string `json:"chat_allowed_write_tools"`
}

var chatWriteToolCatalog = map[string]bool{
	"create_gear":                         true,
	"update_gear_metadata":                true,
	"retire_gear":                         true,
	"set_activity_gear":                   true,
	"batch_set_activity_gear":             true,
	"update_activity_metadata":            true,
	"rename_activity":                     true,
	"tag_activity":                        true,
	"bulk_tag_activities":                 true,
	"record_health_metric":                true,
	"create_tag":                          true,
	"update_tag":                          true,
	"delete_tag":                          true,
	"merge_tags":                          true,
	"create_automation_rule":              true,
	"update_automation_rule":              true,
	"delete_automation_rule":              true,
	"enable_automation_rule":              true,
	"disable_automation_rule":             true,
	"apply_automation_rule_retroactively": true,
	"add_gear_component":                  true,
	"update_gear_component_status":        true,
	"record_gear_service_event":           true,
	"update_gear_default_predicates":      true,
	"create_routing_profile_variant":      true,
	"save_planned_route":                  true,
	"start_recompute_activities":          true,
}

// chatRateLimiter tracks per-user message rates in memory.
type chatRateLimiter struct {
	mu     sync.Mutex
	counts map[string][]time.Time
}

var globalChatRateLimiter = &chatRateLimiter{
	counts: make(map[string][]time.Time),
}

var chatPreferencesRateLimiter = &chatRateLimiter{
	counts: make(map[string][]time.Time),
}

var confirmRateLimiter = &chatRateLimiter{
	counts: make(map[string][]time.Time),
}

// Allow returns true if the user has not exceeded maxPerMin messages in the
// last minute, and records the current request if allowed.
func (rl *chatRateLimiter) Allow(userID string, maxPerMin int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	times := rl.counts[userID]
	var recent []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	// Cleanup: remove map entry entirely when no recent requests remain, preventing unbounded growth.
	if len(recent) == 0 {
		delete(rl.counts, userID)
	}
	if len(recent) >= maxPerMin {
		rl.counts[userID] = recent
		return false
	}
	rl.counts[userID] = append(recent, now)
	return true
}

// ChatRoutes returns a chi.Router with all chat-related routes.
func (h *Handler) ChatRoutes() http.Handler {
	r := chi.NewRouter()
	r.Get("/preferences", h.GetChatPreferences)
	r.Put("/preferences", h.PutChatPreferences)
	r.Get("/sessions", h.ListChatSessions)
	r.Post("/sessions", h.CreateChatSession)
	r.Get("/sessions/{id}/messages", h.GetChatMessages)
	r.Delete("/sessions/{id}", h.DeleteChatSession)
	r.Post("/sessions/{id}/send", h.SendChatMessage)
	r.Post("/sessions/{id}/confirm", h.ConfirmChatToolCall)
	return r
}

// GetChatPreferences handles GET /api/v1/chat/preferences.
func (h *Handler) GetChatPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if !chatPreferencesRateLimiter.Allow(userID, 60) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too Many Requests", "rate limit exceeded (60/min)", r.URL.Path)
		return
	}

	prefs, err := h.DB.GetChatPrefs(r.Context(), userID)
	if err != nil {
		slog.Error("get chat preferences failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get chat preferences", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, chatPreferencesResponse{
		ChatExposeDocumentExtractedData: prefs.ExposeDocumentExtractedData,
		ChatWriteToolsEnabled:           prefs.ChatWriteToolsEnabled,
		ChatAllowedWriteTools:           prefs.ChatAllowedWriteTools,
	})
}

// PutChatPreferences handles PUT /api/v1/chat/preferences.
func (h *Handler) PutChatPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if !chatPreferencesRateLimiter.Allow(userID, 60) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too Many Requests", "rate limit exceeded (60/min)", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var req updateChatPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	allowedTools, err := validateChatAllowedWriteTools(req.ChatAllowedWriteTools)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	prefs, err := h.DB.UpsertChatPrefs(r.Context(), userID, database.ChatUserPreferences{
		ExposeDocumentExtractedData: req.ChatExposeDocumentExtractedData,
		ChatWriteToolsEnabled:       req.ChatWriteToolsEnabled,
		ChatAllowedWriteTools:       allowedTools,
	})
	if err != nil {
		slog.Error("put chat preferences failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update chat preferences", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, chatPreferencesResponse{
		ChatExposeDocumentExtractedData: prefs.ExposeDocumentExtractedData,
		ChatWriteToolsEnabled:           prefs.ChatWriteToolsEnabled,
		ChatAllowedWriteTools:           prefs.ChatAllowedWriteTools,
	})
}

func validateChatAllowedWriteTools(names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("chat_allowed_write_tools contains an empty tool name")
		}
		if !chatWriteToolCatalog[name] {
			return nil, fmt.Errorf("unknown or unsafe chat write tool: %s", name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, nil
}

// ListChatSessions handles GET /api/v1/chat/sessions
func (h *Handler) ListChatSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	sessions, err := h.DB.ListSessions(r.Context(), userID)
	if err != nil {
		slog.Error("list chat sessions failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list sessions", r.URL.Path)
		return
	}

	if sessions == nil {
		sessions = []*model.ChatSession{}
	}
	httputil.WriteJSON(w, http.StatusOK, sessions)
}

// CreateChatSession handles POST /api/v1/chat/sessions
func (h *Handler) CreateChatSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	session, err := h.DB.CreateSession(r.Context(), userID)
	if err != nil {
		slog.Error("create chat session failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create session", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, session)
}

// GetChatMessages handles GET /api/v1/chat/sessions/{id}/messages
func (h *Handler) GetChatMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	sessionID := chi.URLParam(r, "id")

	// Verify ownership
	if _, err := h.DB.GetSession(r.Context(), sessionID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "session not found", r.URL.Path)
		return
	}

	messages, err := h.DB.GetMessages(r.Context(), sessionID, 200)
	if err != nil {
		slog.Error("get chat messages failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get messages", r.URL.Path)
		return
	}

	if messages == nil {
		messages = []*model.ChatMessage{}
	}
	httputil.WriteJSON(w, http.StatusOK, messages)
}

// DeleteChatSession handles DELETE /api/v1/chat/sessions/{id}
func (h *Handler) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	sessionID := chi.URLParam(r, "id")

	if err := h.DB.DeleteSession(r.Context(), sessionID, userID); err != nil {
		slog.Error("delete chat session failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete session", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SendChatMessage handles POST /api/v1/chat/sessions/{id}/send
// It persists the user message, streams the ML response via SSE, and
// persists the assistant reply when done.
func (h *Handler) SendChatMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Rate limit: 10 messages per minute per user
	if !globalChatRateLimiter.Allow(userID, 10) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too Many Requests", "rate limit exceeded (10/min)", r.URL.Path)
		return
	}

	sessionID := chi.URLParam(r, "id")

	// Verify session ownership
	if _, err := h.DB.GetSession(r.Context(), sessionID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "session not found", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var req struct {
		MessageID string `json:"message_id"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Message == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "message is required", r.URL.Path)
		return
	}
	if len(req.Message) > 4000 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "message too long (max 4000 chars)", r.URL.Path)
		return
	}
	if req.MessageID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "message_id is required", r.URL.Path)
		return
	}

	// Persist user message
	userMsg := &model.ChatMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   req.Message,
	}
	if _, err := h.DB.CreateMessage(r.Context(), userMsg); err != nil {
		slog.Error("persist user chat message failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to persist message", r.URL.Path)
		return
	}

	// Load recent history (last 20 messages)
	history, err := h.DB.GetMessages(r.Context(), sessionID, 20)
	if err != nil {
		slog.Warn("load chat history failed, continuing without history", "error", err)
	}

	if h.Agent == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Unavailable", "chat agent is not configured", r.URL.Path)
		return
	}

	// Build history slice for the agent, excluding only the just-added user message.
	historyPayload := chatHistoryPayload(history, req.Message)

	// SSE response setup
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "Error", "streaming not supported", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("disable chat stream write deadline failed", "error", err)
	}

	var assistantContent strings.Builder
	baseSink := agent.NewHTTPSSESink(w, flusher)
	baseSink.OnChunk(func(delta string) { assistantContent.WriteString(delta) })
	sink := &chatPersistSink{SSESink: baseSink, assistantContent: &assistantContent}
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = sink.WriteRaw(": ping\n\n")
			case <-heartbeatDone:
				return
			case <-r.Context().Done():
				return
			}
		}
	}()
	if err := h.Agent.Stream(r.Context(), agent.ChatRequest{
		UserID:    userID,
		SessionID: sessionID,
		MessageID: req.MessageID,
		Message:   req.Message,
		History:   historyPayload,
	}, sink); err != nil {
		if r.Context().Err() != nil {
			slog.Info("agent chat stream canceled", "error", err)
		} else {
			slog.Error("agent chat stream failed", "error", err)
		}
	}

	// Persist assistant reply
	if content := assistantContent.String(); content != "" {
		var sourcesJSON []byte
		if len(sink.sources) > 0 {
			sourcesJSON, _ = json.Marshal(sink.sources)
		}
		assistantMsg := &model.ChatMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   content,
			Sources:   sourcesJSON,
		}
		persistCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := h.DB.CreateMessage(persistCtx, assistantMsg); err != nil {
			slog.Warn("persist assistant chat message failed", "error", err)
		}
	}
}

type chatPersistSink struct {
	agent.SSESink
	assistantContent *strings.Builder
	sources          []string
}

func (s *chatPersistSink) Send(event agent.SSEEvent) error {
	if event.Type == agent.EventDone {
		s.sources = append([]string(nil), event.Sources...)
	}
	if event.Type == agent.EventUserQuestion && strings.TrimSpace(event.Question) != "" && s.assistantContent != nil {
		if s.assistantContent.Len() > 0 {
			s.assistantContent.WriteString("\n\n")
		}
		s.assistantContent.WriteString(event.Question)
	}
	return s.SSESink.Send(event)
}

func chatHistoryPayload(history []*model.ChatMessage, currentMessage string) []agent.ChatHistoryMessage {
	historyPayload := make([]agent.ChatHistoryMessage, 0, len(history))
	last := len(history) - 1
	for idx, m := range history {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if idx == last && m.Role == "user" && m.Content == currentMessage {
			continue
		}
		historyPayload = append(historyPayload, agent.ChatHistoryMessage{Role: m.Role, Content: m.Content})
	}
	return historyPayload
}

type confirmChatToolCallRequest struct {
	ToolCallID    string          `json:"tool_call_id"`
	Confirmed     bool            `json:"confirmed"`
	ArgsOverrides json.RawMessage `json:"args_overrides,omitempty"`
}

func (h *Handler) ConfirmChatToolCall(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if !confirmRateLimiter.Allow(userID, 30) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too Many Requests", "rate limit exceeded (30/min)", r.URL.Path)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, err := h.DB.GetSession(r.Context(), sessionID, userID); err != nil {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "session not available", r.URL.Path)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var req confirmChatToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if strings.TrimSpace(req.ToolCallID) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "tool_call_id is required", r.URL.Path)
		return
	}
	if h.Confirm == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "confirmation not found", r.URL.Path)
		return
	}
	if len(req.ArgsOverrides) > 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "args_overrides is not supported", r.URL.Path)
		return
	}
	if !h.Confirm.Resolve(sessionID, req.ToolCallID, agent.ConfirmResponse{
		Confirmed: req.Confirmed,
		Source:    "user",
	}) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "confirmation not found", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mustJSON marshals v to JSON, returning "{}" on error.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
