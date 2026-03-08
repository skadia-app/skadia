package handler

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetLLMConfig returns the full LLM system configuration.
func (h *Handler) GetLLMConfig(w http.ResponseWriter, r *http.Request) {
	configs, err := h.DB.GetLLMSystemConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get LLM config", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

// PutLLMConfigRole saves the configuration for a model role.
func (h *Handler) PutLLMConfigRole(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	validRoles := map[string]bool{"default": true, "vlm": true, "vlm_fallback": true, "analysis": true}
	if !validRoles[role] {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid role", r.URL.Path)
		return
	}

	var update model.LLMSystemConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	var encKey *string
	if update.APIKey != nil && *update.APIKey != "" {
		enc, err := encryptAPIKey(*update.APIKey, h.Config.LLMEncryptionKey)
		if err != nil {
			slog.Error("failed to encrypt API key", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to encrypt API key", r.URL.Path)
			return
		}
		encKey = &enc
	}

	if err := h.DB.UpsertLLMSystemConfig(r.Context(), role, update, encKey); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save LLM config", r.URL.Path)
		return
	}

	// Regenerate LiteLLM config.yaml
	if err := h.regenerateLiteLLMConfig(r.Context()); err != nil {
		slog.Warn("failed to regenerate LiteLLM config", "error", err)
		// Don't return error - config was saved to DB
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetLLMModels returns available models for a backend (only Ollama supports auto-discovery).
func (h *Handler) GetLLMModels(w http.ResponseWriter, r *http.Request) {
	backend := r.URL.Query().Get("backend")
	apiBase := r.URL.Query().Get("api_base")
	if apiBase == "" {
		apiBase = "http://ollama:11434"
	}

	if backend != "ollama" {
		writeJSON(w, http.StatusOK, []model.OllamaModel{})
		return
	}

	// Query Ollama /api/tags
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiBase + "/api/tags")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Bad Gateway", "failed to reach Ollama: "+err.Error(), r.URL.Path)
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
		writeError(w, http.StatusBadGateway, "Bad Gateway", "failed to parse Ollama response", r.URL.Path)
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
	writeJSON(w, http.StatusOK, models)
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
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
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
	writeJSON(w, http.StatusOK, result)
}

// PatchLLMUserConfig saves the model override for a user.
func (h *Handler) PatchLLMUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req struct {
		Role      string  `json:"role"`
		ModelName *string `json:"model_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	if err := h.DB.UpsertLLMUserConfig(r.Context(), userID, req.Role, req.ModelName); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to save user LLM config", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetLLMUserConfig returns user overrides.
func (h *Handler) GetLLMUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	configs, err := h.DB.GetLLMUserConfig(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get user LLM config", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

// --- Helper functions ---

func encryptAPIKey(plaintext, hexKey string) (string, error) {
	if hexKey == "" {
		// No encryption key configured: store base64-encoded (not secure, but functional)
		return base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("LLM_ENCRYPTION_KEY must be 32-byte hex string (64 chars)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// liteLLMConfigTemplate is the Go template for LiteLLM config.yaml
const liteLLMConfigTemplate = `model_list:
{{- range .Models}}
{{- if .Enabled}}
  - model_name: skadia-{{.Role}}
    litellm_params:
      model: {{.LiteLLMModel}}
      api_base: {{.APIBase}}
{{- if .APIKey}}
      api_key: "{{.APIKey}}"
{{- end}}
{{- end}}
{{- end}}

litellm_settings:
  drop_params: true
  request_timeout: 120
`

type liteLLMModelEntry struct {
	Role         string
	LiteLLMModel string
	APIBase      string
	APIKey       string
	Enabled      bool
}

func (h *Handler) regenerateLiteLLMConfig(ctx context.Context) error {
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
		if !c.Enabled || c.ModelName == nil || c.APIBase == nil {
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
		entries = append(entries, liteLLMModelEntry{
			Role:         c.Role,
			LiteLLMModel: litellmModel,
			APIBase:      *c.APIBase,
			Enabled:      true,
		})
	}

	tmpl, err := template.New("litellm").Parse(liteLLMConfigTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, map[string]any{"Models": entries}); err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	return os.WriteFile(configPath, []byte(buf.String()), 0644)
}
