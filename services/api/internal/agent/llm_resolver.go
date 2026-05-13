package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type LLMConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	BackendType string
}

type LLMResolverConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	EncryptionKey string
}

type LLMConfigStore interface {
	GetLLMSystemConfig(ctx context.Context) ([]model.LLMSystemConfig, error)
	GetLLMAPIKeyEnc(ctx context.Context, role string) (string, error)
	GetLLMUserConfig(ctx context.Context, userID string) ([]model.LLMUserConfig, error)
}

type LLMResolver struct {
	store LLMConfigStore
	cfg   LLMResolverConfig
}

func NewLLMResolver(store LLMConfigStore, cfg LLMResolverConfig) *LLMResolver {
	return &LLMResolver{store: store, cfg: cfg}
}

func (c LLMConfig) SupportsStreamingUsage() bool {
	return c.BackendType == model.LLMBackendOpenAI
}

func (c LLMConfig) SupportsParallelToolControl() bool {
	return c.BackendType == model.LLMBackendOpenAI
}

func (r *LLMResolver) Resolve(ctx context.Context, userID, role string) (LLMConfig, error) {
	if strings.TrimSpace(role) == "" {
		role = model.LLMRoleToolAgent
	}
	if !model.IsValidLLMRole(role) {
		return LLMConfig{}, fmt.Errorf("invalid LLM role %q", role)
	}

	systemConfigs, err := r.store.GetLLMSystemConfig(ctx)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("get system llm config: %w", err)
	}
	var system *model.LLMSystemConfig
	for _, cfg := range systemConfigs {
		if cfg.Role == role {
			system = &cfg
			break
		}
	}
	if system == nil || !system.Enabled {
		return LLMConfig{}, fmt.Errorf("LLM role %q is not configured", role)
	}
	if !system.EvalGatePassed() {
		return LLMConfig{}, fmt.Errorf("LLM role %q has not passed eval gates", role)
	}
	if system.APIBase == nil || strings.TrimSpace(*system.APIBase) == "" || system.ModelName == nil || strings.TrimSpace(*system.ModelName) == "" {
		return LLMConfig{}, fmt.Errorf("LLM role %q configuration is incomplete", role)
	}
	baseURL, err := model.NormalizeLLMAPIBase(system.BackendType, *system.APIBase)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("LLM role %q has invalid api_base: %w", role, err)
	}

	resolved := LLMConfig{
		BaseURL:     baseURL,
		Model:       strings.TrimSpace(*system.ModelName),
		BackendType: system.BackendType,
	}
	if system.HasAPIKey {
		enc, err := r.store.GetLLMAPIKeyEnc(ctx, role)
		if err != nil {
			return LLMConfig{}, fmt.Errorf("get system llm api key: %w", err)
		}
		if strings.TrimSpace(enc) == "" {
			return LLMConfig{}, fmt.Errorf("LLM role %q is marked with an API key but no key is available", role)
		}
		dec, err := crypto.Decrypt(enc, r.cfg.EncryptionKey)
		if err != nil {
			return LLMConfig{}, fmt.Errorf("decrypt system llm api key: %w", err)
		}
		resolved.APIKey = dec
	}

	userConfigs, err := r.store.GetLLMUserConfig(ctx, userID)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("get user llm config: %w", err)
	}
	for _, cfg := range userConfigs {
		if cfg.Role == role && cfg.ModelName != nil && strings.TrimSpace(*cfg.ModelName) != "" {
			overrideModel := strings.TrimSpace(*cfg.ModelName)
			if !system.AllowsModelOverride(overrideModel) {
				return LLMConfig{}, fmt.Errorf("LLM role %q model override %q has not passed eval gates", role, overrideModel)
			}
			resolved.Model = overrideModel
			break
		}
	}

	return resolved, nil
}
