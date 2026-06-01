package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"gopkg.in/yaml.v3"
)

func TestPutLLMConfigRoleAcceptsExplicitRoleAndRejectsLegacyDefault(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"llama_server","api_base":"http://llm.example/v1","model_name":"tool-model","context_window":16000,"api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tool_agent status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	configs, err := h.DB.GetLLMSystemConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	var toolConfig *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == model.LLMRoleToolAgent {
			toolConfig = &configs[i]
			break
		}
	}
	if toolConfig == nil {
		t.Fatal("tool_agent config not found")
	}
	if toolConfig.EvalStatus != model.LLMEvalStatusWaived {
		t.Fatalf("eval status = %q, want admin release after saving active config", toolConfig.EvalStatus)
	}
	if reason, _ := toolConfig.EvalReport["admin_reason"].(string); reason == "" {
		t.Fatalf("eval report = %#v, want admin_reason after saving active config", toolConfig.EvalReport)
	}
	if toolConfig.ContextWindow == nil || *toolConfig.ContextWindow != 16000 {
		t.Fatalf("context_window = %#v, want 16000", toolConfig.ContextWindow)
	}
	if identity, _ := toolConfig.EvalReport["identity"].(map[string]any); identity["context_window"] != float64(16000) {
		t.Fatalf("eval identity = %#v, want context_window 16000", toolConfig.EvalReport["identity"])
	}

	req = llmRoleRequest(t, http.MethodPut, "/admin/llm-config/default", "default", body)
	rec = httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("default status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleRejectsInvalidContextWindow(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"llama_server","api_base":"http://llm.example/v1","model_name":"tool-model","context_window":512,"api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for invalid context_window", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRolePreservesContextWindowWhenOmitted(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, context_window, enabled)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'tool-model', 16000, true)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			context_window = EXCLUDED.context_window,
			enabled = EXCLUDED.enabled
	`); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	body := []byte(`{"backend_type":"llama_server","api_base":"http://llm.example/v1","model_name":"tool-model","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	var contextWindow int
	if err := pool.QueryRow(ctx, `SELECT context_window FROM llm_system_config WHERE role = 'tool_agent'`).Scan(&contextWindow); err != nil {
		t.Fatalf("query context_window: %v", err)
	}
	if contextWindow != 16000 {
		t.Fatalf("context_window = %d, want preserved 16000", contextWindow)
	}
}

func TestPutLLMConfigRoleRejectsUnsupportedBackendType(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"anthropic","api_base":"https://api.anthropic.com/v1","model_name":"claude","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for unsupported direct backend", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleNormalizesOllamaExecutionBaseBeforeStorage(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}
	if _, err := pool.Exec(context.Background(), `UPDATE llm_system_config SET api_key_enc = NULL WHERE role = 'tool_agent'`); err != nil {
		t.Fatalf("clear previous key state: %v", err)
	}

	body := []byte(`{"backend_type":"ollama","api_base":"http://ollama:11434","model_name":"qwen3:8b","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	var apiBase string
	if err := pool.QueryRow(context.Background(), `SELECT api_base FROM llm_system_config WHERE role = 'tool_agent'`).Scan(&apiBase); err != nil {
		t.Fatalf("query api_base: %v", err)
	}
	if apiBase != "http://ollama:11434/v1" {
		t.Fatalf("stored api_base = %q, want OpenAI-compatible Ollama execution base", apiBase)
	}
}

func TestPutLLMConfigRoleRejectsImplicitKeyCarryAcrossProviderBaseChange(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled)
		VALUES ('tool_agent', 'openai', 'https://api.openai.com/v1', 'old-model', 'encrypted-key', true)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			api_key_enc = EXCLUDED.api_key_enc,
			enabled = EXCLUDED.enabled
	`); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	body := []byte(`{"backend_type":"llama_server","api_base":"https://llm.example/v1","model_name":"new-model","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for implicit key carry", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleRejectsClearWithAPIKey(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"llama_server","api_base":"https://llm.example/v1","model_name":"tool-model","api_key_state":"clear","api_key":"new-key","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for clear plus api_key", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleRejectsDisabledRoleWithAPIKey(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"disabled","api_key_state":"set","api_key":"hidden-key","enabled":false}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for disabled role with api_key", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleRejectsRecursiveLiteLLMAPIBase(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"openai","api_base":"http://litellm:4000/v1","model_name":"tool-model","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for recursive LiteLLM base", rec.Code, rec.Body.String())
	}
}

func TestPutLLMConfigRoleSurfacesLiteLLMConfigWriteFailure(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{LiteLLMConfigPath: t.TempDir()}
	if _, err := pool.Exec(context.Background(), `UPDATE llm_system_config SET api_key_enc = NULL WHERE role = 'tool_agent'`); err != nil {
		t.Fatalf("clear previous key state: %v", err)
	}

	body := []byte(`{"backend_type":"llama_server","api_base":"https://llm.example/v1","model_name":"tool-model","api_key_state":"unchanged","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 because DB config is authoritative", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "warning") {
		t.Fatalf("body = %s, want warning for optional LiteLLM artifact failure", rec.Body.String())
	}
}

func TestPatchLLMUserConfigRejectsUnevaluatedOverride(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	ctx := context.Background()
	evalReport := map[string]any{
		"identity": map[string]any{
			"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "llm.example", "model_name": "system-model",
			"suite_id": model.LLMEvalSuiteID, "suite_hash": model.LLMEvalSuiteHash,
		},
		"model_evals": []map[string]string{},
	}
	raw, err := json.Marshal(evalReport)
	if err != nil {
		t.Fatalf("marshal eval report: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'system-model', true, 'passed', $1::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
					model_name = EXCLUDED.model_name,
					context_window = NULL,
					api_key_enc = NULL,
				enabled = EXCLUDED.enabled,
				eval_status = EXCLUDED.eval_status,
				eval_report = EXCLUDED.eval_report
	`, string(raw)); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	req := llmUserRequest(t, userID, []byte(`{"role":"tool_agent","model_name":"unevaluated-model"}`))
	rec := httptest.NewRecorder()
	h.PatchLLMUserConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestPatchLLMUserConfigAcceptsEvaluatedOverride(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	evalReport := map[string]any{
		"identity": map[string]any{
			"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "llm.example", "model_name": "system-model",
			"suite_id": model.LLMEvalSuiteID, "suite_hash": model.LLMEvalSuiteHash,
		},
		"model_evals": []map[string]string{{
			"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "llm.example", "model_name": "user-model", "status": "passed",
			"suite_id": model.LLMEvalSuiteID, "suite_hash": model.LLMEvalSuiteHash,
		}},
	}
	raw, err := json.Marshal(evalReport)
	if err != nil {
		t.Fatalf("marshal eval report: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'system-model', true, 'passed', $1::jsonb)
		ON CONFLICT (role) DO UPDATE SET
				backend_type = EXCLUDED.backend_type,
				api_base = EXCLUDED.api_base,
					model_name = EXCLUDED.model_name,
					context_window = NULL,
					api_key_enc = NULL,
				enabled = EXCLUDED.enabled,
				eval_status = EXCLUDED.eval_status,
				eval_report = EXCLUDED.eval_report
	`, string(raw)); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	req := llmUserRequest(t, userID, []byte(`{"role":"tool_agent","model_name":"user-model"}`))
	rec := httptest.NewRecorder()
	h.PatchLLMUserConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	configs, err := h.DB.GetLLMUserConfig(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetLLMUserConfig: %v", err)
	}
	if len(configs) != 1 || configs[0].ModelName == nil || *configs[0].ModelName != "user-model" {
		t.Fatalf("configs = %#v, want saved user-model override", configs)
	}
}

func TestGetLLMConfigReturnsEvalFieldsAndKeyPresence(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, enabled, capability_flags, eval_status, eval_report, api_key_enc)
		VALUES ('document_extract', 'disabled', false, ARRAY['document_extract'], 'waived', '{"admin_reason":"manual"}'::jsonb, 'encrypted')
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			enabled = EXCLUDED.enabled,
			capability_flags = EXCLUDED.capability_flags,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report,
			api_key_enc = EXCLUDED.api_key_enc
	`); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	rec := httptest.NewRecorder()
	h.GetLLMConfig(rec, httptest.NewRequest(http.MethodGet, "/admin/llm-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var configs []model.LLMSystemConfig
	if err := json.NewDecoder(rec.Body).Decode(&configs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == "document_extract" {
			found = &configs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("document_extract config not returned")
	}
	if found.EvalStatus != model.LLMEvalStatusWaived || len(found.CapabilityFlags) != 1 || found.CapabilityFlags[0] != "document_extract" || !found.HasAPIKey {
		t.Fatalf("document_extract = %#v, want eval/capability/key fields", found)
	}
}

func TestPatchLLMConfigEvalWaiverRequiresAdminReason(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	req := llmRoleRequest(t, http.MethodPatch, "/admin/llm-config/tool_agent/eval-waiver", "tool_agent", []byte(`{"admin_reason":"  "}`))
	rec := httptest.NewRecorder()
	h.PatchLLMConfigEvalWaiver(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestPatchLLMConfigEvalWaiverStoresReason(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	path := t.TempDir() + "/config.yaml"
	h.Config = &HandlerConfig{LiteLLMConfigPath: path}

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'candidate-model', true, 'failed', '{"failed_cases":["tool"]}'::jsonb)
			ON CONFLICT (role) DO UPDATE SET
				backend_type = EXCLUDED.backend_type,
				api_base = EXCLUDED.api_base,
				model_name = EXCLUDED.model_name,
				api_key_enc = NULL,
				enabled = EXCLUDED.enabled,
				eval_status = EXCLUDED.eval_status,
				eval_report = EXCLUDED.eval_report
	`); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	req := llmRoleRequest(t, http.MethodPatch, "/admin/llm-config/tool_agent/eval-waiver", "tool_agent", []byte(`{"admin_reason":"Temporary local validation while eval service is offline"}`))
	rec := httptest.NewRecorder()
	h.PatchLLMConfigEvalWaiver(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "litellm_pending_apply") {
		t.Fatalf("body = %s, want pending-apply marker after waiver config regeneration", rec.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("waiver did not regenerate LiteLLM config at %s: %v", path, err)
	}

	configs, err := h.DB.GetLLMSystemConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	var got *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == model.LLMRoleToolAgent {
			got = &configs[i]
			break
		}
	}
	if got == nil {
		t.Fatal("tool_agent config not found")
	}
	if got.EvalStatus != model.LLMEvalStatusWaived {
		t.Fatalf("eval status = %q, want waived", got.EvalStatus)
	}
	if reason, _ := got.EvalReport["admin_reason"].(string); reason == "" {
		t.Fatalf("eval report = %#v, want admin_reason", got.EvalReport)
	}
	identity, _ := got.EvalReport["identity"].(map[string]any)
	if identity["api_base_host"] != "llm.example" || identity["model_name"] != "candidate-model" {
		t.Fatalf("eval report identity = %#v, want current config identity", got.EvalReport["identity"])
	}
}

func TestPatchLLMConfigEvalWaiverDoesNotFailWhenOptionalLiteLLMConfigCannotBeWritten(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{LiteLLMConfigPath: t.TempDir()}

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'candidate-model', true, 'failed', '{"failed_cases":["tool"]}'::jsonb)
			ON CONFLICT (role) DO UPDATE SET
				backend_type = EXCLUDED.backend_type,
				api_base = EXCLUDED.api_base,
				model_name = EXCLUDED.model_name,
				api_key_enc = NULL,
				enabled = EXCLUDED.enabled,
				eval_status = EXCLUDED.eval_status,
				eval_report = EXCLUDED.eval_report
	`); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	req := llmRoleRequest(t, http.MethodPatch, "/admin/llm-config/tool_agent/eval-waiver", "tool_agent", []byte(`{"admin_reason":"Temporary local validation while eval service is offline"}`))
	rec := httptest.NewRecorder()
	h.PatchLLMConfigEvalWaiver(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 because LiteLLM config is optional", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "warning") || !strings.Contains(rec.Body.String(), "litellm_config_stale") {
		t.Fatalf("body = %s, want warning for optional LiteLLM artifact failure", rec.Body.String())
	}

	configs, err := h.DB.GetLLMSystemConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	var got *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == model.LLMRoleToolAgent {
			got = &configs[i]
			break
		}
	}
	if got == nil {
		t.Fatal("tool_agent config not found")
	}
	if got.EvalStatus != model.LLMEvalStatusWaived {
		t.Fatalf("eval status = %q, want waived despite optional LiteLLM artifact failure", got.EvalStatus)
	}
}

func TestLLMBackendHostBlockedAllowsLocalLLMServiceHosts(t *testing.T) {
	for _, host := range []string{"ollama", "litellm", "host.containers.internal"} {
		if llmBackendHostBlocked(host) {
			t.Fatalf("host %q was blocked, want allowed local LLM service host", host)
		}
	}
	if !llmBackendHostBlocked("127.0.0.1") {
		t.Fatal("127.0.0.1 was allowed without translation, want blocked")
	}
}

func TestLLMConnectionReturnsStructuredMessageCodes(t *testing.T) {
	h := &Handler{}
	body := []byte(`{"backend_type":"openai","api_base":"https://llm.invalid/v1","model_name":"tool-model"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/llm-config/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.TestLLMConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var got struct {
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		MessageCode string `json:"message_code"`
		Warning     string `json:"warning"`
		WarningCode string `json:"warning_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.MessageCode != "connection_failed" {
		t.Fatalf("message_code = %q, want connection_failed; body=%s", got.MessageCode, rec.Body.String())
	}
	if strings.Contains(got.Message, "Verbindung fehlgeschlagen") || strings.Contains(got.Warning, "localhost wurde") {
		t.Fatalf("response contains localized backend text: %#v", got)
	}
}

func TestLLMConnectionReturnsStructuredInvalidURLCode(t *testing.T) {
	h := &Handler{}
	body := []byte(`{"backend_type":"openai","api_base":"http://[::1","model_name":"tool-model"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/llm-config/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.TestLLMConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var got struct {
		Message     string `json:"message"`
		MessageCode string `json:"message_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.MessageCode != "invalid_url" {
		t.Fatalf("message_code = %q, want invalid_url; body=%s", got.MessageCode, rec.Body.String())
	}
	if strings.Contains(got.Message, "Ungueltige URL") || strings.Contains(got.Message, "Ungültige URL") {
		t.Fatalf("response contains localized backend text: %#v", got)
	}
}

func TestLLMConnectionReturnsStructuredLocalhostRewriteWarning(t *testing.T) {
	h := &Handler{}
	body := []byte(`{"backend_type":"llama_server","api_base":"http://localhost:8000","model_name":"tool-model"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/llm-config/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.TestLLMConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var got struct {
		Warning     string `json:"warning"`
		WarningCode string `json:"warning_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.WarningCode != "localhost_rewritten_for_container" {
		t.Fatalf("warning_code = %q, want localhost_rewritten_for_container; body=%s", got.WarningCode, rec.Body.String())
	}
	if strings.Contains(got.Warning, "localhost wurde") {
		t.Fatalf("response contains localized backend warning: %#v", got)
	}
}

func TestRegenerateLiteLLMConfigUsesYAMLEncoder(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	path := t.TempDir() + "/config.yaml"
	h.Config = &HandlerConfig{LiteLLMConfigPath: path}
	evalReport, err := json.Marshal(map[string]any{"identity": map[string]any{
		"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "llm.example", "model_name": "model:with:colon",
		"suite_id": model.LLMEvalSuiteID, "suite_hash": model.LLMEvalSuiteHash,
	}})
	if err != nil {
		t.Fatalf("marshal eval report: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
			INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
			VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'model:with:colon', true, 'passed', $1::jsonb)
			ON CONFLICT (role) DO UPDATE SET
				backend_type = EXCLUDED.backend_type,
				api_base = EXCLUDED.api_base,
				model_name = EXCLUDED.model_name,
				api_key_enc = NULL,
				enabled = EXCLUDED.enabled,
				eval_status = EXCLUDED.eval_status,
				eval_report = EXCLUDED.eval_report
	`, string(evalReport)); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	if err := h.regenerateLiteLLMConfig(context.Background()); err != nil {
		t.Fatalf("regenerateLiteLLMConfig: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("generated YAML did not parse: %v\n%s", err, raw)
	}
	if !bytes.Contains(raw, []byte("skadia-tool_agent")) || !bytes.Contains(raw, []byte("openai/model:with:colon")) {
		t.Fatalf("generated YAML missing model entry:\n%s", raw)
	}
}

func llmRoleRequest(t *testing.T, method, target, role string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("role", role)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func llmUserRequest(t *testing.T, userID string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings/llm", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	return req.WithContext(ctx)
}
