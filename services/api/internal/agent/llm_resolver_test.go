package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

var testLLMKey = strings.Repeat("0123456789abcdef", 4)

func TestLLMResolverErrorsWhenRoleHasNoUsableSystemConfig(t *testing.T) {
	resolver := NewLLMResolver(fakeLLMStore{}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
	})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want missing role configuration error")
	}
}

func TestLLMResolverUsesPassedSystemRoleConfig(t *testing.T) {
	enc, err := crypto.Encrypt("system-key", testLLMKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			HasAPIKey:   true,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", modelName),
		}},
		keys: map[string]string{"tool_agent": enc},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, APIKey: "system-key", Model: modelName, BackendType: "llama_server"})
}

func TestLLMResolverAllowsLocalBackendWithoutAPIKey(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", modelName),
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: modelName, BackendType: "llama_server"})
}

func TestLLMResolverDoesNotInheritEnvAPIKeyForDBRole(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", modelName),
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.APIKey != "" {
		t.Fatalf("APIKey = %q, want no inherited env key", got.APIKey)
	}
}

func TestLLMResolverUsesRoleContextWindowBeforeFallback(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	contextWindow := 16000
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:          "tool_agent",
			BackendType:   "llama_server",
			APIBase:       &apiBase,
			ModelName:     &modelName,
			ContextWindow: &contextWindow,
			Enabled:       true,
			EvalStatus:    model.LLMEvalStatusPassed,
			EvalReport:    evalIdentityReportWithContext("tool_agent", "llama_server", "system.example", modelName, contextWindow),
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
		ContextWindow: 8000,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: modelName, BackendType: "llama_server", ContextWindow: contextWindow})
}

func TestLLMResolverUsesFallbackContextWindowWhenRoleUnset(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReportWithContext("tool_agent", "llama_server", "system.example", modelName, 8000),
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
		ContextWindow: 8000,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: modelName, BackendType: "llama_server", ContextWindow: 8000})
}

func TestLLMResolverRejectsStaleContextWindowIdentity(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	contextWindow := 16000
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:          "tool_agent",
			BackendType:   "llama_server",
			APIBase:       &apiBase,
			ModelName:     &modelName,
			ContextWindow: &contextWindow,
			Enabled:       true,
			EvalStatus:    model.LLMEvalStatusPassed,
			EvalReport:    evalIdentityReportWithContext("tool_agent", "llama_server", "system.example", modelName, 8000),
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
		ContextWindow: 8000,
	})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want stale context window eval identity error")
	}
}

func TestLLMResolverAcceptsLegacyFallbackContextWindowIdentityWhenRoleContextUnset(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", modelName),
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
		ContextWindow: 16000,
	})

	if got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err != nil {
		t.Fatalf("Resolve returned error for legacy eval identity with unset role context: %v", err)
	} else if got.ContextWindow != 16000 {
		t.Fatalf("ContextWindow = %d, want fallback 16000", got.ContextWindow)
	}
}

func TestLLMResolverFailsClosedWhenMarkedAPIKeyCannotBeLoaded(t *testing.T) {
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			HasAPIKey:   true,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", modelName),
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want fail-closed key retrieval error")
	}
}

func TestLLMResolverNormalizesOllamaRuntimeBaseToOpenAICompatibleV1(t *testing.T) {
	apiBase := "http://ollama:11434"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "ollama",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "ollama", "ollama:11434", modelName),
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != "http://ollama:11434/v1" {
		t.Fatalf("BaseURL = %q, want Ollama OpenAI-compatible /v1 base", got.BaseURL)
	}
}

func TestLLMResolverRejectsEvalFailedSystemRoleConfig(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:       "tool_agent",
			APIBase:    &apiBase,
			ModelName:  &systemModel,
			Enabled:    true,
			EvalStatus: model.LLMEvalStatusFailed,
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want eval gate error")
	}
}

func TestLLMResolverAcceptsWaivedSystemRoleWithAdminReason(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &systemModel,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusWaived,
			EvalReport: mergeEvalReport(evalIdentityReport("tool_agent", "llama_server", "system.example", systemModel), map[string]any{
				"admin_reason": "temporary operator waiver",
			}),
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: systemModel, BackendType: "llama_server"})
}

func TestLLMResolverRejectsWaivedSystemRoleWithoutAdminReason(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:       "tool_agent",
			APIBase:    &apiBase,
			ModelName:  &systemModel,
			Enabled:    true,
			EvalStatus: model.LLMEvalStatusWaived,
			EvalReport: map[string]any{},
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want waiver reason error")
	}
}

func TestLLMResolverUserModelOverrideRequiresEvaluatedTuple(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	userModel := "user-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &systemModel,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport: mergeEvalReport(evalIdentityReport("tool_agent", "llama_server", "system.example", systemModel), map[string]any{"model_evals": []any{
				map[string]any{"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "system.example", "model_name": userModel, "status": "passed", "suite_id": model.LLMEvalSuiteID, "suite_hash": model.LLMEvalSuiteHash},
			}}),
		}},
		user: []model.LLMUserConfig{{
			UserID:    "user-1",
			Role:      "tool_agent",
			ModelName: &userModel,
		}},
	}, LLMResolverConfig{
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: userModel, BackendType: "llama_server"})
}

func TestLLMResolverRejectsUnevaluatedUserModelOverride(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	userModel := "user-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &systemModel,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  mergeEvalReport(evalIdentityReport("tool_agent", "llama_server", "system.example", systemModel), map[string]any{"model_evals": []any{}}),
		}},
		user: []model.LLMUserConfig{{
			UserID:    "user-1",
			Role:      "tool_agent",
			ModelName: &userModel,
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want unevaluated override error")
	}
}

func TestLLMResolverErrorsWhenEncryptedSystemKeyNeedsMissingEncryptionKey(t *testing.T) {
	enc, err := crypto.Encrypt("system-key", testLLMKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &systemModel,
			HasAPIKey:   true,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "system.example", systemModel),
		}},
		keys: map[string]string{"tool_agent": enc},
	}, LLMResolverConfig{})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want decrypt error")
	}
}

func TestLLMResolverRejectsStaleEvalIdentity(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:        "tool_agent",
			BackendType: "llama_server",
			APIBase:     &apiBase,
			ModelName:   &systemModel,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusPassed,
			EvalReport:  evalIdentityReport("tool_agent", "llama_server", "old.example", systemModel),
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want stale eval identity error")
	}
}

func TestLLMResolverRejectsLiteralDefaultRole(t *testing.T) {
	resolver := NewLLMResolver(fakeLLMStore{}, LLMResolverConfig{EncryptionKey: testLLMKey})

	if _, err := resolver.Resolve(context.Background(), "user-1", "default"); err == nil {
		t.Fatal("Resolve accepted legacy default role")
	}
}

func assertLLMConfig(t *testing.T, got, want LLMConfig) {
	t.Helper()
	if got != want {
		t.Fatalf("LLMConfig = %+v, want %+v", got, want)
	}
}

func evalIdentityReport(role, backendType, host, modelName string) map[string]any {
	return map[string]any{"identity": map[string]any{
		"role":          role,
		"backend_type":  backendType,
		"api_base_host": host,
		"model_name":    modelName,
		"suite_id":      model.LLMEvalSuiteID,
		"suite_hash":    model.LLMEvalSuiteHash,
	}}
}

func evalIdentityReportWithContext(role, backendType, host, modelName string, contextWindow int) map[string]any {
	report := evalIdentityReport(role, backendType, host, modelName)
	report["identity"].(map[string]any)["context_window"] = contextWindow
	return report
}

func mergeEvalReport(base map[string]any, extra map[string]any) map[string]any {
	for key, value := range extra {
		base[key] = value
	}
	return base
}

type fakeLLMStore struct {
	system []model.LLMSystemConfig
	user   []model.LLMUserConfig
	keys   map[string]string
	err    error
}

func (s fakeLLMStore) GetLLMSystemConfig(context.Context) ([]model.LLMSystemConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.system, nil
}

func (s fakeLLMStore) GetLLMAPIKeyEnc(_ context.Context, role string) (string, error) {
	if s.keys == nil || s.keys[role] == "" {
		return "", errors.New("not found")
	}
	return s.keys[role], nil
}

func (s fakeLLMStore) GetLLMUserConfig(context.Context, string) ([]model.LLMUserConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}
