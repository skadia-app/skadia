package model

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	LLMRoleToolAgent       = "tool_agent"
	LLMRoleDocumentExtract = "document_extract"
	LLMRoleMarkdownExtract = "markdown_extract"
	LLMRoleVision          = "vision"

	LLMEvalStatusUntested = "untested"
	LLMEvalStatusPassed   = "passed"
	LLMEvalStatusFailed   = "failed"
	LLMEvalStatusWaived   = "waived"

	LLMBackendDisabled    = "disabled"
	LLMBackendOllama      = "ollama"
	LLMBackendLlamaServer = "llama_server"
	LLMBackendOpenAI      = "openai"

	LLMAPIKeyStateUnchanged = "unchanged"
	LLMAPIKeyStateSet       = "set"
	LLMAPIKeyStateClear     = "clear"

	LLMEvalSuiteID   = "skadia-agent-eval-v2"
	LLMEvalSuiteHash = "367ddd89b5b30040743f3e2248a06f6cf69cee7f15478224589d51ddfcdd0a38"
)

var ValidLLMRoles = []string{
	LLMRoleToolAgent,
	LLMRoleDocumentExtract,
	LLMRoleMarkdownExtract,
	LLMRoleVision,
}

var ValidLLMBackendTypes = []string{
	LLMBackendDisabled,
	LLMBackendOllama,
	LLMBackendLlamaServer,
	LLMBackendOpenAI,
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
	APIKeyState string  `json:"api_key_state,omitempty"`
	Enabled     bool    `json:"enabled"`
}

type LLMEvalIdentity struct {
	Role        string `json:"role"`
	BackendType string `json:"backend_type"`
	APIBaseHost string `json:"api_base_host"`
	ModelName   string `json:"model_name"`
	SuiteID     string `json:"suite_id"`
	SuiteHash   string `json:"suite_hash"`
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

func IsValidLLMBackendType(backendType string) bool {
	for _, valid := range ValidLLMBackendTypes {
		if backendType == valid {
			return true
		}
	}
	return false
}

func IsDirectLLMBackendType(backendType string) bool {
	switch backendType {
	case LLMBackendOllama, LLMBackendLlamaServer, LLMBackendOpenAI:
		return true
	default:
		return false
	}
}

func IsValidLLMAPIKeyState(state string) bool {
	switch state {
	case LLMAPIKeyStateUnchanged, LLMAPIKeyStateSet, LLMAPIKeyStateClear:
		return true
	default:
		return false
	}
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
		SuiteID:     LLMEvalSuiteID,
		SuiteHash:   LLMEvalSuiteHash,
	}
	return identity, identity.Role != "" && identity.BackendType != "" && identity.APIBaseHost != "" && identity.ModelName != "" && identity.SuiteID != "" && identity.SuiteHash != ""
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
		SuiteID:     stringValue(record["suite_id"]),
		SuiteHash:   stringValue(record["suite_hash"]),
	}
	return identity, identity.Role != "" && identity.BackendType != "" && identity.APIBaseHost != "" && identity.ModelName != "" && identity.SuiteID != "" && identity.SuiteHash != ""
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
	return llmEvalRecordString(r, "role") == role &&
		llmEvalRecordString(r, "backend_type") == backendType &&
		llmEvalRecordString(r, "api_base_host") == apiBaseHost &&
		llmEvalRecordString(r, "model_name") == modelName &&
		llmEvalRecordString(r, "suite_id") == LLMEvalSuiteID &&
		llmEvalRecordString(r, "suite_hash") == LLMEvalSuiteHash
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

func llmEvalRecordString(r llmEvalRecord, key string) string {
	if value := stringValue(r[key]); value != "" {
		return value
	}
	identity, ok := r["identity"].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(identity[key])
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

func NormalizeLLMAPIBase(backendType, raw string) (string, error) {
	backendType = strings.TrimSpace(backendType)
	raw = strings.TrimSpace(raw)
	if backendType == LLMBackendDisabled {
		if raw == "" {
			return "", nil
		}
		return "", fmt.Errorf("disabled LLM backend must not store api_base")
	}
	if !IsDirectLLMBackendType(backendType) {
		return "", fmt.Errorf("unsupported direct LLM backend_type %q", backendType)
	}
	if raw == "" {
		return "", fmt.Errorf("api_base is required for backend_type %q", backendType)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse api_base: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("api_base scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("api_base host is required")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("api_base must not contain query or fragment")
	}
	rewriteLocalLLMHost(parsed)
	if isRecursiveLiteLLMAPIBase(parsed.Hostname(), parsed.Port()) {
		return "", fmt.Errorf("api_base must point to a direct provider, not the LiteLLM gateway")
	}
	if LLMBackendAddrBlocked(backendType, parsed.Hostname(), parsed.Port()) {
		return "", fmt.Errorf("api_base address %q is blocked by LLM SSRF policy", parsed.Host)
	}
	if err := normalizeOpenAICompatiblePath(parsed); err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeOpenAICompatiblePath(parsed *url.URL) error {
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	switch path {
	case "", "/v1":
		parsed.Path = "/v1"
		parsed.RawPath = ""
		return nil
	default:
		return fmt.Errorf("api_base path must be empty or /v1 for the direct chat-completions adapter")
	}
}

func rewriteLocalLLMHost(parsed *url.URL) {
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort("host.containers.internal", port)
		return
	}
	parsed.Host = "host.containers.internal"
}

func isRecursiveLiteLLMAPIBase(host, port string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	port = strings.TrimSpace(port)
	return (host == "litellm" || host == "host.containers.internal") && port == "4000"
}

func LLMBackendHostBlocked(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || isAllowedLocalLLMBackendHost(host) {
		return false
	}
	if host == "localhost" {
		return true
	}
	ips, err := net.LookupHost(host)
	if err != nil {
		ips = []string{host}
	}
	for _, ip := range ips {
		if isBlockedLLMBackendIP(ip) {
			return true
		}
	}
	return false
}

func LLMBackendAddrBlocked(backendType, host, port string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	port = strings.TrimSpace(port)
	if host == "" {
		return false
	}
	if isAllowedLocalLLMBackendHost(host) {
		return !isAllowedLocalLLMBackendPort(backendType, host, port)
	}
	return LLMBackendHostBlocked(host)
}

func isAllowedLocalLLMBackendHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "ollama", "litellm", "host.containers.internal":
		return true
	default:
		return false
	}
}

func isAllowedLocalLLMBackendPort(backendType, host, port string) bool {
	if port == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "ollama":
		return port == "11434"
	case "litellm":
		return port == "4000"
	case "host.containers.internal":
		switch strings.TrimSpace(backendType) {
		case LLMBackendOllama:
			return port == "11434"
		case LLMBackendLlamaServer:
			return port == "8000" || port == "8080"
		default:
			return false
		}
	default:
		return false
	}
}

func isBlockedLLMBackendIP(ipStr string) bool {
	if after, ok := strings.CutPrefix(ipStr, "::ffff:"); ok {
		ipStr = after
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.Equal(net.IPv4zero) || ipStr == "0.0.0.0" {
		return true
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified()
}
