package model

import "testing"

func TestAllowsModelOverrideRequiresMatchingAPIBaseHost(t *testing.T) {
	apiBase := "https://llm-a.example.test/v1"
	systemModel := "system-model"
	cfg := LLMSystemConfig{
		Role:        LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &systemModel,
		Enabled:     true,
		EvalStatus:  LLMEvalStatusPassed,
		EvalReport: map[string]any{
			"identity": map[string]any{
				"role":          LLMRoleToolAgent,
				"backend_type":  "llama_server",
				"api_base_host": "llm-a.example.test",
				"model_name":    systemModel,
			},
			"model_evals": []any{
				map[string]any{
					"role":          LLMRoleToolAgent,
					"backend_type":  "llama_server",
					"api_base_host": "llm-b.example.test",
					"model_name":    "user-model",
					"status":        LLMEvalStatusPassed,
				},
			}},
	}

	if cfg.AllowsModelOverride("user-model") {
		t.Fatal("AllowsModelOverride accepted eval record from a different api_base_host")
	}
}

func TestEvalGatePassedRejectsStaleIdentity(t *testing.T) {
	apiBase := "https://llm-a.example.test/v1"
	systemModel := "system-model"
	cfg := LLMSystemConfig{
		Role:        LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &systemModel,
		Enabled:     true,
		EvalStatus:  LLMEvalStatusPassed,
		EvalReport: map[string]any{"identity": map[string]any{
			"role":          LLMRoleToolAgent,
			"backend_type":  "llama_server",
			"api_base_host": "old.example.test",
			"model_name":    systemModel,
		}},
	}

	if cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed accepted stale eval identity")
	}
}
