package model

import (
	"strings"
	"time"
)

const (
	LLMRoleToolAgent       = "tool_agent"
	LLMRoleSQLReadonly     = "sql_readonly"
	LLMRoleWritePreview    = "write_preview"
	LLMRoleDocumentExtract = "document_extract"
	LLMRoleMarkdownExtract = "markdown_extract"
	LLMRoleVision          = "vision"

	LLMEvalStatusUntested = "untested"
	LLMEvalStatusPassed   = "passed"
	LLMEvalStatusFailed   = "failed"
	LLMEvalStatusWaived   = "waived"
)

var ValidLLMRoles = []string{
	LLMRoleToolAgent,
	LLMRoleSQLReadonly,
	LLMRoleWritePreview,
	LLMRoleDocumentExtract,
	LLMRoleMarkdownExtract,
	LLMRoleVision,
}

type LLMSystemConfig struct {
	Role            string         `json:"role"`
	BackendType     string         `json:"backend_type"`
	APIBase         *string        `json:"api_base,omitempty"`
	ModelName       *string        `json:"model_name,omitempty"`
	APIKeyMasked    *string        `json:"api_key_masked,omitempty"` // never return plaintext
	HasAPIKey       bool           `json:"has_api_key"`
	Enabled         bool           `json:"enabled"`
	CapabilityFlags []string       `json:"capability_flags"`
	EvalStatus      string         `json:"eval_status"`
	EvalReport      map[string]any `json:"eval_report"`
	UpdatedAt       time.Time      `json:"updated_at"`
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
	Warning   string `json:"warning,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

func IsValidLLMRole(role string) bool {
	for _, valid := range ValidLLMRoles {
		if role == valid {
			return true
		}
	}
	return false
}

func (c LLMSystemConfig) EvalGatePassed() bool {
	if !c.Enabled {
		return false
	}
	switch c.EvalStatus {
	case LLMEvalStatusPassed:
		return true
	case LLMEvalStatusWaived:
		return c.HasAdminWaiverReason()
	default:
		return false
	}
}

func (c LLMSystemConfig) HasAdminWaiverReason() bool {
	for _, key := range []string{"admin_reason", "waiver_reason"} {
		if v, ok := c.EvalReport[key].(string); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func (c LLMSystemConfig) AllowsModelOverride(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return true
	}
	if c.ModelName != nil && strings.TrimSpace(*c.ModelName) == modelName {
		return c.EvalGatePassed()
	}
	for _, record := range llmEvalRecords(c.EvalReport["model_evals"]) {
		if record.matches(c.Role, c.BackendType, modelName) && record.gatePassed() {
			return true
		}
	}
	return false
}

type llmEvalRecord map[string]any

func llmEvalRecords(raw any) []llmEvalRecord {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	records := make([]llmEvalRecord, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			records = append(records, record)
		}
	}
	return records
}

func (r llmEvalRecord) matches(role, backendType, modelName string) bool {
	return stringValue(r["role"]) == role &&
		stringValue(r["backend_type"]) == backendType &&
		stringValue(r["model_name"]) == modelName
}

func (r llmEvalRecord) gatePassed() bool {
	switch stringValue(r["status"]) {
	case LLMEvalStatusPassed:
		return true
	case LLMEvalStatusWaived:
		return strings.TrimSpace(stringValue(r["admin_reason"])) != "" ||
			strings.TrimSpace(stringValue(r["waiver_reason"])) != ""
	default:
		return false
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
