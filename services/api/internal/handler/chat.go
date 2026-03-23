package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// chatRateLimiter tracks per-user message rates in memory.
type chatRateLimiter struct {
	mu     sync.Mutex
	counts map[string][]time.Time
}

var globalChatRateLimiter = &chatRateLimiter{
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
	r.Get("/sessions", h.ListChatSessions)
	r.Post("/sessions", h.CreateChatSession)
	r.Get("/sessions/{id}/messages", h.GetChatMessages)
	r.Delete("/sessions/{id}", h.DeleteChatSession)
	r.Post("/sessions/{id}/send", h.SendChatMessage)
	return r
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

	// Build history slice for ML service (exclude the just-added user message)
	historyPayload := make([]map[string]string, 0, len(history))
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			historyPayload = append(historyPayload, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}

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

	// Forward to ML service
	mlPayload, _ := json.Marshal(map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"message_id": req.MessageID,
		"message":    req.Message,
		"history":    historyPayload,
	})

	mlBaseURL := "http://ml:8000"
	if h.Config != nil && h.Config.MLServiceURL != "" {
		mlBaseURL = h.Config.MLServiceURL
	}
	mlReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		mlBaseURL+"/ml/chat", strings.NewReader(string(mlPayload)))
	if err != nil {
		slog.Error("build ML chat request failed", "error", err)
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(model.WSChatResponse{
			Type:  "chat.error",
			ID:    req.MessageID,
			Error: "failed to reach ML service",
		}))
		flusher.Flush()
		return
	}
	mlReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	mlResp, err := client.Do(mlReq)
	if err != nil {
		slog.Error("ML chat request failed", "error", err)
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(model.WSChatResponse{
			Type:  "chat.error",
			ID:    req.MessageID,
			Error: fmt.Sprintf("ML service unavailable: %v", err),
		}))
		flusher.Flush()
		return
	}
	defer mlResp.Body.Close()

	// Stream SSE from ML service to client; accumulate assistant reply
	var assistantContent strings.Builder
	var sourcesRaw []string

	scanner := bufio.NewScanner(mlResp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Parse the event to accumulate content and detect done
		var evt model.WSChatResponse
		if err := json.Unmarshal([]byte(data), &evt); err == nil {
			switch evt.Type {
			case "chat.chunk":
				assistantContent.WriteString(evt.Delta)
			case "chat.done":
				sourcesRaw = evt.Sources
			}
		}

		// Forward to client verbatim
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Stop after done or error
		if evt.Type == "chat.done" || evt.Type == "chat.error" {
			break
		}
	}

	// Persist assistant reply
	if content := assistantContent.String(); content != "" {
		var sourcesJSON []byte
		if len(sourcesRaw) > 0 {
			sourcesJSON, _ = json.Marshal(sourcesRaw)
		}
		assistantMsg := &model.ChatMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   content,
			Sources:   sourcesJSON,
		}
		if _, err := h.DB.CreateMessage(r.Context(), assistantMsg); err != nil {
			slog.Warn("persist assistant chat message failed", "error", err)
		}
	}
}

// mustJSON marshals v to JSON, returning "{}" on error.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
