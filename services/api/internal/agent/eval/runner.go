package eval

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"net/url"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	CategorySQLReadonly        = "sql_readonly"
	CategoryToolCall           = "tool_call"
	CategoryDocumentExtract    = "document_extract"
	CategoryMarkdownNoLoop     = "markdown_no_loop"
	CategoryVision             = "vision"
	CategoryWritePreview       = "write_preview"
	CategoryGermanFactual      = "german_factual"
	CategoryStreamingToolDelta = "streaming_tool_delta"
)

type Store interface {
	GetLLMSystemConfig(ctx context.Context) ([]model.LLMSystemConfig, error)
	GetLLMAPIKeyEnc(ctx context.Context, role string) (string, error)
	UpdateLLMEvalResult(ctx context.Context, role, status string, report any) error
}

type ModelEvalStore interface {
	UpdateLLMModelEvalResult(ctx context.Context, role string, report any) error
}

type Client interface {
	RunCase(ctx context.Context, candidate Candidate, tc Case) (Transcript, error)
}

type RunnerOptions struct {
	Cases             []Case
	Now               func() time.Time
	EncryptionKey     string
	ModelNameOverride string
}

type Runner struct {
	store             Store
	client            Client
	cases             []Case
	now               func() time.Time
	key               string
	suiteID           string
	suiteHash         string
	modelNameOverride string
}

type Candidate struct {
	Identity
	APIBase      string
	APIKey       string
	Capabilities []string
}

type Identity struct {
	Role        string `json:"role"`
	BackendType string `json:"backend_type"`
	APIBaseHost string `json:"api_base_host"`
	ModelName   string `json:"model_name"`
	SuiteID     string `json:"suite_id"`
	SuiteHash   string `json:"suite_hash"`
}

type Case struct {
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	Category             string       `json:"category"`
	Roles                []string     `json:"roles,omitempty"`
	RequiredCapabilities []string     `json:"required_capabilities,omitempty"`
	Prompt               string       `json:"prompt"`
	ImageDataURL         string       `json:"image_data_url,omitempty"`
	Expectations         Expectations `json:"expectations"`
}

type Expectations struct {
	MustContain           []string        `json:"must_contain,omitempty"`
	MustNotContain        []string        `json:"must_not_contain,omitempty"`
	RequireToolCall       string          `json:"require_tool_call,omitempty"`
	ToolArgsSchema        json.RawMessage `json:"tool_args_schema,omitempty"`
	FinishReason          string          `json:"finish_reason,omitempty"`
	RequireSQLTenantGuard bool            `json:"require_sql_tenant_guard,omitempty"`
	RequireJSONOutput     bool            `json:"require_json_output,omitempty"`
	MinContentChars       int             `json:"min_content_chars,omitempty"`
}

type Transcript struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type Report struct {
	Status              string       `json:"status"`
	Identity            Identity     `json:"identity"`
	SuiteID             string       `json:"suite_id"`
	SuiteHash           string       `json:"suite_hash"`
	SuiteCaseCount      int          `json:"suite_case_count"`
	LiteLLMPendingApply bool         `json:"litellm_pending_apply,omitempty"`
	Completed           bool         `json:"completed"`
	StartedAt           time.Time    `json:"started_at"`
	CompletedAt         time.Time    `json:"completed_at"`
	Cases               []CaseResult `json:"cases"`
	FailedCases         []string     `json:"failed_cases,omitempty"`
}

type CaseResult struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
}

func NewRunner(store Store, client Client, opts RunnerOptions) *Runner {
	cases := opts.Cases
	suiteHash := ""
	if len(cases) == 0 {
		cases = DefaultCases()
		suiteHash = DefaultSuiteHash()
	} else {
		suiteHash = hashCases(cases)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Runner{store: store, client: client, cases: cases, now: now, key: opts.EncryptionKey, suiteID: model.LLMEvalSuiteID, suiteHash: suiteHash, modelNameOverride: strings.TrimSpace(opts.ModelNameOverride)}
}

func (r *Runner) Run(ctx context.Context, role string) (Report, error) {
	if r.store == nil {
		return Report{}, errors.New("eval store is not configured")
	}
	if r.client == nil {
		return Report{}, errors.New("eval client is not configured")
	}
	role = strings.TrimSpace(role)
	if !model.IsValidLLMRole(role) {
		return Report{}, fmt.Errorf("invalid LLM role %q", role)
	}

	candidate, err := r.loadCandidate(ctx, role)
	if err != nil {
		return Report{}, err
	}
	if r.modelNameOverride != "" {
		candidate.Identity.ModelName = r.modelNameOverride
	}
	candidate.Identity.SuiteID = r.suiteID
	candidate.Identity.SuiteHash = r.suiteHash
	cases := r.applicableCases(candidate)
	if len(cases) == 0 {
		return Report{}, fmt.Errorf("no eval cases apply to LLM role %q and capabilities %v", role, candidate.Capabilities)
	}

	started := r.now().UTC()
	report := Report{
		Status:         model.LLMEvalStatusFailed,
		Identity:       candidate.Identity,
		SuiteID:        r.suiteID,
		SuiteHash:      r.suiteHash,
		SuiteCaseCount: len(cases),
		StartedAt:      started,
		Cases:          make([]CaseResult, 0, len(cases)),
	}
	for _, tc := range cases {
		transcript, err := r.client.RunCase(ctx, candidate, tc)
		result := validateCase(tc, transcript)
		if err != nil {
			result.Passed = false
			result.Failures = append(result.Failures, err.Error())
		}
		if !result.Passed {
			report.FailedCases = append(report.FailedCases, tc.ID)
		}
		report.Cases = append(report.Cases, result)
	}
	report.Completed = true
	report.CompletedAt = r.now().UTC()
	if len(report.FailedCases) == 0 {
		report.Status = model.LLMEvalStatusPassed
	}

	if r.modelNameOverride != "" {
		store, ok := r.store.(ModelEvalStore)
		if !ok {
			return Report{}, errors.New("eval store cannot persist model override results")
		}
		if err := store.UpdateLLMModelEvalResult(ctx, role, report); err != nil {
			return Report{}, fmt.Errorf("store model eval result: %w", err)
		}
	} else if err := r.store.UpdateLLMEvalResult(ctx, role, report.Status, report); err != nil {
		return Report{}, fmt.Errorf("store eval result: %w", err)
	}
	return report, nil
}

func (r *Runner) loadCandidate(ctx context.Context, role string) (Candidate, error) {
	configs, err := r.store.GetLLMSystemConfig(ctx)
	if err != nil {
		return Candidate{}, fmt.Errorf("get system LLM config: %w", err)
	}
	var cfg *model.LLMSystemConfig
	for i := range configs {
		if configs[i].Role == role {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil || !cfg.Enabled {
		return Candidate{}, fmt.Errorf("LLM role %q is not configured", role)
	}
	if cfg.APIBase == nil || strings.TrimSpace(*cfg.APIBase) == "" || cfg.ModelName == nil || strings.TrimSpace(*cfg.ModelName) == "" {
		return Candidate{}, fmt.Errorf("LLM role %q configuration is incomplete", role)
	}
	identity, err := IdentityForConfig(*cfg)
	if err != nil {
		return Candidate{}, err
	}
	apiBase, err := model.NormalizeLLMAPIBase(cfg.BackendType, *cfg.APIBase)
	if err != nil {
		return Candidate{}, fmt.Errorf("LLM role %q has invalid api_base: %w", role, err)
	}
	candidate := Candidate{
		Identity:     identity,
		APIBase:      apiBase,
		Capabilities: append([]string(nil), cfg.CapabilityFlags...),
	}
	if cfg.HasAPIKey {
		key, err := r.store.GetLLMAPIKeyEnc(ctx, role)
		if err != nil {
			return Candidate{}, fmt.Errorf("get eval LLM api key: %w", err)
		}
		if strings.TrimSpace(key) == "" {
			return Candidate{}, fmt.Errorf("LLM role %q is marked with an API key but no key is available", role)
		}
		dec, err := crypto.Decrypt(key, r.key)
		if err != nil {
			return Candidate{}, fmt.Errorf("decrypt eval LLM api key: %w", err)
		}
		candidate.APIKey = dec
	}
	return candidate, nil
}

func (r *Runner) applicableCases(candidate Candidate) []Case {
	out := make([]Case, 0, len(r.cases))
	for _, tc := range r.cases {
		if !caseMatchesRole(tc, candidate.Role) {
			continue
		}
		if !caseCapabilitiesSatisfied(tc, candidate.Capabilities) {
			continue
		}
		out = append(out, tc)
	}
	return out
}

func caseMatchesRole(tc Case, role string) bool {
	if len(tc.Roles) == 0 {
		return true
	}
	for _, candidateRole := range tc.Roles {
		if candidateRole == role {
			return true
		}
	}
	return false
}

func caseCapabilitiesSatisfied(tc Case, capabilities []string) bool {
	if len(tc.RequiredCapabilities) == 0 {
		return true
	}
	have := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		have[capability] = true
	}
	for _, required := range tc.RequiredCapabilities {
		if !have[required] {
			return false
		}
	}
	return true
}

func IdentityForConfig(cfg model.LLMSystemConfig) (Identity, error) {
	if cfg.APIBase == nil || strings.TrimSpace(*cfg.APIBase) == "" {
		return Identity{}, errors.New("api_base is required for eval identity")
	}
	if cfg.ModelName == nil || strings.TrimSpace(*cfg.ModelName) == "" {
		return Identity{}, errors.New("model_name is required for eval identity")
	}
	parsed, err := url.Parse(strings.TrimSpace(*cfg.APIBase))
	if err != nil {
		return Identity{}, fmt.Errorf("parse api_base: %w", err)
	}
	if parsed.Host == "" {
		return Identity{}, errors.New("api_base host is required for eval identity")
	}
	return Identity{
		Role:        strings.TrimSpace(cfg.Role),
		BackendType: strings.TrimSpace(cfg.BackendType),
		APIBaseHost: parsed.Host,
		ModelName:   strings.TrimSpace(*cfg.ModelName),
		SuiteID:     model.LLMEvalSuiteID,
		SuiteHash:   model.LLMEvalSuiteHash,
	}, nil
}

func validateCase(tc Case, transcript Transcript) CaseResult {
	result := CaseResult{
		ID:       tc.ID,
		Name:     tc.Name,
		Category: tc.Category,
		Passed:   true,
	}
	content := strings.ToLower(transcript.Content)
	if strings.TrimSpace(transcript.Content) == "" && tc.Expectations.RequireToolCall == "" {
		result.Passed = false
		result.Failures = append(result.Failures, "empty content")
	}
	if tc.Expectations.MinContentChars > 0 && len([]rune(strings.TrimSpace(transcript.Content))) < tc.Expectations.MinContentChars {
		result.Passed = false
		result.Failures = append(result.Failures, fmt.Sprintf("content shorter than %d chars", tc.Expectations.MinContentChars))
	}
	for _, want := range tc.Expectations.MustContain {
		if !strings.Contains(content, strings.ToLower(want)) {
			result.Passed = false
			result.Failures = append(result.Failures, fmt.Sprintf("missing content %q", want))
		}
	}
	for _, forbidden := range tc.Expectations.MustNotContain {
		if strings.Contains(content, strings.ToLower(forbidden)) {
			result.Passed = false
			result.Failures = append(result.Failures, fmt.Sprintf("forbidden content %q", forbidden))
		}
	}
	if tc.Expectations.RequireToolCall != "" && !hasToolCall(transcript, tc.Expectations.RequireToolCall) {
		result.Passed = false
		result.Failures = append(result.Failures, fmt.Sprintf("missing tool call %q", tc.Expectations.RequireToolCall))
	}
	if tc.Expectations.RequireToolCall != "" && len(tc.Expectations.ToolArgsSchema) > 0 {
		call, ok := findToolCall(transcript, tc.Expectations.RequireToolCall)
		if !ok {
			result.Passed = false
		} else if err := validateToolArguments(tc.Expectations.RequireToolCall, tc.Expectations.ToolArgsSchema, call.Arguments); err != nil {
			result.Passed = false
			result.Failures = append(result.Failures, fmt.Sprintf("invalid tool arguments for %q: %v", tc.Expectations.RequireToolCall, err))
		}
	}
	if tc.Expectations.FinishReason != "" && transcript.FinishReason != tc.Expectations.FinishReason {
		result.Passed = false
		result.Failures = append(result.Failures, fmt.Sprintf("finish_reason = %q, want %q", transcript.FinishReason, tc.Expectations.FinishReason))
	}
	if tc.Expectations.RequireSQLTenantGuard && !hasSQLTenantGuard(transcript.Content) {
		result.Passed = false
		result.Failures = append(result.Failures, "missing SQL tenant guard")
	}
	if tc.Category == CategorySQLReadonly && looksLikeSQL(transcript.Content) {
		if err := agent.ValidateSQL(agent.AutofixSQL(transcript.Content)); err != nil {
			result.Passed = false
			result.Failures = append(result.Failures, fmt.Sprintf("SQL rejected by runtime validator: %v", err))
		}
	}
	if tc.Expectations.RequireJSONOutput {
		var parsed any
		if err := json.Unmarshal([]byte(strings.TrimSpace(transcript.Content)), &parsed); err != nil {
			result.Passed = false
			result.Failures = append(result.Failures, fmt.Sprintf("invalid JSON output: %v", err))
		}
		switch parsed.(type) {
		case map[string]any, []any:
		default:
			result.Passed = false
			result.Failures = append(result.Failures, "JSON output must be an object or array")
		}
	}
	return result
}

func hasToolCall(transcript Transcript, name string) bool {
	_, ok := findToolCall(transcript, name)
	return ok
}

func findToolCall(transcript Transcript, name string) (ToolCall, bool) {
	for _, call := range transcript.ToolCalls {
		if call.Name == name {
			return call, true
		}
	}
	return ToolCall{}, false
}

func validateToolArguments(toolName string, schema json.RawMessage, args string) error {
	args = strings.TrimSpace(args)
	if args == "" {
		return fmt.Errorf("empty arguments")
	}
	var schemaDoc any
	if err := json.Unmarshal(schema, &schemaDoc); err != nil {
		return fmt.Errorf("parse tool schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	url := fmt.Sprintf("agent/eval/%s.args.schema.json", toolName)
	if err := compiler.AddResource(url, schemaDoc); err != nil {
		return err
	}
	compiled, err := compiler.Compile(url)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(args)))
	if err != nil {
		return err
	}
	return compiled.Validate(instance)
}

func hasSQLTenantGuard(sql string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
	return strings.Contains(normalized, "current_setting('app.current_user_id") ||
		strings.Contains(normalized, `current_setting("app.current_user_id`)
}

func looksLikeSQL(content string) bool {
	first, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(content)), " ")
	return first == "select" || first == "with"
}

func decodeCases(raw []byte) ([]Case, error) {
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	if err := validateCases(cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func hashCases(cases []Case) string {
	raw, err := json.Marshal(cases)
	if err != nil {
		return ""
	}
	return hashSuiteBytes(raw)
}

func validateCases(cases []Case) error {
	if len(cases) == 0 {
		return errors.New("eval suite has no cases")
	}
	seen := make(map[string]bool, len(cases))
	for i, tc := range cases {
		label := fmt.Sprintf("case[%d]", i)
		if strings.TrimSpace(tc.ID) == "" {
			return fmt.Errorf("%s id is required", label)
		}
		label = fmt.Sprintf("case %q", tc.ID)
		if seen[tc.ID] {
			return fmt.Errorf("duplicate eval case id %q", tc.ID)
		}
		seen[tc.ID] = true
		if !isKnownCategory(tc.Category) {
			return fmt.Errorf("%s has unknown category %q", label, tc.Category)
		}
		for _, role := range tc.Roles {
			if !model.IsValidLLMRole(role) {
				return fmt.Errorf("%s has unknown role %q", label, role)
			}
		}
		for _, capability := range tc.RequiredCapabilities {
			if !isKnownCapability(capability) {
				return fmt.Errorf("%s has unknown capability %q", label, capability)
			}
		}
		if len(tc.Expectations.ToolArgsSchema) > 0 {
			if err := validateToolSchema(tc.Expectations.RequireToolCall, tc.Expectations.ToolArgsSchema); err != nil {
				return fmt.Errorf("%s has invalid tool_args_schema: %w", label, err)
			}
		}
		if tc.ImageDataURL != "" {
			if err := validateImageDataURL(tc.ImageDataURL); err != nil {
				return fmt.Errorf("%s has invalid image_data_url: %w", label, err)
			}
		}
	}
	return nil
}

func isKnownCategory(category string) bool {
	switch category {
	case CategorySQLReadonly, CategoryToolCall, CategoryDocumentExtract, CategoryMarkdownNoLoop,
		CategoryVision, CategoryWritePreview, CategoryGermanFactual, CategoryStreamingToolDelta:
		return true
	default:
		return false
	}
}

func isKnownCapability(capability string) bool {
	switch capability {
	case "streaming_chat", "tool_calls", "json_args", "german_answers",
		"sql_readonly", "write_preview", "document_extract", "json_extract",
		"long_context", "markdown_extract", "table_preservation", "vision":
		return true
	default:
		return false
	}
}

func validateToolSchema(toolName string, schema json.RawMessage) error {
	if strings.TrimSpace(toolName) == "" {
		toolName = "tool"
	}
	var schemaDoc any
	if err := json.Unmarshal(schema, &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	url := fmt.Sprintf("agent/eval/%s.validation.schema.json", toolName)
	if err := compiler.AddResource(url, schemaDoc); err != nil {
		return err
	}
	_, err := compiler.Compile(url)
	return err
}

func validateImageDataURL(raw string) error {
	meta, encoded, ok := strings.Cut(raw, ",")
	if !ok || !strings.HasPrefix(meta, "data:image/") || !strings.Contains(meta, ";base64") {
		return errors.New("must be a base64 image data URL")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return err
	}
	if format == "" || cfg.Width <= 0 || cfg.Height <= 0 {
		return errors.New("image has invalid dimensions")
	}
	return nil
}
