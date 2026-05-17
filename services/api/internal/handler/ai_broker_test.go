package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
)

type fakeAIBrokerResolver struct {
	cfg agent.LLMConfig
	err error
}

func (r fakeAIBrokerResolver) Resolve(ctx context.Context, userID, role string) (agent.LLMConfig, error) {
	if r.err != nil {
		return agent.LLMConfig{}, r.err
	}
	return r.cfg, nil
}

func TestAIBrokerRequiresMLServiceToken(t *testing.T) {
	h := &Handler{
		Config:      &HandlerConfig{MLCallbackSecret: "secret"},
		LLMResolver: fakeAIBrokerResolver{},
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(`{"role":"document_extract","task_type":"document_extract","messages":[]}`))
	rec := httptest.NewRecorder()

	h.HandleAIBrokerChat(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAIBrokerUsesDedicatedBrokerTokenWhenConfigured(t *testing.T) {
	h := &Handler{
		Config:      &HandlerConfig{MLCallbackSecret: "callback-secret", MLLLMBrokerToken: "broker-secret"},
		LLMResolver: fakeAIBrokerResolver{},
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(`{"role":"document_extract","task_type":"document_extract","messages":[]}`))
	req.Header.Set("X-ML-Service-Token", "callback-secret")
	rec := httptest.NewRecorder()
	h.HandleAIBrokerChat(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("callback token status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(`{"role":"document_extract","task_type":"document_extract","messages":[]}`))
	req.Header.Set("X-ML-Service-Token", "broker-secret")
	rec = httptest.NewRecorder()
	h.HandleAIBrokerChat(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("broker token status = %d, want request to reach validation with %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAIBrokerRejectsProviderURLInRequest(t *testing.T) {
	h := &Handler{
		Config:      &HandlerConfig{MLCallbackSecret: "secret"},
		LLMResolver: fakeAIBrokerResolver{},
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(`{"role":"document_extract","task_type":"document_extract","provider_url":"http://attacker","messages":[]}`))
	req.Header.Set("X-ML-Service-Token", "secret")
	rec := httptest.NewRecorder()

	h.HandleAIBrokerChat(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAIBrokerResolvesRoleAndForwardsWithoutProviderURLFromML(t *testing.T) {
	var providerBody map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer resolved-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&providerBody); err != nil {
			t.Fatalf("decode provider body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"products":[]}`}},
			},
		})
	}))
	defer provider.Close()

	h := &Handler{
		Config:             &HandlerConfig{MLCallbackSecret: "secret"},
		AIBrokerHTTPClient: provider.Client(),
		LLMResolver: fakeAIBrokerResolver{cfg: agent.LLMConfig{
			BaseURL: provider.URL + "/v1",
			APIKey:  "resolved-key",
			Model:   "resolved-model",
		}},
	}

	body := `{"role":"document_extract","task_type":"document_extract","messages":[{"role":"user","content":"invoice"}]}`
	req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(body))
	req.Header.Set("X-ML-Service-Token", "secret")
	rec := httptest.NewRecorder()

	h.HandleAIBrokerChat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if providerBody["model"] != "resolved-model" {
		t.Fatalf("provider model = %v, want resolved-model", providerBody["model"])
	}
	if _, ok := providerBody["provider_url"]; ok {
		t.Fatalf("provider body leaked provider_url: %#v", providerBody)
	}
	var out struct {
		Status string `json:"status"`
		Role   string `json:"role"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "ok" || out.Role != "document_extract" || out.Text != `{"products":[]}` {
		t.Fatalf("response = %#v", out)
	}
}

func TestAIBrokerRejectsTaskRoleMismatch(t *testing.T) {
	h := &Handler{
		Config:      &HandlerConfig{MLCallbackSecret: "secret"},
		LLMResolver: fakeAIBrokerResolver{},
	}

	tests := []struct {
		name     string
		role     string
		taskType string
	}{
		{name: "document role cannot run markdown extraction", role: "document_extract", taskType: "markdown_extract"},
		{name: "markdown role cannot run document extraction", role: "markdown_extract", taskType: "document_extract"},
		{name: "document role cannot run document page vision", role: "document_extract", taskType: "document_page"},
		{name: "document role cannot run structured document vision", role: "document_extract", taskType: "document_vision"},
		{name: "document role cannot run invoice page vision", role: "document_extract", taskType: "invoice_page"},
		{name: "vision role cannot run text document extraction", role: "vision", taskType: "document_extract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"role":"` + tt.role + `","task_type":"` + tt.taskType + `","messages":[{"role":"user","content":"x"}]}`
			req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(body))
			req.Header.Set("X-ML-Service-Token", "secret")
			rec := httptest.NewRecorder()

			h.HandleAIBrokerChat(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s, want %d", rec.Code, rec.Body.String(), http.StatusBadRequest)
			}
		})
	}
}

func TestAIBrokerAppliesTaskSpecificTokenLimits(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		taskType  string
		maxTokens float64
	}{
		{name: "document extraction", role: "document_extract", taskType: "document_extract", maxTokens: 2048},
		{name: "markdown extraction", role: "markdown_extract", taskType: "markdown_extract", maxTokens: 2048},
		{name: "vision document image", role: "vision", taskType: "vision", maxTokens: 2048},
		{name: "document page vision", role: "vision", taskType: "document_page", maxTokens: 4096},
		{name: "structured document vision", role: "vision", taskType: "document_vision", maxTokens: 4096},
		{name: "invoice page vision", role: "vision", taskType: "invoice_page", maxTokens: 4096},
		{name: "invoice parts vision", role: "vision", taskType: "invoice_parts", maxTokens: 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var providerBody map[string]any
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&providerBody); err != nil {
					t.Fatalf("decode provider body: %v", err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": "ok"}},
					},
				})
			}))
			defer provider.Close()

			h := &Handler{
				Config:             &HandlerConfig{MLCallbackSecret: "secret"},
				AIBrokerHTTPClient: provider.Client(),
				LLMResolver: fakeAIBrokerResolver{cfg: agent.LLMConfig{
					BaseURL: provider.URL + "/v1",
					Model:   "resolved-model",
				}},
			}
			body := `{"role":"` + tt.role + `","task_type":"` + tt.taskType + `","messages":[{"role":"user","content":"x"}]}`
			req := httptest.NewRequest(http.MethodPost, "/internal/ai-broker/chat", strings.NewReader(body))
			req.Header.Set("X-ML-Service-Token", "secret")
			rec := httptest.NewRecorder()

			h.HandleAIBrokerChat(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
			}
			if providerBody["max_tokens"] != tt.maxTokens {
				t.Fatalf("max_tokens = %v, want %v", providerBody["max_tokens"], tt.maxTokens)
			}
		})
	}
}

func TestAIBrokerUsesLongerTimeoutForVisionPageTasks(t *testing.T) {
	if got := aiBrokerTimeoutForTask("document_page"); got != 5*time.Minute {
		t.Fatalf("document_page timeout = %s, want 5m", got)
	}
	if got := aiBrokerTimeoutForTask("document_vision"); got != 5*time.Minute {
		t.Fatalf("document_vision timeout = %s, want 5m", got)
	}
	if got := aiBrokerTimeoutForTask("invoice_page"); got != 5*time.Minute {
		t.Fatalf("invoice_page timeout = %s, want 5m", got)
	}
	if got := aiBrokerTimeoutForTask("markdown_extract"); got != 120*time.Second {
		t.Fatalf("markdown_extract timeout = %s, want 120s", got)
	}
}
