package agent

import (
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

func NewLLMClient(cfg LLMConfig) *llmwire.ChatCompletionsClient {
	return llmwire.NewChatCompletionsClient(
		cfg.BaseURL,
		cfg.APIKey,
		llmwire.NewSSRFProtectedHTTPClient(10*time.Minute, nil),
	)
}
