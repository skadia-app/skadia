package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	agenteval "github.com/skadia-app/skadia/services/api/internal/agent/eval"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
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
	if err := h.prepareLLMSystemConfigUpdate(r.Context(), role, &update); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	if update.APIKeyState == model.LLMAPIKeyStateSet {
		encryptionKey := ""
		if h.Config != nil {
			encryptionKey = h.Config.LLMEncryptionKey
		}
		enc, err := crypto.Encrypt(strings.TrimSpace(*update.APIKey), encryptionKey)
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
	if update.Enabled {
		if err := h.markLLMRoleAdminConfigured(r.Context(), role, "Admin configured direct LLM backend"); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to activate LLM config", r.URL.Path)
			return
		}
	}

	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config", "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":                "saved",
			"warning":               "optional LiteLLM config could not be regenerated: " + err.Error(),
			"litellm_config_stale":  true,
			"litellm_pending_apply": false,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":                "saved",
		"litellm_pending_apply": h.liteLLMConfigWritesNeedApply(),
	})
}

func (h *Handler) markLLMRoleAdminConfigured(ctx context.Context, role, reason string) error {
	configs, err := h.DB.GetLLMSystemConfig(ctx)
	if err != nil {
		return fmt.Errorf("get LLM config after save: %w", err)
	}
	var current *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == role {
			current = &configs[i]
			break
		}
	}
	if current == nil || !current.Enabled {
		return nil
	}
	identity, ok := current.CurrentEvalIdentity()
	if !ok {
		return nil
	}
	report := map[string]any{
		"status":       model.LLMEvalStatusWaived,
		"identity":     identity,
		"admin_reason": reason,
		"waived_at":    time.Now().UTC().Format(time.RFC3339),
	}
	return h.DB.UpdateLLMEvalResult(ctx, role, model.LLMEvalStatusWaived, report)
}

func (h *Handler) prepareLLMSystemConfigUpdate(ctx context.Context, role string, update *model.LLMSystemConfigUpdate) error {
	update.BackendType = strings.TrimSpace(update.BackendType)
	if !model.IsValidLLMBackendType(update.BackendType) {
		return fmt.Errorf("invalid backend_type")
	}
	if update.APIKeyState == "" {
		return fmt.Errorf("api_key_state is required")
	}
	if !model.IsValidLLMAPIKeyState(update.APIKeyState) {
		return fmt.Errorf("invalid api_key_state")
	}
	if update.APIKeyState == model.LLMAPIKeyStateSet {
		if update.APIKey == nil || strings.TrimSpace(*update.APIKey) == "" {
			return fmt.Errorf("api_key is required when api_key_state is set")
		}
	}
	if update.APIKeyState == model.LLMAPIKeyStateClear && update.APIKey != nil && strings.TrimSpace(*update.APIKey) != "" {
		return fmt.Errorf("api_key must be omitted when api_key_state is clear")
	}
	if update.APIKeyState == model.LLMAPIKeyStateUnchanged && update.APIKey != nil && strings.TrimSpace(*update.APIKey) != "" {
		return fmt.Errorf("api_key_state must be set when api_key is provided")
	}
	if update.Enabled && update.BackendType == model.LLMBackendDisabled {
		return fmt.Errorf("enabled LLM role requires a direct backend_type")
	}
	if update.BackendType == model.LLMBackendDisabled && update.APIKeyState == model.LLMAPIKeyStateSet {
		return fmt.Errorf("disabled LLM role must not store an api_key")
	}

	if update.BackendType == model.LLMBackendDisabled {
		update.APIBase = nil
		update.ModelName = nil
		update.APIKey = nil
		update.APIKeyState = model.LLMAPIKeyStateClear
	} else {
		if update.APIBase == nil {
			return fmt.Errorf("api_base is required")
		}
		normalized, err := model.NormalizeLLMAPIBase(update.BackendType, *update.APIBase)
		if err != nil {
			return err
		}
		update.APIBase = &normalized
		if update.ModelName != nil {
			modelName := strings.TrimSpace(*update.ModelName)
			update.ModelName = &modelName
		}
		if update.Enabled && (update.ModelName == nil || *update.ModelName == "") {
			return fmt.Errorf("model_name is required for enabled LLM roles")
		}
	}

	if update.APIKeyState == model.LLMAPIKeyStateUnchanged {
		carries, err := updateCarriesOldKeyAcrossProviderBase(ctx, h, role, *update)
		if err != nil {
			return err
		}
		if carries {
			return fmt.Errorf("api_key_state must be set or clear when backend_type or api_base changes from a role with a stored key")
		}
	}

	return nil
}

func updateCarriesOldKeyAcrossProviderBase(ctx context.Context, h *Handler, role string, update model.LLMSystemConfigUpdate) (bool, error) {
	configs, err := h.DB.GetLLMSystemConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get current LLM config before key-state validation: %w", err)
	}
	for _, current := range configs {
		if current.Role != role || !current.HasAPIKey {
			continue
		}
		return current.BackendType != update.BackendType || normalizedAPIBaseString(current.BackendType, current.APIBase) != normalizedAPIBaseString(update.BackendType, update.APIBase), nil
	}
	return false, nil
}

func normalizedAPIBaseString(backendType string, apiBase *string) string {
	if apiBase == nil {
		return ""
	}
	normalized, err := model.NormalizeLLMAPIBase(backendType, *apiBase)
	if err != nil {
		return strings.TrimSpace(*apiBase)
	}
	return normalized
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
	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config after eval waiver", "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":                "waived",
			"warning":               "optional LiteLLM config could not be regenerated: " + err.Error(),
			"litellm_config_stale":  true,
			"litellm_pending_apply": false,
		})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":                "waived",
		"litellm_pending_apply": h.liteLLMConfigWritesNeedApply(),
	})
}

func (h *Handler) PostLLMConfigEval(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if !model.IsValidLLMRole(role) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}
	var req struct {
		ModelName string `json:"model_name"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
			return
		}
	}
	modelNameOverride := strings.TrimSpace(r.URL.Query().Get("model_name"))
	if strings.TrimSpace(req.ModelName) != "" {
		modelNameOverride = strings.TrimSpace(req.ModelName)
	}
	key := ""
	if h.Config != nil {
		key = h.Config.LLMEncryptionKey
	}
	runner := agenteval.NewRunner(h.DB, agenteval.WireClient{HTTPClient: llmwire.NewSSRFProtectedHTTPClient(60*time.Second, nil)}, agenteval.RunnerOptions{
		EncryptionKey:     key,
		ModelNameOverride: modelNameOverride,
	})
	report, err := runner.Run(r.Context(), role)
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "Bad Gateway", err.Error(), r.URL.Path)
		return
	}
	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config after eval", "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":                report.Status,
			"identity":              report.Identity,
			"suite_id":              report.SuiteID,
			"suite_hash":            report.SuiteHash,
			"suite_case_count":      report.SuiteCaseCount,
			"completed":             report.Completed,
			"started_at":            report.StartedAt,
			"completed_at":          report.CompletedAt,
			"cases":                 report.Cases,
			"failed_cases":          report.FailedCases,
			"warning":               "optional LiteLLM config could not be regenerated: " + err.Error(),
			"litellm_config_stale":  true,
			"litellm_pending_apply": false,
		})
		return
	}
	report.LiteLLMPendingApply = h.liteLLMConfigWritesNeedApply()
	httputil.WriteJSON(w, http.StatusOK, report)
}

// GetLLMModels returns available models for a backend (only Ollama supports auto-discovery).
func (h *Handler) GetLLMModels(w http.ResponseWriter, r *http.Request) {
	backend := r.URL.Query().Get("backend")
	apiBase := r.URL.Query().Get("api_base")
	if apiBase == "" {
		apiBase = "http://ollama:11434"
	}

	if backend != model.LLMBackendOllama {
		httputil.WriteJSON(w, http.StatusOK, []model.OllamaModel{})
		return
	}

	normalized, err := model.NormalizeLLMAPIBase(model.LLMBackendOllama, apiBase)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	discoveryBase := strings.TrimSuffix(normalized, "/v1")

	// Query Ollama /api/tags
	client := llmwire.NewSSRFProtectedHTTPClient(10*time.Second, nil)
	resp, err := client.Get(discoveryBase + "/api/tags")
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

	if !model.IsValidLLMBackendType(req.BackendType) || !model.IsDirectLLMBackendType(req.BackendType) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported direct backend_type", r.URL.Path)
		return
	}

	testAPIBase, err := model.NormalizeLLMAPIBase(req.BackendType, req.APIBase)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if llmAPIBaseHostWasRewrittenForContainer(req.APIBase, testAPIBase) {
		result.Warning = "localhost wurde für den Test automatisch zu host.containers.internal übersetzt (Container-Netzwerk)."
	}

	switch req.BackendType {
	case model.LLMBackendOllama, model.LLMBackendLlamaServer, model.LLMBackendOpenAI:
		client := llmwire.NewSSRFProtectedHTTPClient(10*time.Second, nil)
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
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "model override is not released for this role", r.URL.Path)
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
		case model.LLMBackendOllama:
			litellmModel = "ollama/" + *c.ModelName
		case model.LLMBackendLlamaServer, model.LLMBackendOpenAI:
			litellmModel = "openai/" + *c.ModelName
		default:
			return fmt.Errorf("unsupported LiteLLM backend_type %q for role %s", c.BackendType, c.Role)
		}
		apiBase, err := model.NormalizeLLMAPIBase(c.BackendType, *c.APIBase)
		if err != nil {
			return fmt.Errorf("normalize LiteLLM api_base for role %s: %w", c.Role, err)
		}

		// For backends without a real API key (llama_server, ollama),
		// LiteLLM still requires api_key to be set — use "none" as dummy.
		apiKey := "none"
		if c.HasAPIKey {
			enc, err := h.DB.GetLLMAPIKeyEnc(ctx, c.Role)
			if err != nil {
				return fmt.Errorf("get LiteLLM api key for role %s: %w", c.Role, err)
			}
			if enc != "" {
				dec, err := crypto.Decrypt(enc, h.Config.LLMEncryptionKey)
				if err != nil {
					return fmt.Errorf("decrypt LiteLLM api key for role %s: %w", c.Role, err)
				}
				apiKey = dec
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
	var decoded liteLLMConfigFile
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("validate LiteLLM config: %w", err)
	}

	return writeFileAtomic(configPath, raw, 0600)
}

func llmBackendHostBlocked(host string) bool {
	return model.LLMBackendHostBlocked(host)
}

func llmAPIBaseHostWasRewrittenForContainer(raw, normalized string) bool {
	before, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	after, err := url.Parse(strings.TrimSpace(normalized))
	if err != nil {
		return false
	}
	switch strings.ToLower(before.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return strings.EqualFold(after.Hostname(), "host.containers.internal")
	default:
		return false
	}
}

func writeFileAtomic(path string, raw []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func (h *Handler) liteLLMConfigWritesNeedApply() bool {
	return h != nil && h.Config != nil && strings.TrimSpace(h.Config.LiteLLMConfigPath) != ""
}
