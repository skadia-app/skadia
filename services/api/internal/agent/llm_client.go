package agent

import (
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func NewLLMClient(cfg LLMConfig) *openai.Client {
	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(cfg.APIKey),
	)
	return &client
}
