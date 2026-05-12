package agent

import (
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

func NewLLMClient(cfg LLMConfig) *llmwire.ChatCompletionsClient {
	return llmwire.NewChatCompletionsClient(cfg.BaseURL, cfg.APIKey, nil)
}
