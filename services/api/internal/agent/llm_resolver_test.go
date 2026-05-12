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

func TestLLMResolverUsesEnvFallbackWhenDBEmpty(t *testing.T) {
	resolver := NewLLMResolver(fakeLLMStore{}, LLMResolverConfig{
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: "http://env.example/v1", APIKey: "env-key", Model: "env-model"})
}

func TestLLMResolverUsesSystemConfig(t *testing.T) {
	enc, err := crypto.Encrypt("system-key", testLLMKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	apiBase := "http://system.example/v1"
	modelName := "system-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:      "default",
			APIBase:   &apiBase,
			ModelName: &modelName,
			Enabled:   true,
		}},
		keys: map[string]string{"default": enc},
	}, LLMResolverConfig{
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, APIKey: "system-key", Model: modelName})
}

func TestLLMResolverUserModelOverridesSystemModel(t *testing.T) {
	apiBase := "http://system.example/v1"
	systemModel := "system-model"
	userModel := "user-model"
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:      "default",
			APIBase:   &apiBase,
			ModelName: &systemModel,
			Enabled:   true,
		}},
		user: []model.LLMUserConfig{{
			UserID:    "user-1",
			Role:      "default",
			ModelName: &userModel,
		}},
	}, LLMResolverConfig{
		BaseURL:       "http://env.example/v1",
		APIKey:        "env-key",
		Model:         "env-model",
		EncryptionKey: testLLMKey,
	})

	got, err := resolver.Resolve(context.Background(), "user-1", "default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertLLMConfig(t, got, LLMConfig{BaseURL: apiBase, APIKey: "env-key", Model: userModel})
}

func TestLLMResolverErrorsWhenEncryptedSystemKeyNeedsMissingEncryptionKey(t *testing.T) {
	enc, err := crypto.Encrypt("system-key", testLLMKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	resolver := NewLLMResolver(fakeLLMStore{
		system: []model.LLMSystemConfig{{
			Role:    "default",
			Enabled: true,
		}},
		keys: map[string]string{"default": enc},
	}, LLMResolverConfig{
		BaseURL: "http://env.example/v1",
		APIKey:  "env-key",
		Model:   "env-model",
	})

	if _, err := resolver.Resolve(context.Background(), "user-1", "default"); err == nil {
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
