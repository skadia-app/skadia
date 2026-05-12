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
		role = "default"
	}

	resolved := LLMConfig{
		BaseURL: r.cfg.BaseURL,
		APIKey:  r.cfg.APIKey,
		Model:   r.cfg.Model,
	}

	systemConfigs, err := r.store.GetLLMSystemConfig(ctx)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("get system llm config: %w", err)
	}
	for _, cfg := range systemConfigs {
		if cfg.Role != role || !cfg.Enabled {
			continue
		}
		if cfg.APIBase != nil && strings.TrimSpace(*cfg.APIBase) != "" {
			resolved.BaseURL = strings.TrimSpace(*cfg.APIBase)
		}
		if cfg.ModelName != nil && strings.TrimSpace(*cfg.ModelName) != "" {
			resolved.Model = strings.TrimSpace(*cfg.ModelName)
		}
		if enc, err := r.store.GetLLMAPIKeyEnc(ctx, role); err == nil && strings.TrimSpace(enc) != "" {
			dec, err := crypto.Decrypt(enc, r.cfg.EncryptionKey)
			if err != nil {
				return LLMConfig{}, fmt.Errorf("decrypt system llm api key: %w", err)
			}
			resolved.APIKey = dec
		}
		break
	}

	userConfigs, err := r.store.GetLLMUserConfig(ctx, userID)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("get user llm config: %w", err)
	}
	for _, cfg := range userConfigs {
		if cfg.Role == role && cfg.ModelName != nil && strings.TrimSpace(*cfg.ModelName) != "" {
			resolved.Model = strings.TrimSpace(*cfg.ModelName)
			break
		}
	}

	return resolved, nil
}
