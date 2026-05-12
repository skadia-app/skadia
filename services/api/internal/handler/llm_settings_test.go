package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

func TestPutLLMConfigRoleAcceptsExplicitRoleAndRejectsLegacyDefault(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	h.Config = &HandlerConfig{}

	body := []byte(`{"backend_type":"llama_server","api_base":"http://llm.example/v1","model_name":"tool-model","enabled":true}`)
	req := llmRoleRequest(t, http.MethodPut, "/admin/llm-config/tool_agent", "tool_agent", body)
	rec := httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tool_agent status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	req = llmRoleRequest(t, http.MethodPut, "/admin/llm-config/default", "default", body)
	rec = httptest.NewRecorder()
	h.PutLLMConfigRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("default status = %d body=%s, want 400", rec.Code, rec.Body.String())
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
		},
		"model_evals": []map[string]string{{
			"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "llm.example", "model_name": "user-model", "status": "passed",
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
		VALUES ('write_preview', 'disabled', false, ARRAY['write_preview'], 'waived', '{"admin_reason":"manual"}'::jsonb, 'encrypted')
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
		if configs[i].Role == "write_preview" {
			found = &configs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("write_preview config not returned")
	}
	if found.EvalStatus != model.LLMEvalStatusWaived || len(found.CapabilityFlags) != 1 || found.CapabilityFlags[0] != "write_preview" || !found.HasAPIKey {
		t.Fatalf("write_preview = %#v, want eval/capability/key fields", found)
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

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'candidate-model', true, 'failed', '{"failed_cases":["tool"]}'::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
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

func TestRegenerateLiteLLMConfigUsesYAMLEncoder(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	path := t.TempDir() + "/config.yaml"
	h.Config = &HandlerConfig{LiteLLMConfigPath: path}
	evalReport := `{"identity":{"role":"tool_agent","backend_type":"llama_server","api_base_host":"llm.example","model_name":"model:with:colon"}}`
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'model:with:colon', true, 'passed', $1::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			enabled = EXCLUDED.enabled,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report
	`, evalReport); err != nil {
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
