package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
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

func (r *LLMResolver) Resolve(ctx context.Context, userID, role string) (LLMConfig, error) {
	if strings.TrimSpace(role) == "" {
		role = model.LLMRoleToolAgent
	}
	if role == "default" {
		role = model.LLMRoleToolAgent
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

	resolved := LLMConfig{
		BaseURL: strings.TrimSpace(*system.APIBase),
		APIKey:  r.cfg.APIKey,
		Model:   strings.TrimSpace(*system.ModelName),
	}
	if enc, err := r.store.GetLLMAPIKeyEnc(ctx, role); err == nil && strings.TrimSpace(enc) != "" {
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
