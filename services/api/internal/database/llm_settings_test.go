package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestUpdateLLMEvalResultStoresStatusAndReport(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', 'http://llm.example/v1', 'candidate-model', true, 'untested', '{}'::jsonb)
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

	report := map[string]any{
		"status":       model.LLMEvalStatusFailed,
		"failed_cases": []string{"tool_single_call"},
	}
	if err := db.UpdateLLMEvalResult(ctx, model.LLMRoleToolAgent, model.LLMEvalStatusFailed, report); err != nil {
		t.Fatalf("UpdateLLMEvalResult: %v", err)
	}

	var status string
	var raw []byte
	if err := pool.QueryRow(ctx, `
		SELECT eval_status, eval_report
		FROM llm_system_config
		WHERE role = 'tool_agent'
	`).Scan(&status, &raw); err != nil {
		t.Fatalf("query eval result: %v", err)
	}
	if status != model.LLMEvalStatusFailed {
		t.Fatalf("status = %q, want failed", status)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if stored["status"] != model.LLMEvalStatusFailed {
		t.Fatalf("report = %#v, want failed status", stored)
	}
}

func TestUpdateLLMModelEvalResultPersistsAndReplacesOverrideRecord(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()
	systemReport, err := json.Marshal(map[string]any{"identity": map[string]any{
		"role":          model.LLMRoleToolAgent,
		"backend_type":  model.LLMBackendLlamaServer,
		"api_base_host": "llm.example",
		"model_name":    "system-model",
		"suite_id":      model.LLMEvalSuiteID,
		"suite_hash":    model.LLMEvalSuiteHash,
	}})
	if err != nil {
		t.Fatalf("marshal system report: %v", err)
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
	`, string(systemReport)); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	report := map[string]any{
		"status": model.LLMEvalStatusPassed,
		"identity": map[string]any{
			"role":          model.LLMRoleToolAgent,
			"backend_type":  model.LLMBackendLlamaServer,
			"api_base_host": "llm.example",
			"model_name":    "user-model",
			"suite_id":      model.LLMEvalSuiteID,
			"suite_hash":    model.LLMEvalSuiteHash,
		},
		"cases": []any{map[string]any{"id": "tool", "passed": true}},
	}
	if err := db.UpdateLLMModelEvalResult(ctx, model.LLMRoleToolAgent, report); err != nil {
		t.Fatalf("UpdateLLMModelEvalResult passed: %v", err)
	}
	report["status"] = model.LLMEvalStatusFailed
	if err := db.UpdateLLMModelEvalResult(ctx, model.LLMRoleToolAgent, report); err != nil {
		t.Fatalf("UpdateLLMModelEvalResult replacement: %v", err)
	}

	configs, err := db.GetLLMSystemConfig(ctx)
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	var cfg *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == model.LLMRoleToolAgent {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil {
		t.Fatal("tool_agent config missing")
	}
	records, ok := cfg.EvalReport["model_evals"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("model_evals = %#v, want one replacement record", cfg.EvalReport["model_evals"])
	}
	record := records[0].(map[string]any)
	if record["status"] != model.LLMEvalStatusFailed {
		t.Fatalf("model eval status = %#v, want failed replacement", record["status"])
	}
}

func TestGetLLMSystemConfigFiltersLegacyRoles(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, enabled)
		VALUES ('vlm', 'ollama', true), ('default', 'ollama', true)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			enabled = EXCLUDED.enabled
	`); err != nil {
		t.Fatalf("seed legacy roles: %v", err)
	}

	configs, err := db.GetLLMSystemConfig(ctx)
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	for _, cfg := range configs {
		if cfg.Role == "vlm" || cfg.Role == "default" {
			t.Fatalf("GetLLMSystemConfig returned stale legacy role %#v", cfg)
		}
	}
}

func TestUpsertLLMSystemConfigResetsEvalOnIdentityChange(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()
	apiBase := "http://llm.example/v1"
	modelName := "candidate-model"

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', $1, $2, true, 'passed', '{"identity":{"role":"tool_agent","backend_type":"llama_server","api_base_host":"llm.example","model_name":"candidate-model"}}'::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			enabled = EXCLUDED.enabled,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report
	`, apiBase, modelName); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	newModel := "different-model"
	if err := db.UpsertLLMSystemConfig(ctx, model.LLMRoleToolAgent, model.LLMSystemConfigUpdate{
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &newModel,
		Enabled:     true,
	}, nil); err != nil {
		t.Fatalf("UpsertLLMSystemConfig: %v", err)
	}

	var status string
	var raw []byte
	if err := pool.QueryRow(ctx, `
		SELECT eval_status, eval_report
		FROM llm_system_config
		WHERE role = 'tool_agent'
	`).Scan(&status, &raw); err != nil {
		t.Fatalf("query eval result: %v", err)
	}
	if status != model.LLMEvalStatusUntested {
		t.Fatalf("status = %q, want untested", status)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("eval report = %#v, want reset empty object", stored)
	}
}

func TestUpsertLLMSystemConfigStoresContextWindowAndResetsEval(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()
	apiBase := "http://llm.example/v1"
	modelName := "candidate-model"

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', $1, $2, true, 'passed', '{"identity":{"role":"tool_agent","backend_type":"llama_server","api_base_host":"llm.example","model_name":"candidate-model","suite_id":"legacy","suite_hash":"legacy"}}'::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			context_window = NULL,
			enabled = EXCLUDED.enabled,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report
	`, apiBase, modelName); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	contextWindow := 16000
	if err := db.UpsertLLMSystemConfig(ctx, model.LLMRoleToolAgent, model.LLMSystemConfigUpdate{
		BackendType:   "llama_server",
		APIBase:       &apiBase,
		ModelName:     &modelName,
		ContextWindow: &contextWindow,
		APIKeyState:   model.LLMAPIKeyStateUnchanged,
		Enabled:       true,
	}, nil); err != nil {
		t.Fatalf("UpsertLLMSystemConfig: %v", err)
	}

	configs, err := db.GetLLMSystemConfig(ctx)
	if err != nil {
		t.Fatalf("GetLLMSystemConfig: %v", err)
	}
	var cfg *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == model.LLMRoleToolAgent {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil || cfg.ContextWindow == nil || *cfg.ContextWindow != contextWindow {
		t.Fatalf("context_window = %#v, want %d", cfg, contextWindow)
	}
	if cfg.EvalStatus != model.LLMEvalStatusUntested {
		t.Fatalf("eval status = %q, want untested", cfg.EvalStatus)
	}
	if len(cfg.EvalReport) != 0 {
		t.Fatalf("eval report = %#v, want reset empty object", cfg.EvalReport)
	}
}

func TestUpsertLLMSystemConfigClearsHiddenKeyWhenDisabled(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()
	apiBase := "http://llm.example/v1"
	modelName := "candidate-model"

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled)
		VALUES ('tool_agent', 'llama_server', $1, $2, 'encrypted-key', true)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			api_key_enc = EXCLUDED.api_key_enc,
			enabled = EXCLUDED.enabled
	`, apiBase, modelName); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	if err := db.UpsertLLMSystemConfig(ctx, model.LLMRoleToolAgent, model.LLMSystemConfigUpdate{
		BackendType: model.LLMBackendDisabled,
		APIKeyState: model.LLMAPIKeyStateUnchanged,
		Enabled:     false,
	}, nil); err != nil {
		t.Fatalf("UpsertLLMSystemConfig disabled: %v", err)
	}

	var hasKey bool
	if err := pool.QueryRow(ctx, `
		SELECT api_key_enc IS NOT NULL
		FROM llm_system_config
		WHERE role = 'tool_agent'
	`).Scan(&hasKey); err != nil {
		t.Fatalf("query key state: %v", err)
	}
	if hasKey {
		t.Fatal("api_key_enc is still set after disabling role")
	}
}

func TestUpsertLLMSystemConfigClearsAPIKeyExplicitly(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()
	apiBase := "http://llm.example/v1"
	modelName := "candidate-model"

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled, eval_status, eval_report)
		VALUES ('tool_agent', 'llama_server', $1, $2, 'encrypted-key', true, 'passed', '{"identity":{"role":"tool_agent","backend_type":"llama_server","api_base_host":"llm.example","model_name":"candidate-model","suite_id":"legacy","suite_hash":"legacy"}}'::jsonb)
		ON CONFLICT (role) DO UPDATE SET
			backend_type = EXCLUDED.backend_type,
			api_base = EXCLUDED.api_base,
			model_name = EXCLUDED.model_name,
			api_key_enc = EXCLUDED.api_key_enc,
			enabled = EXCLUDED.enabled,
			eval_status = EXCLUDED.eval_status,
			eval_report = EXCLUDED.eval_report
	`, apiBase, modelName); err != nil {
		t.Fatalf("seed system config: %v", err)
	}

	if err := db.UpsertLLMSystemConfig(ctx, model.LLMRoleToolAgent, model.LLMSystemConfigUpdate{
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		APIKeyState: model.LLMAPIKeyStateClear,
		Enabled:     true,
	}, nil); err != nil {
		t.Fatalf("UpsertLLMSystemConfig clear key: %v", err)
	}

	var hasKey bool
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT api_key_enc IS NOT NULL, eval_status
		FROM llm_system_config
		WHERE role = 'tool_agent'
	`).Scan(&hasKey, &status); err != nil {
		t.Fatalf("query key state: %v", err)
	}
	if hasKey {
		t.Fatal("api_key_enc is still set after explicit clear")
	}
	if status != model.LLMEvalStatusUntested {
		t.Fatalf("eval status = %q, want reset to untested after key clear", status)
	}
}
