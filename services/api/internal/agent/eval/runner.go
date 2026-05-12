package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	CategorySQLReadonly        = "sql_readonly"
	CategoryToolCall           = "tool_call"
	CategoryMarkdownNoLoop     = "markdown_no_loop"
	CategoryWritePreview       = "write_preview"
	CategoryGermanFactual      = "german_factual"
	CategoryStreamingToolDelta = "streaming_tool_delta"
)

type Store interface {
	GetLLMSystemConfig(ctx context.Context) ([]model.LLMSystemConfig, error)
	GetLLMAPIKeyEnc(ctx context.Context, role string) (string, error)
	UpdateLLMEvalResult(ctx context.Context, role, status string, report any) error
}

type Client interface {
	RunCase(ctx context.Context, candidate Candidate, tc Case) (Transcript, error)
}

type RunnerOptions struct {
	Cases         []Case
	Now           func() time.Time
	EncryptionKey string
}

type Runner struct {
	store  Store
	client Client
	cases  []Case
	now    func() time.Time
	key    string
}

type Candidate struct {
	Identity
	APIBase string
	APIKey  string
}

type Identity struct {
	Role        string `json:"role"`
	BackendType string `json:"backend_type"`
	APIBaseHost string `json:"api_base_host"`
	ModelName   string `json:"model_name"`
}

type Case struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Category     string       `json:"category"`
	Prompt       string       `json:"prompt"`
	Expectations Expectations `json:"expectations"`
}

type Expectations struct {
	MustContain     []string `json:"must_contain,omitempty"`
	MustNotContain  []string `json:"must_not_contain,omitempty"`
	RequireToolCall string   `json:"require_tool_call,omitempty"`
	FinishReason    string   `json:"finish_reason,omitempty"`
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
	Status      string       `json:"status"`
	Identity    Identity     `json:"identity"`
	Completed   bool         `json:"completed"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at"`
	Cases       []CaseResult `json:"cases"`
	FailedCases []string     `json:"failed_cases,omitempty"`
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
	if len(cases) == 0 {
		cases = DefaultCases()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Runner{store: store, client: client, cases: cases, now: now, key: opts.EncryptionKey}
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

	started := r.now().UTC()
	report := Report{
		Status:    model.LLMEvalStatusFailed,
		Identity:  candidate.Identity,
		StartedAt: started,
		Cases:     make([]CaseResult, 0, len(r.cases)),
	}
	for _, tc := range r.cases {
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

	if err := r.store.UpdateLLMEvalResult(ctx, role, report.Status, report); err != nil {
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
	candidate := Candidate{
		Identity: identity,
		APIBase:  strings.TrimSpace(*cfg.APIBase),
	}
	if key, err := r.store.GetLLMAPIKeyEnc(ctx, role); err == nil && strings.TrimSpace(key) != "" {
		dec, err := crypto.Decrypt(key, r.key)
		if err != nil {
			return Candidate{}, fmt.Errorf("decrypt eval LLM api key: %w", err)
		}
		candidate.APIKey = dec
	}
	return candidate, nil
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
	if tc.Expectations.FinishReason != "" && transcript.FinishReason != tc.Expectations.FinishReason {
		result.Passed = false
		result.Failures = append(result.Failures, fmt.Sprintf("finish_reason = %q, want %q", transcript.FinishReason, tc.Expectations.FinishReason))
	}
	return result
}

func hasToolCall(transcript Transcript, name string) bool {
	for _, call := range transcript.ToolCalls {
		if call.Name == name {
			return true
		}
	}
	return false
}

func decodeCases(raw []byte) ([]Case, error) {
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}
