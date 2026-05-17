package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type LLMRoleResolver interface {
	Resolve(ctx context.Context, userID, role string) (agent.LLMConfig, error)
}

type aiBrokerMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type aiBrokerChatRequest struct {
	Role     string            `json:"role"`
	TaskType string            `json:"task_type"`
	Messages []aiBrokerMessage `json:"messages"`
}

type aiBrokerChatResponse struct {
	Status string `json:"status"`
	Role   string `json:"role"`
	Text   string `json:"text,omitempty"`
	Error  string `json:"error,omitempty"`
}

type aiBrokerRoleResponse struct {
	Roles map[string]aiBrokerRoleStatus `json:"roles"`
}

type aiBrokerRoleStatus struct {
	Status string `json:"status"`
}

const aiBrokerServiceUserID = "00000000-0000-0000-0000-000000000000"

var aiBrokerAllowedRoles = map[string]bool{
	model.LLMRoleDocumentExtract: true,
	model.LLMRoleMarkdownExtract: true,
	model.LLMRoleVision:          true,
}

var aiBrokerTaskTokenLimits = map[string]map[string]int{
	model.LLMRoleDocumentExtract: {
		"document_extract": 2048,
	},
	model.LLMRoleMarkdownExtract: {
		"markdown_extract": 2048,
	},
	model.LLMRoleVision: {
		"vision":          2048,
		"document_page":   4096,
		"document_vision": 4096,
		"invoice_page":    4096,
		"invoice_parts":   2048,
	},
}

var aiBrokerTaskTimeouts = map[string]time.Duration{
	"document_page":   5 * time.Minute,
	"document_vision": 5 * time.Minute,
	"invoice_page":    5 * time.Minute,
}

// HandleAIBrokerChat is an internal ML-service endpoint. It accepts only the
// requested Skadia role, task type, and message payload; provider URLs and
// credentials stay inside the API service.
func (h *Handler) HandleAIBrokerChat(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAIBroker(w, r) {
		return
	}

	var req aiBrokerChatRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, aiBrokerChatResponse{
			Status: "bad_request",
			Error:  "invalid broker request",
		})
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.TaskType = strings.TrimSpace(req.TaskType)
	maxTokens, validTask := aiBrokerMaxTokens(req.Role, req.TaskType)
	if !validTask || len(req.Messages) == 0 {
		httputil.WriteJSON(w, http.StatusBadRequest, aiBrokerChatResponse{
			Status: "bad_request",
			Role:   req.Role,
			Error:  "invalid role/task_type combination or messages",
		})
		return
	}
	if h.LLMResolver == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, aiBrokerChatResponse{
			Status: "unavailable",
			Role:   req.Role,
			Error:  "LLM resolver unavailable",
		})
		return
	}

	cfg, err := h.LLMResolver.Resolve(r.Context(), aiBrokerServiceUserID, req.Role)
	if err != nil {
		status := aiBrokerStatusForResolveError(err)
		code := http.StatusFailedDependency
		if status == "unavailable" {
			code = http.StatusServiceUnavailable
		}
		httputil.WriteJSON(w, code, aiBrokerChatResponse{
			Status: status,
			Role:   req.Role,
			Error:  err.Error(),
		})
		return
	}

	text, err := h.forwardAIBrokerChat(r.Context(), cfg, req, maxTokens)
	if err != nil {
		httputil.WriteJSON(w, http.StatusBadGateway, aiBrokerChatResponse{
			Status: "unavailable",
			Role:   req.Role,
			Error:  err.Error(),
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, aiBrokerChatResponse{
		Status: "ok",
		Role:   req.Role,
		Text:   text,
	})
}

// HandleAIBrokerRoleStatus reports internal ML role readiness without leaking
// provider URLs, model names, or API keys.
func (h *Handler) HandleAIBrokerRoleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAIBroker(w, r) {
		return
	}
	out := aiBrokerRoleResponse{Roles: make(map[string]aiBrokerRoleStatus, len(aiBrokerAllowedRoles))}
	for _, role := range []string{model.LLMRoleDocumentExtract, model.LLMRoleMarkdownExtract, model.LLMRoleVision} {
		status := "unavailable"
		if h.LLMResolver != nil {
			if _, err := h.LLMResolver.Resolve(r.Context(), aiBrokerServiceUserID, role); err != nil {
				status = aiBrokerStatusForResolveError(err)
			} else {
				status = "configured"
			}
		}
		out.Roles[role] = aiBrokerRoleStatus{Status: status}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) authorizeAIBroker(w http.ResponseWriter, r *http.Request) bool {
	secret := ""
	if h.Config != nil {
		secret = strings.TrimSpace(h.Config.MLLLMBrokerToken)
		if secret == "" {
			secret = strings.TrimSpace(h.Config.MLCallbackSecret)
		}
	}
	if secret == "" {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  "ML service token is not configured",
		})
		return false
	}
	got := r.Header.Get("X-ML-Service-Token")
	if got == "" {
		got = r.Header.Get("X-ML-Callback-Secret")
	}
	if !aiBrokerTokenMatches(got, secret) {
		httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{
			"status": "unauthorized",
			"error":  "unauthorized",
		})
		return false
	}
	return true
}

func aiBrokerTokenMatches(got, secret string) bool {
	gotHash := sha256.Sum256([]byte(got))
	secretHash := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(gotHash[:], secretHash[:]) == 1
}

func aiBrokerStatusForResolveError(err error) string {
	if err == nil {
		return "configured"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not configured") ||
		strings.Contains(msg, "not passed") ||
		strings.Contains(msg, "incomplete") {
		return "not_configured"
	}
	return "unavailable"
}

func (h *Handler) forwardAIBrokerChat(ctx context.Context, cfg agent.LLMConfig, req aiBrokerChatRequest, maxTokens int) (string, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		return "", errors.New("resolved LLM config is incomplete")
	}
	body := providerChatRequest{
		Model:       cfg.Model,
		Messages:    req.Messages,
		Stream:      false,
		Temperature: floatPtr(0.1),
		MaxTokens:   intPtr(maxTokens),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := h.AIBrokerHTTPClient
	if client == nil {
		client = llmwire.NewSSRFProtectedHTTPClient(aiBrokerTimeoutForTask(req.TaskType), nil)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rawErr)))
	}
	var out providerChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("provider returned no choices")
	}
	return out.Choices[0].Message.text()
}

type providerChatRequest struct {
	Model       string            `json:"model"`
	Messages    []aiBrokerMessage `json:"messages"`
	Stream      bool              `json:"stream"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
}

type providerChatResponse struct {
	Choices []struct {
		Message providerChatMessage `json:"message"`
	} `json:"choices"`
}

type providerChatMessage struct {
	Content json.RawMessage `json:"content"`
}

func (m providerChatMessage) text() (string, error) {
	if len(m.Content) == 0 {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		return text, nil
	}
	return string(m.Content), nil
}

func aiBrokerMaxTokens(role, taskType string) (int, bool) {
	tasks, ok := aiBrokerTaskTokenLimits[role]
	if !ok {
		return 0, false
	}
	maxTokens, ok := tasks[taskType]
	return maxTokens, ok
}

func aiBrokerTimeoutForTask(taskType string) time.Duration {
	if timeout, ok := aiBrokerTaskTimeouts[taskType]; ok {
		return timeout
	}
	return 120 * time.Second
}

func floatPtr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}
