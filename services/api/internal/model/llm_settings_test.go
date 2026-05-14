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
				"suite_id":      LLMEvalSuiteID,
				"suite_hash":    LLMEvalSuiteHash,
			},
			"model_evals": []any{
				map[string]any{
					"role":          LLMRoleToolAgent,
					"backend_type":  "llama_server",
					"api_base_host": "llm-b.example.test",
					"model_name":    "user-model",
					"status":        LLMEvalStatusPassed,
					"suite_id":      LLMEvalSuiteID,
					"suite_hash":    LLMEvalSuiteHash,
				},
			}},
	}

	if cfg.AllowsModelOverride("user-model") {
		t.Fatal("AllowsModelOverride accepted eval record from a different api_base_host")
	}
}

func TestAllowsModelOverrideAcceptsPersistedEvalReportRecord(t *testing.T) {
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
				"suite_id":      LLMEvalSuiteID,
				"suite_hash":    LLMEvalSuiteHash,
			},
			"model_evals": []any{
				map[string]any{
					"status": LLMEvalStatusPassed,
					"identity": map[string]any{
						"role":          LLMRoleToolAgent,
						"backend_type":  "llama_server",
						"api_base_host": "llm-a.example.test",
						"model_name":    "user-model",
						"suite_id":      LLMEvalSuiteID,
						"suite_hash":    LLMEvalSuiteHash,
					},
				},
			}},
	}

	if !cfg.AllowsModelOverride("user-model") {
		t.Fatal("AllowsModelOverride rejected persisted eval report model_evals record")
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
			"suite_id":      LLMEvalSuiteID,
			"suite_hash":    LLMEvalSuiteHash,
		}},
	}

	if cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed accepted stale eval identity")
	}
}

func TestEvalGatePassedRejectsMissingSuiteIdentity(t *testing.T) {
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
			"api_base_host": "llm-a.example.test",
			"model_name":    systemModel,
		}},
	}

	if cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed accepted a report without the current suite identity")
	}
}

func TestEvalGatePassedIncludesConfiguredContextWindowInIdentity(t *testing.T) {
	apiBase := "https://llm-a.example.test/v1"
	systemModel := "system-model"
	contextWindow := 16000
	cfg := LLMSystemConfig{
		Role:          LLMRoleToolAgent,
		BackendType:   "llama_server",
		APIBase:       &apiBase,
		ModelName:     &systemModel,
		ContextWindow: &contextWindow,
		Enabled:       true,
		EvalStatus:    LLMEvalStatusPassed,
		EvalReport: map[string]any{"identity": map[string]any{
			"role":           LLMRoleToolAgent,
			"backend_type":   "llama_server",
			"api_base_host":  "llm-a.example.test",
			"model_name":     systemModel,
			"context_window": float64(8000),
			"suite_id":       LLMEvalSuiteID,
			"suite_hash":     LLMEvalSuiteHash,
		}},
	}

	if cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed accepted stale context_window identity")
	}

	cfg.EvalReport["identity"].(map[string]any)["context_window"] = float64(contextWindow)
	if !cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed rejected matching context_window identity")
	}
}

func TestEvalGatePassedKeepsLegacyReportsCompatibleWhenContextWindowUnset(t *testing.T) {
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
			"api_base_host": "llm-a.example.test",
			"model_name":    systemModel,
			"suite_id":      LLMEvalSuiteID,
			"suite_hash":    LLMEvalSuiteHash,
		}},
	}

	if !cfg.EvalGatePassed() {
		t.Fatal("EvalGatePassed rejected legacy identity without context_window")
	}
}

func TestEvalGatePassedWithFallbackKeepsLegacyReportsCompatibleWhenContextWindowUnset(t *testing.T) {
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
			"api_base_host": "llm-a.example.test",
			"model_name":    systemModel,
			"suite_id":      LLMEvalSuiteID,
			"suite_hash":    LLMEvalSuiteHash,
		}},
	}

	if !cfg.EvalGatePassedWithFallback(16000) {
		t.Fatal("EvalGatePassedWithFallback rejected legacy identity without explicit role context_window")
	}
}

func TestEvalGatePassedWithFallbackRequiresContextWindowIdentityWhenRoleContextSet(t *testing.T) {
	apiBase := "https://llm-a.example.test/v1"
	systemModel := "system-model"
	contextWindow := 16000
	cfg := LLMSystemConfig{
		Role:          LLMRoleToolAgent,
		BackendType:   "llama_server",
		APIBase:       &apiBase,
		ModelName:     &systemModel,
		ContextWindow: &contextWindow,
		Enabled:       true,
		EvalStatus:    LLMEvalStatusPassed,
		EvalReport: map[string]any{"identity": map[string]any{
			"role":          LLMRoleToolAgent,
			"backend_type":  "llama_server",
			"api_base_host": "llm-a.example.test",
			"model_name":    systemModel,
			"suite_id":      LLMEvalSuiteID,
			"suite_hash":    LLMEvalSuiteHash,
		}},
	}

	if cfg.EvalGatePassedWithFallback(16000) {
		t.Fatal("EvalGatePassedWithFallback accepted legacy identity with explicit role context_window")
	}

	cfg.EvalReport["identity"].(map[string]any)["context_window"] = float64(16000)
	if !cfg.EvalGatePassedWithFallback(16000) {
		t.Fatal("EvalGatePassedWithFallback rejected matching fallback context_window identity")
	}
}

func TestNormalizeLLMAPIBaseNormalizesOpenAICompatibleExecutionBase(t *testing.T) {
	got, err := NormalizeLLMAPIBase(LLMBackendOllama, "http://localhost:11434")
	if err != nil {
		t.Fatalf("NormalizeLLMAPIBase: %v", err)
	}
	if got != "http://host.containers.internal:11434/v1" {
		t.Fatalf("normalized Ollama base = %q, want host.containers.internal /v1 base", got)
	}

	got, err = NormalizeLLMAPIBase(LLMBackendLlamaServer, "http://notlocalhost.example:8080/v1")
	if err != nil {
		t.Fatalf("NormalizeLLMAPIBase non-localhost: %v", err)
	}
	if got != "http://notlocalhost.example:8080/v1" {
		t.Fatalf("normalized non-localhost base = %q, want unchanged hostname", got)
	}
}

func TestNormalizeLLMAPIBaseRejectsLocalhostOnUnexpectedPort(t *testing.T) {
	if _, err := NormalizeLLMAPIBase(LLMBackendOllama, "http://localhost:5432/v1"); err == nil {
		t.Fatal("NormalizeLLMAPIBase accepted localhost on a non-LLM port")
	}
}

func TestNormalizeLLMAPIBaseRejectsUnsupportedDirectBackend(t *testing.T) {
	if _, err := NormalizeLLMAPIBase("anthropic", "https://api.anthropic.com/v1"); err == nil {
		t.Fatal("NormalizeLLMAPIBase accepted anthropic without a direct OpenAI-compatible adapter")
	}
}

func TestNormalizeLLMAPIBaseRejectsRecursiveLiteLLMGateway(t *testing.T) {
	for _, raw := range []string{
		"http://litellm:4000/v1",
		"http://host.containers.internal:4000/v1",
	} {
		if _, err := NormalizeLLMAPIBase(LLMBackendOpenAI, raw); err == nil {
			t.Fatalf("NormalizeLLMAPIBase accepted recursive gateway base %q", raw)
		}
	}
}
