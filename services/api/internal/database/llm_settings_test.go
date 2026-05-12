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
