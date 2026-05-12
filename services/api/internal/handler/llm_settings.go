package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	agenteval "github.com/skadia-app/skadia/services/api/internal/agent/eval"
	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetLLMConfig returns the full LLM system configuration.
func (h *Handler) GetLLMConfig(w http.ResponseWriter, r *http.Request) {
	configs, err := h.DB.GetLLMSystemConfig(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get LLM config", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, configs)
}

// PutLLMConfigRole saves the configuration for a model role.
func (h *Handler) PutLLMConfigRole(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if !model.IsValidLLMRole(role) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}

	update, ok := httputil.DecodeJSON[model.LLMSystemConfigUpdate](w, r)
	if !ok {
		return
	}

	var encKey *string
	if update.APIKey != nil && *update.APIKey != "" {
		enc, err := crypto.Encrypt(*update.APIKey, h.Config.LLMEncryptionKey)
		if err != nil {
			slog.Error("failed to encrypt API key", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to encrypt API key", r.URL.Path)
			return
		}
		encKey = &enc
	}

	if err := h.DB.UpsertLLMSystemConfig(r.Context(), role, update, encKey); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save LLM config", r.URL.Path)
		return
	}

	// Regenerate LiteLLM config.yaml
	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config", "error", err)
		// Don't return error - config was saved to DB
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// PatchLLMConfigEvalWaiver records an explicit admin waiver for a role eval gate.
func (h *Handler) PatchLLMConfigEvalWaiver(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if !model.IsValidLLMRole(role) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}
	var req struct {
		AdminReason string `json:"admin_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	reason := strings.TrimSpace(req.AdminReason)
	if reason == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "admin_reason is required", r.URL.Path)
		return
	}
	configs, err := h.DB.GetLLMSystemConfig(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get LLM config", r.URL.Path)
		return
	}
	var current *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == role {
			current = &configs[i]
			break
		}
	}
	if current == nil || !current.Enabled {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "role is not configured", r.URL.Path)
		return
	}
	identity, ok := current.CurrentEvalIdentity()
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "role configuration is incomplete", r.URL.Path)
		return
	}
	report := map[string]any{
		"status":       model.LLMEvalStatusWaived,
		"identity":     identity,
		"admin_reason": reason,
		"waived_at":    time.Now().UTC().Format(time.RFC3339),
	}
	if err := h.DB.UpdateLLMEvalResult(r.Context(), role, model.LLMEvalStatusWaived, report); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save eval waiver", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "waived"})
}

func (h *Handler) PostLLMConfigEval(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if !model.IsValidLLMRole(role) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}
	key := ""
	if h.Config != nil {
		key = h.Config.LLMEncryptionKey
	}
	runner := agenteval.NewRunner(h.DB, agenteval.WireClient{HTTPClient: &http.Client{Timeout: 60 * time.Second}}, agenteval.RunnerOptions{
		EncryptionKey: key,
	})
	report, err := runner.Run(r.Context(), role)
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "Bad Gateway", err.Error(), r.URL.Path)
		return
	}
	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config after eval", "error", err)
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

// GetLLMModels returns available models for a backend (only Ollama supports auto-discovery).
func (h *Handler) GetLLMModels(w http.ResponseWriter, r *http.Request) {
	backend := r.URL.Query().Get("backend")
	apiBase := r.URL.Query().Get("api_base")
	if apiBase == "" {
		apiBase = "http://ollama:11434"
	}

	if backend != "ollama" {
		httputil.WriteJSON(w, http.StatusOK, []model.OllamaModel{})
		return
	}

	// SSRF protection: validate api_base host before making outbound requests.
	if parsed, err := url.Parse(apiBase); err != nil || !validLLMAPIBase(parsed) || llmBackendHostBlocked(parsed.Hostname()) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid or internal api_base URL", r.URL.Path)
		return
	}

	// Query Ollama /api/tags
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiBase + "/api/tags")
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "Bad Gateway", "failed to reach Ollama: "+err.Error(), r.URL.Path)
		return
	}
	defer resp.Body.Close()

	var ollamaResp struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "Bad Gateway", "failed to parse Ollama response", r.URL.Path)
		return
	}

	models := make([]model.OllamaModel, 0, len(ollamaResp.Models))
	for _, m := range ollamaResp.Models {
		models = append(models, model.OllamaModel{
			Name:       m.Name,
			ModifiedAt: m.ModifiedAt,
			Size:       m.Size,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, models)
}

// TestLLMConnection tests connectivity to an LLM backend.
func (h *Handler) TestLLMConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackendType string  `json:"backend_type"`
		APIBase     string  `json:"api_base"`
		ModelName   string  `json:"model_name"`
		APIKey      *string `json:"api_key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	start := time.Now()
	result := model.LLMTestResult{}

	// Translate localhost/127.0.0.1 to host.containers.internal for container-to-host access.
	// The test runs server-side inside the API container, so localhost would refer to the
	// container itself rather than the host machine where llama-server is running.
	testAPIBase := strings.NewReplacer(
		"localhost", "host.containers.internal",
		"127.0.0.1", "host.containers.internal",
	).Replace(req.APIBase)
	if testAPIBase != req.APIBase {
		result.Warning = "localhost wurde für den Test automatisch zu host.containers.internal übersetzt (Container-Netzwerk)."
	}

	// SSRF protection: validate the (possibly translated) api_base host.
	// host.containers.internal is explicitly allowed as it is the canonical way
	// to reach llama-server running on the host from inside a container.
	if parsed, err := url.Parse(testAPIBase); err != nil || !validLLMAPIBase(parsed) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid api_base URL", r.URL.Path)
		return
	} else if llmBackendHostBlocked(parsed.Hostname()) {
		result.Success = false
		result.Message = "api_base verweist auf eine interne Adresse (SSRF-Schutz)"
		result.LatencyMs = time.Since(start).Milliseconds()
		httputil.WriteJSON(w, http.StatusOK, result)
		return
	}

	switch req.BackendType {
	case "ollama":
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(testAPIBase + "/api/tags")
		if err != nil {
			result.Message = "Verbindung fehlgeschlagen: " + err.Error()
		} else {
			resp.Body.Close()
			result.Success = resp.StatusCode == http.StatusOK
			result.Message = fmt.Sprintf("Ollama erreichbar (HTTP %d)", resp.StatusCode)
		}
	case "llama_server", "openai", "anthropic":
		// OpenAI-compatible test: GET /v1/models
		client := &http.Client{Timeout: 10 * time.Second}
		testReq, err := http.NewRequest("GET", testAPIBase+"/models", nil)
		if err != nil {
			result.Message = "Ungueltige URL: " + err.Error()
			break
		}
		if req.APIKey != nil {
			testReq.Header.Set("Authorization", "Bearer "+*req.APIKey)
		}
		resp, err := client.Do(testReq)
		if err != nil {
			result.Message = "Verbindung fehlgeschlagen: " + err.Error()
		} else {
			resp.Body.Close()
			result.Success = resp.StatusCode < 400
			result.Message = fmt.Sprintf("API erreichbar (HTTP %d)", resp.StatusCode)
		}
	default:
		result.Message = "Unbekannter Backend-Typ"
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	httputil.WriteJSON(w, http.StatusOK, result)
}

// PatchLLMUserConfig saves the model override for a user.
func (h *Handler) PatchLLMUserConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Role      string  `json:"role"`
		ModelName *string `json:"model_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	if !model.IsValidLLMRole(req.Role) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}
	if req.ModelName != nil && strings.TrimSpace(*req.ModelName) != "" {
		configs, err := h.DB.GetLLMSystemConfig(r.Context())
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get LLM config", r.URL.Path)
			return
		}
		var system *model.LLMSystemConfig
		for i := range configs {
			if configs[i].Role == req.Role {
				system = &configs[i]
				break
			}
		}
		if system == nil || !system.EvalGatePassed() || !system.AllowsModelOverride(*req.ModelName) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "model override has not passed eval gates", r.URL.Path)
			return
		}
	}
	if err := h.DB.UpsertLLMUserConfig(r.Context(), userID, req.Role, req.ModelName); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save user LLM config", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetLLMUserConfig returns user overrides.
func (h *Handler) GetLLMUserConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	configs, err := h.DB.GetLLMUserConfig(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get user LLM config", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, configs)
}

type liteLLMModelEntry struct {
	ModelName     string            `yaml:"model_name"`
	LiteLLMParams map[string]string `yaml:"litellm_params"`
}

type liteLLMConfigFile struct {
	ModelList       []liteLLMModelEntry `yaml:"model_list"`
	LiteLLMSettings map[string]any      `yaml:"litellm_settings"`
}

func (h *Handler) regenerateLiteLLMConfig(ctx context.Context) error {
	if h.Config == nil {
		return nil
	}
	configPath := h.Config.LiteLLMConfigPath
	if configPath == "" {
		return nil
	}

	configs, err := h.DB.GetLLMSystemConfig(ctx)
	if err != nil {
		return fmt.Errorf("get LLM config: %w", err)
	}

	entries := make([]liteLLMModelEntry, 0, len(configs))
	for _, c := range configs {
		if !c.EvalGatePassed() || c.ModelName == nil || c.APIBase == nil {
			continue
		}
		// LiteLLM provider prefix
		var litellmModel string
		switch c.BackendType {
		case "ollama":
			litellmModel = "ollama/" + *c.ModelName
		case "llama_server", "openai":
			litellmModel = "openai/" + *c.ModelName
		case "anthropic":
			litellmModel = "anthropic/" + *c.ModelName
		default:
			continue
		}
		// LiteLLM runs inside a container — translate localhost to
		// host.containers.internal so it can reach host services.
		apiBase := strings.NewReplacer(
			"localhost", "host.containers.internal",
			"127.0.0.1", "host.containers.internal",
		).Replace(*c.APIBase)

		// For backends without a real API key (llama_server, ollama),
		// LiteLLM still requires api_key to be set — use "none" as dummy.
		apiKey := "none"
		if c.HasAPIKey {
			if enc, err := h.DB.GetLLMAPIKeyEnc(ctx, c.Role); err == nil && enc != "" {
				if dec, err := crypto.Decrypt(enc, h.Config.LLMEncryptionKey); err == nil {
					apiKey = dec
				}
			}
		}

		entries = append(entries, liteLLMModelEntry{
			ModelName: "skadia-" + c.Role,
			LiteLLMParams: map[string]string{
				"model":    litellmModel,
				"api_base": apiBase,
				"api_key":  apiKey,
			},
		})
	}

	raw, err := yaml.Marshal(liteLLMConfigFile{
		ModelList: entries,
		LiteLLMSettings: map[string]any{
			"drop_params":     true,
			"request_timeout": 120,
		},
	})
	if err != nil {
		return fmt.Errorf("render LiteLLM config: %w", err)
	}

	return os.WriteFile(configPath, raw, 0600)
}

func llmBackendHostBlocked(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || isAllowedLocalLLMBackendHost(host) {
		return false
	}
	return isInternalHost(host)
}

func isAllowedLocalLLMBackendHost(host string) bool {
	switch host {
	case "ollama", "litellm", "host.containers.internal":
		return true
	default:
		return false
	}
}

func validLLMAPIBase(parsed *url.URL) bool {
	if parsed == nil || parsed.Hostname() == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
