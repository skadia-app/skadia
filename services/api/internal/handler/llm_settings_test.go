package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
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
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'system-model', true, 'passed', '{"model_evals":[]}'::jsonb)
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

	evalReport := map[string]any{"model_evals": []map[string]string{{
		"role": "tool_agent", "backend_type": "llama_server", "model_name": "user-model", "status": "passed",
	}}}
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
