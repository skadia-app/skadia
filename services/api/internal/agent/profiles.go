package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
)

type ModelProfile struct {
	Enabled           bool    `json:"enabled"`
	MaxTools          int     `json:"max_tools"`
	UseRouter         bool    `json:"use_router"`
	MaxToolsPerTurn   int     `json:"max_tools_per_turn"`
	ToolResultLimit   int     `json:"tool_result_limit"`
	SchemaMode        string  `json:"schema_mode"`
	EnableSQLAgent    bool    `json:"enable_sql_agent"`
	SQLComplexity     string  `json:"sql_complexity"`
	MaxSQLRetries     int     `json:"max_sql_retries"`
	FewShotCount      int     `json:"few_shot_count"`
	SystemPromptMode  string  `json:"system_prompt_mode"`
	MaxContextTokens  int     `json:"max_context_tokens"`
	HistoryMessages   int     `json:"history_messages"`
	ReasoningMode     string  `json:"reasoning_mode"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	PresencePenalty   float64 `json:"presence_penalty"`
	MaxResponseTokens int     `json:"max_response_tokens"`
}

var defaultProfiles = map[string]ModelProfile{
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

var (
	modelSizeKeywordRE = regexp.MustCompile(`\b(mini|nano|micro|tiny)\b`)
	modelSizeParamRE   = regexp.MustCompile(`(?i)(^|[^.\d])(\d+)b($|[^\w])`)
)

type ProfileStore interface {
	GetMLUserSetting(ctx context.Context, userID, key string) ([]byte, error)
}

type ProfileDefaults struct {
	DefaultProfile string                  `json:"default_profile"`
	Presets        map[string]ModelProfile `json:"presets"`
}

func ChatProfileDefaults(defaultProfile string) ProfileDefaults {
	if defaultProfile == "" {
		defaultProfile = "small"
	}
	return ProfileDefaults{
		DefaultProfile: defaultProfile,
		Presets:        maps.Clone(defaultProfiles),
	}
}

func DetectModelSize(modelName string) string {
	name := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(modelName), "-", ""), "_", "")
	rawName := strings.ToLower(modelName)

	for _, apiName := range []string{"claude", "gpt-4", "gpt-4o", "gemini", "command-r"} {
		if strings.Contains(name, strings.ReplaceAll(apiName, "-", "")) {
			return "large"
		}
	}

	if modelSizeKeywordRE.MatchString(rawName) {
		return "tiny"
	}

	match := modelSizeParamRE.FindStringSubmatch(rawName)
	if len(match) == 4 {
		params, err := strconv.Atoi(match[2])
		if err == nil {
			if params <= 4 {
				return "tiny"
			}
			if params <= 12 {
				return "small"
			}
			if params <= 40 {
				return "medium"
			}
			return "large"
		}
	}

	return "small"
}

func GetProfile(profileName, modelName string, overrides map[string]any) ModelProfile {
	if profileName == "auto" {
		if modelName == "" {
			profileName = "small"
		} else {
			profileName = DetectModelSize(modelName)
		}
	}

	profile, ok := defaultProfiles[profileName]
	if !ok {
		profile = defaultProfiles["small"]
	}

	applyProfileOverrides(&profile, overrides)
	return profile
}

func LoadProfile(ctx context.Context, store ProfileStore, userID, defaultProfile string) (*ModelProfile, error) {
	fallback := GetProfile(defaultProfile, "", nil)
	if store == nil {
		return &fallback, nil
	}

	valueJSON, err := store.GetMLUserSetting(ctx, userID, "chat_profile")
	if err != nil || len(valueJSON) == 0 {
		return &fallback, nil
	}

	var setting struct {
		Enabled     *bool          `json:"enabled"`
		BaseProfile string         `json:"base_profile"`
		Overrides   map[string]any `json:"overrides"`
	}
	if err := json.Unmarshal(valueJSON, &setting); err != nil {
		return &fallback, nil
	}

	baseProfile := setting.BaseProfile
	if baseProfile == "" {
		baseProfile = defaultProfile
	}
	profile := GetProfile(baseProfile, "", setting.Overrides)
	if setting.Enabled != nil {
		profile.Enabled = *setting.Enabled
	}
	return &profile, nil
}

func (p ModelProfile) DisabledError() error {
	if p.Enabled {
		return nil
	}
	return fmt.Errorf("chat profile is disabled")
}

func applyProfileOverrides(profile *ModelProfile, overrides map[string]any) {
	if len(overrides) == 0 {
		return
	}
	for key, value := range overrides {
		switch key {
		case "max_tools":
			setInt(value, &profile.MaxTools)
		case "use_router":
			setBool(value, &profile.UseRouter)
		case "max_tools_per_turn":
			setInt(value, &profile.MaxToolsPerTurn)
		case "tool_result_limit":
			setInt(value, &profile.ToolResultLimit)
		case "schema_mode":
			setString(value, &profile.SchemaMode)
		case "enable_sql_agent":
			setBool(value, &profile.EnableSQLAgent)
		case "sql_complexity":
			setString(value, &profile.SQLComplexity)
		case "max_sql_retries":
			setInt(value, &profile.MaxSQLRetries)
		case "few_shot_count":
			setInt(value, &profile.FewShotCount)
		case "system_prompt_mode":
			setString(value, &profile.SystemPromptMode)
		case "max_context_tokens":
			setInt(value, &profile.MaxContextTokens)
		case "history_messages":
			setInt(value, &profile.HistoryMessages)
		case "reasoning_mode":
			setString(value, &profile.ReasoningMode)
		case "temperature":
			setFloat(value, &profile.Temperature)
		case "top_p":
			setFloat(value, &profile.TopP)
		case "top_k":
			setInt(value, &profile.TopK)
		case "presence_penalty":
			setFloat(value, &profile.PresencePenalty)
		case "max_response_tokens":
			setInt(value, &profile.MaxResponseTokens)
		}
	}
}

func setInt(value any, dst *int) {
	switch v := value.(type) {
	case int:
		*dst = v
	case int32:
		*dst = int(v)
	case int64:
		*dst = int(v)
	case float64:
		*dst = int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			*dst = int(n)
		}
	}
}

func setBool(value any, dst *bool) {
	if v, ok := value.(bool); ok {
		*dst = v
	}
}

func setString(value any, dst *string) {
	if v, ok := value.(string); ok {
		*dst = v
	}
}

func setFloat(value any, dst *float64) {
	switch v := value.(type) {
	case float32:
		*dst = float64(v)
	case float64:
		*dst = v
	case json.Number:
		if n, err := v.Float64(); err == nil {
			*dst = n
		}
	}
}
