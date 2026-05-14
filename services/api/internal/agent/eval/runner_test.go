package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestIdentityIncludesRoleProviderHostAndModel(t *testing.T) {
	apiBase := "https://llm.example.test:8443/v1"
	modelName := "candidate-model"
	contextWindow := 16000

	got, err := IdentityForConfig(model.LLMSystemConfig{
		Role:          model.LLMRoleToolAgent,
		BackendType:   "llama_server",
		APIBase:       &apiBase,
		ModelName:     &modelName,
		ContextWindow: &contextWindow,
	})
	if err != nil {
		t.Fatalf("IdentityForConfig: %v", err)
	}

	want := Identity{
		Role:          model.LLMRoleToolAgent,
		BackendType:   "llama_server",
		APIBaseHost:   "llm.example.test:8443",
		ModelName:     "candidate-model",
		ContextWindow: contextWindow,
		SuiteID:       model.LLMEvalSuiteID,
		SuiteHash:     model.LLMEvalSuiteHash,
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
		CategoryDocumentExtract,
		CategoryMarkdownNoLoop,
		CategoryVision,
		CategoryWritePreview,
		CategoryGermanFactual,
		CategoryStreamingToolDelta,
	} {
		if !got[category] {
			t.Fatalf("DefaultCases missing category %q; got %#v", category, got)
		}
	}
}

func TestDefaultSuiteHashMatchesModelGateConstant(t *testing.T) {
	if got := DefaultSuiteHash(); got != model.LLMEvalSuiteHash {
		t.Fatalf("DefaultSuiteHash = %q, want model gate hash %q", got, model.LLMEvalSuiteHash)
	}
}

func TestDefaultToolCallCasesReferenceRegisteredRuntimeTools(t *testing.T) {
	registered := registeredRuntimeToolNamesFromSource(t)
	for _, tc := range DefaultCases() {
		name := tc.Expectations.RequireToolCall
		if name == "" {
			continue
		}
		if !registered[name] {
			t.Fatalf("eval case %q requires unregistered runtime tool %q", tc.ID, name)
		}
	}
}

func registeredRuntimeToolNamesFromSource(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir("../tools")
	if err != nil {
		t.Fatalf("read tools directory: %v", err)
	}
	names := map[string]bool{"run_sql": true}
	re := regexp.MustCompile(`Name:\s*"([a-z0-9_]+)"`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile("../tools/" + entry.Name())
		if err != nil {
			t.Fatalf("read tool source %s: %v", entry.Name(), err)
		}
		for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
			names[match[1]] = true
		}
	}
	return names
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
		"sql": {Content: "SELECT count(*) FROM activities", FinishReason: "stop"},
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
	if store.lastReport.SuiteID == "" || store.lastReport.SuiteHash == "" {
		t.Fatalf("stored report missing suite identity: %#v", store.lastReport)
	}
}

func TestRunnerScopesCasesByRoleAndCapabilities(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	store := &fakeStore{system: []model.LLMSystemConfig{{
		Role:            model.LLMRoleMarkdownExtract,
		BackendType:     "llama_server",
		APIBase:         &apiBase,
		ModelName:       &modelName,
		Enabled:         true,
		CapabilityFlags: []string{"markdown_extract", "table_preservation"},
		EvalStatus:      model.LLMEvalStatusUntested,
	}}}
	client := &fakeClient{responses: map[string]Transcript{
		"markdown": {Content: "| A | B |\n| - | - |\n| 1 | 2 |", FinishReason: "stop"},
	}}

	report, err := NewRunner(store, client, RunnerOptions{Cases: []Case{
		{
			ID:           "sql",
			Category:     CategorySQLReadonly,
			Roles:        []string{model.LLMRoleToolAgent},
			Expectations: Expectations{MustContain: []string{"select"}},
		},
		{
			ID:                   "markdown",
			Category:             CategoryMarkdownNoLoop,
			Roles:                []string{model.LLMRoleMarkdownExtract},
			RequiredCapabilities: []string{"markdown_extract"},
			Expectations:         Expectations{MustContain: []string{"|"}, FinishReason: "stop"},
		},
	}}).Run(context.Background(), model.LLMRoleMarkdownExtract)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.calledCases) != 1 || client.calledCases[0] != "markdown" {
		t.Fatalf("called cases = %#v, want only markdown case", client.calledCases)
	}
	if len(report.Cases) != 1 || report.Cases[0].ID != "markdown" {
		t.Fatalf("report cases = %#v, want only markdown case", report.Cases)
	}
}

func TestValidateCaseRejectsSQLWithoutTenantGuard(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			MustContain:           []string{"select"},
			RequireSQLTenantGuard: true,
		},
	}, Transcript{Content: "SELECT count(*) FROM activities", FinishReason: "stop"})

	if result.Passed {
		t.Fatal("validateCase accepted SQL without a user_id/current_setting tenant guard")
	}
}

func TestValidateCaseRejectsLiteralUserIDTenantGuard(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			RequireSQLTenantGuard: true,
		},
	}, Transcript{Content: "SELECT count(*) FROM activities WHERE user_id = '00000000-0000-0000-0000-000000000000'", FinishReason: "stop"})

	if result.Passed {
		t.Fatal("validateCase accepted a literal user_id predicate as tenant-safe")
	}
}

func TestValidateCaseRejectsSQLRejectedByRuntimeValidator(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			MustContain: []string{"select"},
		},
	}, Transcript{Content: "SELECT current_setting('app.current_user_id')", FinishReason: "stop"})

	if result.Passed {
		t.Fatal("validateCase accepted SQL that runtime ValidateSQL rejects")
	}
}

func TestValidateCaseRejectsProseWrappedSQLWhenSQLOutputRequired(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			MustContain: []string{"select", "activities"},
		},
	}, Transcript{Content: "Here is the query:\nSELECT count(*) FROM activities", FinishReason: "stop"})

	if result.Passed {
		t.Fatal("validateCase accepted prose-wrapped SQL output")
	}
}

func TestValidateCaseAcceptsSingleFencedSQLBlockWhenSQLOutputRequired(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			MustContain: []string{"select", "activities"},
		},
	}, Transcript{Content: "```sql\nSELECT count(*) FROM activities\n```", FinishReason: "stop"})

	if !result.Passed {
		t.Fatalf("validateCase rejected single fenced SQL block: %#v", result.Failures)
	}
}

func TestValidateCaseAllowsReadonlyRefusalWithoutSQL(t *testing.T) {
	result := validateCase(Case{
		ID:       "sql-refusal",
		Category: CategorySQLReadonly,
		Expectations: Expectations{
			MinContentChars: 10,
		},
	}, Transcript{Content: "Das kann ich nicht als reine Leseabfrage ausfuehren.", FinishReason: "stop"})

	if !result.Passed {
		t.Fatalf("validateCase rejected non-SQL refusal: %#v", result.Failures)
	}
}

func TestValidateCaseRejectsMalformedToolArguments(t *testing.T) {
	result := validateCase(Case{
		ID:       "tool",
		Category: CategoryToolCall,
		Expectations: Expectations{
			RequireToolCall: "get_activity_summary",
			ToolArgsSchema:  []byte(`{"type":"object","required":["activity_id"],"additionalProperties":false,"properties":{"activity_id":{"type":"string","minLength":36}}}`),
		},
	}, Transcript{ToolCalls: []ToolCall{{Name: "get_activity_summary", Arguments: `{"activity_id":7}`}}, FinishReason: "tool_calls"})

	if result.Passed {
		t.Fatal("validateCase accepted tool arguments that do not satisfy the schema")
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

func TestRunnerDoesNotDemoteCurrentAdminReleaseOnFailedQualityCheck(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	releaseReport := evalIdentityReport(model.LLMRoleToolAgent, "llama_server", "llm.example.test", modelName)
	releaseReport["admin_reason"] = "Admin configured local model"
	store := &fakeStore{system: []model.LLMSystemConfig{{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusWaived,
		EvalReport:  releaseReport,
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
		t.Fatalf("returned status = %q, want diagnostic failure", report.Status)
	}
	if store.lastStatus != model.LLMEvalStatusWaived {
		t.Fatalf("stored status = %q, want preserved admin release", store.lastStatus)
	}
	if got, _ := store.lastReportAny.(map[string]any)["quality_status"].(string); got != model.LLMEvalStatusFailed {
		t.Fatalf("stored quality_status = %q, want failed in diagnostic report %#v", got, store.lastReportAny)
	}
	if got, _ := store.lastReportAny.(map[string]any)["admin_reason"].(string); got != "Admin configured local model" {
		t.Fatalf("stored admin_reason = %q, want preserved reason", got)
	}
}

func evalIdentityReport(role, backendType, apiBaseHost, modelName string) map[string]any {
	return map[string]any{"identity": map[string]any{
		"role":          role,
		"backend_type":  backendType,
		"api_base_host": apiBaseHost,
		"model_name":    modelName,
		"suite_id":      model.LLMEvalSuiteID,
		"suite_hash":    model.LLMEvalSuiteHash,
	}}
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
			HasAPIKey:   true,
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

func TestRunnerFailsClosedWhenMarkedAPIKeyCannotBeLoaded(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "candidate-model"
	store := &fakeStore{
		system: []model.LLMSystemConfig{{
			Role:        model.LLMRoleToolAgent,
			BackendType: "openai",
			APIBase:     &apiBase,
			ModelName:   &modelName,
			HasAPIKey:   true,
			Enabled:     true,
			EvalStatus:  model.LLMEvalStatusUntested,
		}},
	}
	client := &fakeClient{responses: map[string]Transcript{
		"de": {Content: "Antwort.", FinishReason: "stop"},
	}}

	_, err := NewRunner(store, client, RunnerOptions{Cases: []Case{{
		ID:           "de",
		Category:     CategoryGermanFactual,
		Expectations: Expectations{MustContain: []string{"Antwort"}},
	}}}).Run(context.Background(), model.LLMRoleToolAgent)
	if err == nil || !strings.Contains(err.Error(), "get eval LLM api key") {
		t.Fatalf("Run error = %v, want fail-closed key error", err)
	}
	if len(client.called) != 0 {
		t.Fatalf("client called despite key lookup failure: %#v", client.called)
	}
}

func TestRunnerPersistsModelOverrideEvalSeparately(t *testing.T) {
	apiBase := "http://llm.example.test/v1"
	modelName := "system-model"
	store := &fakeStore{system: []model.LLMSystemConfig{{
		Role:        model.LLMRoleToolAgent,
		BackendType: "llama_server",
		APIBase:     &apiBase,
		ModelName:   &modelName,
		Enabled:     true,
		EvalStatus:  model.LLMEvalStatusPassed,
	}}}
	client := &fakeClient{responses: map[string]Transcript{
		"de": {Content: "Antwort.", FinishReason: "stop"},
	}}

	report, err := NewRunner(store, client, RunnerOptions{
		ModelNameOverride: "user-model",
		Cases: []Case{{
			ID:           "de",
			Category:     CategoryGermanFactual,
			Expectations: Expectations{MustContain: []string{"Antwort"}},
		}},
	}).Run(context.Background(), model.LLMRoleToolAgent)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if store.lastRole != "" || store.lastModelEvalRole != model.LLMRoleToolAgent || store.lastModelEvalReport.Identity.ModelName != "user-model" {
		t.Fatalf("store state = lastRole:%q modelEval:%#v, want only model eval for user-model", store.lastRole, store.lastModelEvalReport)
	}
	if report.Identity.ModelName != "user-model" || len(client.called) != 1 || client.called[0].ModelName != "user-model" {
		t.Fatalf("report/client model = %#v / %#v, want override model", report.Identity, client.called)
	}
}

func TestWireClientFailsOnProviderStreamErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"quota exceeded\",\"type\":\"rate_limit\",\"code\":\"429\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	_, err := (WireClient{}).RunCase(context.Background(), Candidate{
		Identity: Identity{ModelName: "candidate-model"},
		APIBase:  server.URL,
	}, Case{
		ID:       "de",
		Category: CategoryGermanFactual,
		Prompt:   "Antworte kurz.",
	})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("RunCase error = %v, want provider stream error", err)
	}
}

func TestWireClientSendsVisionFixtureAsImageContent(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"{}\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	_, err := (WireClient{HTTPClient: server.Client()}).RunCase(context.Background(), Candidate{
		Identity: Identity{ModelName: "candidate-model"},
		APIBase:  server.URL,
	}, Case{
		ID:           "vision",
		Category:     CategoryVision,
		Prompt:       "Describe visible objects.",
		ImageDataURL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
	})
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	messages := gotBody["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "text" || content[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("vision content = %#v, want text plus image_url parts", content)
	}
}

func TestWireClientRejectsOutOfBoundsToolCallIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":-1,\"function\":{\"name\":\"search_activities\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n")
	}))
	t.Cleanup(server.Close)

	_, err := (WireClient{HTTPClient: server.Client()}).RunCase(context.Background(), Candidate{
		Identity: Identity{ModelName: "candidate-model"},
		APIBase:  server.URL,
	}, Case{
		ID:       "tool",
		Category: CategoryToolCall,
		Prompt:   "Call a tool.",
	})
	if err == nil || !strings.Contains(err.Error(), "outside eval bounds") {
		t.Fatalf("RunCase error = %v, want out-of-bounds index error", err)
	}
}

func TestDecodeCasesRejectsDuplicateIDs(t *testing.T) {
	_, err := decodeCases([]byte(`[
		{"id":"dup","name":"one","category":"german_factual","prompt":"p","expectations":{"min_content_chars":1}},
		{"id":"dup","name":"two","category":"german_factual","prompt":"p","expectations":{"min_content_chars":1}}
	]`))
	if err == nil || !strings.Contains(err.Error(), "duplicate eval case id") {
		t.Fatalf("decodeCases error = %v, want duplicate id validation error", err)
	}
}

func TestDecodeCasesRejectsUnknownRoleCapabilityCategoryAndInvalidFixture(t *testing.T) {
	for name, raw := range map[string]string{
		"category":    `[{"id":"bad_category","name":"bad","category":"unknown","prompt":"p","expectations":{"min_content_chars":1}}]`,
		"role":        `[{"id":"bad_role","name":"bad","category":"german_factual","roles":["default"],"prompt":"p","expectations":{"min_content_chars":1}}]`,
		"capability":  `[{"id":"bad_capability","name":"bad","category":"german_factual","required_capabilities":["unknown"],"prompt":"p","expectations":{"min_content_chars":1}}]`,
		"tool_schema": `[{"id":"bad_schema","name":"bad","category":"tool_call","prompt":"p","expectations":{"require_tool_call":"search_activities","tool_args_schema":{"type":"not-a-json-schema-type"}}}]`,
		"image":       `[{"id":"bad_image","name":"bad","category":"vision","prompt":"p","image_data_url":"data:image/png;base64,iVBORw0KGgo=","expectations":{"require_json_output":true}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCases([]byte(raw)); err == nil {
				t.Fatal("decodeCases returned nil error, want validation failure")
			}
		})
	}
}

var testEvalEncryptionKey = strings.Repeat("0123456789abcdef", 4)

type fakeStore struct {
	system              []model.LLMSystemConfig
	keys                map[string]string
	lastRole            string
	lastStatus          string
	lastReport          Report
	lastReportAny       any
	lastModelEvalRole   string
	lastModelEvalReport Report
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
	s.lastReportAny = report
	switch value := report.(type) {
	case Report:
		s.lastReport = value
	case map[string]any:
		raw, _ := json.Marshal(value)
		_ = json.Unmarshal(raw, &s.lastReport)
	default:
		panic(fmt.Sprintf("unexpected report type %T", report))
	}
	return nil
}

func (s *fakeStore) UpdateLLMModelEvalResult(_ context.Context, role string, report any) error {
	s.lastModelEvalRole = role
	s.lastModelEvalReport = report.(Report)
	return nil
}

type fakeClient struct {
	responses   map[string]Transcript
	called      []Candidate
	calledCases []string
}

func (c *fakeClient) RunCase(_ context.Context, candidate Candidate, tc Case) (Transcript, error) {
	c.called = append(c.called, candidate)
	c.calledCases = append(c.calledCases, tc.ID)
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
