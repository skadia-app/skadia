package model

import "time"

type LLMSystemConfig struct {
	Role         string    `json:"role"`
	BackendType  string    `json:"backend_type"`
	APIBase      *string   `json:"api_base,omitempty"`
	ModelName    *string   `json:"model_name,omitempty"`
	APIKeyMasked *string   `json:"api_key_masked,omitempty"` // never return plaintext
	HasAPIKey    bool      `json:"has_api_key"`
	Enabled      bool      `json:"enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LLMSystemConfigUpdate struct {
	BackendType string  `json:"backend_type"`
	APIBase     *string `json:"api_base,omitempty"`
	ModelName   *string `json:"model_name,omitempty"`
	APIKey      *string `json:"api_key,omitempty"` // plaintext input, encrypted before storage
	Enabled     bool    `json:"enabled"`
}

type LLMUserConfig struct {
	UserID    string  `json:"user_id"`
	Role      string  `json:"role"`
	ModelName *string `json:"model_name,omitempty"`
}

type OllamaModel struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

type LLMTestResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}
