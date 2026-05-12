package eval

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestIdentityIncludesRoleProviderHostAndModel(t *testing.T) {
	apiBase := "https://llm.example.test:8443/v1"
	modelName := "candidate-model"

	got, err := IdentityForConfig(model.LLMSystemConfig{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
	})
	if err != nil {
		t.Fatalf("IdentityForConfig: %v", err)
	}

	want := Identity{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBaseHost: "llm.example.test:8443",
		ModelName:   "candidate-model",
	}
	if got != want {
		t.Fatalf("identity = %#v, want %#v", got, want)
	}
}

func TestDefaultCasesCoverRequiredEvalSurfaces(t *testing.T) {
	got := map[string]bool{}
	for _, tc := range DefaultCases() {
		got[tc.Category] = true
	}
	for _, category := range []string{
		CategorySQLReadonly,
		CategoryToolCall,
		CategoryMarkdownNoLoop,
		CategoryWritePreview,
		CategoryGermanFactual,
		CategoryStreamingToolDelta,
	} {
		if !got[category] {
			t.Fatalf("DefaultCases missing category %q; got %#v", category, got)
		}
	}
}

func TestRunnerStoresPassedOnlyAfterAllCasesComplete(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	store := &fakeStore{system: []model.LLMSystemConfig{{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusUntested,
	}}}
	client := &fakeClient{responses: map[string]Transcript{
		"sql": {Content: "SELECT count(*) FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid", FinishReason: "stop"},
		"de":  {Content: "Du hast 3 Aktivitaeten.", FinishReason: "stop"},
	}}

	report, err := NewRunner(store, client, RunnerOptions{Cases: []Case{
		{ID: "sql", Category: CategorySQLReadonly, Expectations: Expectations{MustContain: []string{"SELECT"}, MustNotContain: []string{"UPDATE", "; DROP"}}},
		{ID: "de", Category: CategoryGermanFactual, Expectations: Expectations{MustContain: []string{"3 Aktivitaeten"}, MustNotContain: []string{"vielleicht"}}},
	}}).Run(context.Background(), model.LLMRoleToolAgent)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Status != model.LLMEvalStatusPassed {
		t.Fatalf("report status = %q, want passed", report.Status)
	}
	if store.lastStatus != model.LLMEvalStatusPassed {
		t.Fatalf("stored status = %q, want passed", store.lastStatus)
	}
	if len(store.lastReport.Cases) != 2 {
		t.Fatalf("stored cases = %d, want 2", len(store.lastReport.Cases))
	}
	if len(client.called) != 2 {
		t.Fatalf("client calls = %d, want 2", len(client.called))
	}
}

func TestRunnerStoresFailedCasesAndDoesNotFamilyAutoPass(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "qwen3.6-newer-family"
	store := &fakeStore{system: []model.LLMSystemConfig{{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusFailed,
	}}}
	client := &fakeClient{responses: map[string]Transcript{
		"tool": {Content: "I cannot call tools, but here is text.", FinishReason: "stop"},
	}}

	report, err := NewRunner(store, client, RunnerOptions{Cases: []Case{{
		ID:           "tool",
		Category:     CategoryToolCall,
		Expectations: Expectations{RequireToolCall: "search_activities", MustNotContain: []string{"cannot call tools"}},
	}}}).Run(context.Background(), model.LLMRoleToolAgent)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Status != model.LLMEvalStatusFailed {
		t.Fatalf("report status = %q, want failed", report.Status)
	}
	if store.lastStatus != model.LLMEvalStatusFailed {
		t.Fatalf("stored status = %q, want failed", store.lastStatus)
	}
	if got, want := failedCaseIDs(store.lastReport), []string{"tool"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed cases = %#v, want %#v", got, want)
	}
}

func TestRunnerIsOnlyPathAllowedToExecuteUntestedCandidate(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	cfg := model.LLMSystemConfig{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusUntested,
	}

	normal := agent.NewLLMResolver(fakeResolverStore{system: []model.LLMSystemConfig{cfg}}, agent.LLMResolverConfig{})
	if _, err := normal.Resolve(context.Background(), "user-1", model.LLMRoleToolAgent); err == nil {
		t.Fatal("normal resolver allowed untested role, want eval gate error")
	}

	store := &fakeStore{system: []model.LLMSystemConfig{cfg}}
	client := &fakeClient{responses: map[string]Transcript{
		"de": {Content: "Antwort aus dem Eval.", FinishReason: "stop"},
	}}
	_, err := NewRunner(store, client, RunnerOptions{Cases: []Case{{
		ID:           "de",
		Category:     CategoryGermanFactual,
		Expectations: Expectations{MustContain: []string{"Antwort"}},
	}}}).Run(context.Background(), model.LLMRoleToolAgent)
	if err != nil {
		t.Fatalf("eval runner rejected untested role: %v", err)
	}
	if len(client.called) != 1 {
		t.Fatalf("client calls = %d, want 1", len(client.called))
	}
}

func TestRunnerDecryptsConfiguredAPIKeyForEvalClient(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	enc, err := crypto.Encrypt("plain-api-key", testEvalEncryptionKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	store := &fakeStore{
		system: []model.LLMSystemConfig{{
			Role:        model.LLMRoleToolAgent,
			BackendType: "openai",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusUntested,
		}},
		keys: map[string]string{model.LLMRoleToolAgent: enc},
	}
	client := &fakeClient{responses: map[string]Transcript{
		"de": {Content: "Antwort.", FinishReason: "stop"},
	}}

	_, err = NewRunner(store, client, RunnerOptions{
		EncryptionKey: testEvalEncryptionKey,
		Cases: []Case{{
			ID:           "de",
			Category:     CategoryGermanFactual,
			Expectations: Expectations{MustContain: []string{"Antwort"}},
		}},
	}).Run(context.Background(), model.LLMRoleToolAgent)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(client.called) != 1 || client.called[0].APIKey != "plain-api-key" {
		t.Fatalf("candidate API key = %#v, want decrypted key", client.called)
	}
}

var testEvalEncryptionKey = strings.Repeat("0123456789abcdef", 4)

type fakeStore struct {
	system     []model.LLMSystemConfig
	keys       map[string]string
	lastRole   string
	lastStatus string
	lastReport Report
}

func (s *fakeStore) GetLLMSystemConfig(context.Context) ([]model.LLMSystemConfig, error) {
	return s.system, nil
}

func (s *fakeStore) GetLLMAPIKeyEnc(_ context.Context, role string) (string, error) {
	if s.keys != nil {
		return s.keys[role], nil
	}
	return "", errors.New("not found")
}

func (s *fakeStore) UpdateLLMEvalResult(_ context.Context, role, status string, report any) error {
	s.lastRole = role
	s.lastStatus = status
	s.lastReport = report.(Report)
	return nil
}

type fakeClient struct {
	responses map[string]Transcript
	called    []Candidate
}

func (c *fakeClient) RunCase(_ context.Context, candidate Candidate, tc Case) (Transcript, error) {
	c.called = append(c.called, candidate)
	if resp, ok := c.responses[tc.ID]; ok {
		return resp, nil
	}
	return Transcript{}, errors.New("missing fake response")
}

type fakeResolverStore struct {
	system []model.LLMSystemConfig
}

func (s fakeResolverStore) GetLLMSystemConfig(context.Context) ([]model.LLMSystemConfig, error) {
	return s.system, nil
}

func (fakeResolverStore) GetLLMAPIKeyEnc(context.Context, string) (string, error) {
	return "", errors.New("not found")
}

func (fakeResolverStore) GetLLMUserConfig(context.Context, string) ([]model.LLMUserConfig, error) {
	return nil, nil
}

func failedCaseIDs(report Report) []string {
	var ids []string
	for _, result := range report.Cases {
		if !result.Passed {
			ids = append(ids, result.ID)
		}
	}
	return ids
}
