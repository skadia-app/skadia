package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDefaultProfilesExposeAgentPresets(t *testing.T) {
	if fields := reflect.TypeOf(ModelProfile{}).NumField(); fields != 19 {
		t.Fatalf("ModelProfile field count = %d, want 19", fields)
	}

	tests := map[string]ModelProfile{
		"tiny": {
			Enabled: true, MaxTools: 4, UseRouter: true, MaxToolsPerTurn: 1, ToolResultLimit: 5,
			SchemaMode: "filtered", EnableSQLAgent: false, SQLComplexity: "simple", MaxSQLRetries: 0,
			FewShotCount: 3, SystemPromptMode: "compact", MaxContextTokens: 2000, HistoryMessages: 5,
			ReasoningMode: "guided", Temperature: 0.2, TopP: 0.8, TopK: 20, PresencePenalty: 0.3, MaxResponseTokens: 1000,
		},
		"small": {
			Enabled: true, MaxTools: 6, UseRouter: true, MaxToolsPerTurn: 3, ToolResultLimit: 12,
			SchemaMode: "filtered", EnableSQLAgent: true, SQLComplexity: "simple", MaxSQLRetries: 1,
			FewShotCount: 3, SystemPromptMode: "compact", MaxContextTokens: 12000, HistoryMessages: 10,
			ReasoningMode: "guided", Temperature: 0.3, TopP: 0.9, TopK: 40, PresencePenalty: 0.3, MaxResponseTokens: 3000,
		},
		"medium": {
			Enabled: true, MaxTools: 10, UseRouter: true, MaxToolsPerTurn: 2, ToolResultLimit: 15,
			SchemaMode: "filtered", EnableSQLAgent: true, SQLComplexity: "moderate", MaxSQLRetries: 2,
			FewShotCount: 2, SystemPromptMode: "standard", MaxContextTokens: 8000, HistoryMessages: 20,
			ReasoningMode: "free", Temperature: 0.5, TopP: 0.9, TopK: 40, PresencePenalty: 1.0, MaxResponseTokens: 6000,
		},
		"large": {
			Enabled: true, MaxTools: 20, UseRouter: false, MaxToolsPerTurn: 5, ToolResultLimit: 50,
			SchemaMode: "full", EnableSQLAgent: true, SQLComplexity: "advanced", MaxSQLRetries: 2,
			FewShotCount: 0, SystemPromptMode: "rich", MaxContextTokens: 16000, HistoryMessages: 30,
			ReasoningMode: "free", Temperature: 0.7, TopP: 0.95, TopK: 50, PresencePenalty: 0.5, MaxResponseTokens: 12000,
		},
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got := GetProfile(name, "", nil)
			assertProfile(t, got, want)
		})
	}
}

func TestDetectModelSizeMatchesPythonRules(t *testing.T) {
	tests := map[string]string{
		"claude-3-5-sonnet":   "large",
		"gpt-4o-mini":         "large",
		"gemini-1.5-pro":      "large",
		"command-r":           "large",
		"local mini":          "tiny",
		"llama-3.2-3b":        "tiny",
		"qwen3.5-9b":          "small",
		"mistral-22b":         "medium",
		"deepseek-r1-70b":     "large",
		"no-size-local-model": "small",
	}

	for modelName, want := range tests {
		t.Run(modelName, func(t *testing.T) {
			if got := DetectModelSize(modelName); got != want {
				t.Fatalf("DetectModelSize(%q) = %q, want %q", modelName, got, want)
			}
		})
	}
}

func TestGetProfileAutoDetectionAndOverrides(t *testing.T) {
	got := GetProfile("auto", "mistral-22b", map[string]any{
		"max_tools":            3,
		"use_router":           false,
		"temperature":          0.15,
		"system_prompt_mode":   "compact",
		"unknown_future_field": "ignored",
	})

	want := GetProfile("medium", "", nil)
	want.MaxTools = 3
	want.UseRouter = false
	want.Temperature = 0.15
	want.SystemPromptMode = "compact"
	assertProfile(t, got, want)
}

func TestLoadProfileAppliesStoredBaseAndOverrides(t *testing.T) {
	store := fakeProfileStore{value: []byte(`{
		"enabled": true,
		"base_profile": "large",
		"overrides": {
			"max_tools": 8,
			"use_router": true,
			"temperature": 0.25,
			"history_messages": 7
		}
	}`)}

	got, err := LoadProfile(context.Background(), store, "user-1", "small")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	want := GetProfile("large", "", nil)
	want.MaxTools = 8
	want.UseRouter = true
	want.Temperature = 0.25
	want.HistoryMessages = 7
	assertProfile(t, *got, want)
}

func TestLoadProfileEnforcesStoredDisabledFlag(t *testing.T) {
	store := fakeProfileStore{value: []byte(`{
		"enabled": false,
		"base_profile": "large"
	}`)}

	got, err := LoadProfile(context.Background(), store, "user-1", "small")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if got.Enabled {
		t.Fatal("LoadProfile ignored enabled=false")
	}
	want := GetProfile("large", "", nil)
	want.Enabled = false
	assertProfile(t, *got, want)
}

func TestLoadProfileFallsBackOnStoreErrorAndMalformedJSON(t *testing.T) {
	t.Run("store error", func(t *testing.T) {
		got, err := LoadProfile(context.Background(), fakeProfileStore{err: errors.New("db unavailable")}, "user-1", "tiny")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		assertProfile(t, *got, GetProfile("tiny", "", nil))
	})

	t.Run("malformed json", func(t *testing.T) {
		got, err := LoadProfile(context.Background(), fakeProfileStore{value: []byte(`{"base_profile":`)}, "user-1", "large")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		assertProfile(t, *got, GetProfile("large", "", nil))
	})
}

func assertProfile(t *testing.T, got, want ModelProfile) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile = %+v, want %+v", got, want)
	}
}

type fakeProfileStore struct {
	value []byte
	err   error
}

func (s fakeProfileStore) GetMLUserSetting(ctx context.Context, userID, key string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.value, nil
}
