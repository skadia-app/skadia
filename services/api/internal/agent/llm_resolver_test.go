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
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
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
			Role:       "tool_agent",
			APIBase:    &apiBase,
			ModelName:  &modelName,
			Enabled:    true,
			EvalStatus: model.LLMEvalStatusPassed,
		}},
		keys: map[string]string{"tool_agent": enc},
	}, LLMResolverConfig{
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, APIKey: "system-key", Model: modelName})
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
			Role:       "tool_agent",
			APIBase:    &apiBase,
			ModelName:  &systemModel,
			Enabled:    true,
			EvalStatus: model.LLMEvalStatusWaived,
			EvalReport: map[string]any{"admin_reason": "temporary operator waiver"},
		}},
	}, LLMResolverConfig{EncryptionKey: testLLMKey})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, Model: systemModel})
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
			EvalReport: map[string]any{"model_evals": []any{
				map[string]any{"role": "tool_agent", "backend_type": "llama_server", "api_base_host": "system.example", "model_name": userModel, "status": "passed"},
			}},
		}},
		user: []model.LLMUserConfig{{
			UserID:    "user-1",
			Role:      "tool_agent",
			ModelName: &userModel,
		}},
	}, LLMResolverConfig{
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "tool_agent")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, APIKey: "env-key", Model: userModel})
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
			EvalReport:  map[string]any{"model_evals": []any{}},
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
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:       "tool_agent",
			Enabled:    true,
			EvalStatus: model.LLMEvalStatusPassed,
		}},
		keys: map[string]string{"tool_agent": enc},
	}, LLMResolverConfig{
		BaseURL: "http://env.example/v1",
		APIKey:  "env-key",
		Model:   "env-model",
	})

	if _, err := resolver.Resolve(context.Background(), "user-1", "tool_agent"); err == nil {
		t.Fatal("Resolve returned nil error, want decrypt error")
	}
}

func assertLLMConfig(t *testing.T, got, want LLMConfig) {
	t.Helper()
	if got != want {
		t.Fatalf("LLMConfig = %+v, want %+v", got, want)
	}
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
