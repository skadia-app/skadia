package model

import (
	"net/url"
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

type LLMEvalIdentity struct {
	Role        string `json:"role"`
	BackendType string `json:"backend_type"`
	APIBaseHost string `json:"api_base_host"`
	ModelName   string `json:"model_name"`
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
	if !c.EvalIdentityMatchesCurrentConfig() {
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

func (c LLMSystemConfig) CurrentEvalIdentity() (LLMEvalIdentity, bool) {
	modelName := ""
	if c.ModelName != nil {
		modelName = strings.TrimSpace(*c.ModelName)
	}
	identity := LLMEvalIdentity{
		Role:        strings.TrimSpace(c.Role),
		BackendType: strings.TrimSpace(c.BackendType),
		APIBaseHost: apiBaseHost(c.APIBase),
		ModelName:   modelName,
	}
	return identity, identity.Role != "" && identity.BackendType != "" && identity.APIBaseHost != "" && identity.ModelName != ""
}

func (c LLMSystemConfig) EvalIdentityMatchesCurrentConfig() bool {
	current, ok := c.CurrentEvalIdentity()
	if !ok {
		return false
	}
	reportIdentity, ok := evalIdentityFromReport(c.EvalReport["identity"])
	return ok && reportIdentity == current
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
	apiBaseHost := apiBaseHost(c.APIBase)
	if apiBaseHost == "" {
		return false
	}
	for _, record := range llmEvalRecords(c.EvalReport["model_evals"]) {
		if record.matches(c.Role, c.BackendType, apiBaseHost, modelName) && record.gatePassed() {
			return true
		}
	}
	return false
}

type llmEvalRecord map[string]any

func evalIdentityFromReport(raw any) (LLMEvalIdentity, bool) {
	record, ok := raw.(map[string]any)
	if !ok {
		return LLMEvalIdentity{}, false
	}
	identity := LLMEvalIdentity{
		Role:        stringValue(record["role"]),
		BackendType: stringValue(record["backend_type"]),
		APIBaseHost: stringValue(record["api_base_host"]),
		ModelName:   stringValue(record["model_name"]),
	}
	return identity, identity.Role != "" && identity.BackendType != "" && identity.APIBaseHost != "" && identity.ModelName != ""
}

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

func (r llmEvalRecord) matches(role, backendType, apiBaseHost, modelName string) bool {
	return stringValue(r["role"]) == role &&
		stringValue(r["backend_type"]) == backendType &&
		stringValue(r["api_base_host"]) == apiBaseHost &&
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

func apiBaseHost(raw *string) string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return ""
	}
	return parsed.Host
}
