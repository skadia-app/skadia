package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"github.com/openai/openai-go/shared/constant"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Permission string

const (
	PermissionRead             Permission = "read"
	PermissionWriteSafe        Permission = "write_safe"
	PermissionWriteDestructive Permission = "write_destructive"
)

type AuditMode string

const (
	AuditNone    AuditMode = "none"
	AuditPreview AuditMode = "preview"
	AuditWrite   AuditMode = "write"
)

var SportTypes = []string{
	"running", "walking", "hiking", "mountaineering", "snowshoeing",
	"floor_climbing", "wheelchair_push",
	"cycling", "inline_skating", "alpine_skiing", "backcountry_skiing",
	"cross_country_skiing", "snowboarding", "ice_skating", "snowmobiling",
	"sledding", "swimming", "paddling", "kayaking", "stand_up_paddleboarding",
	"rowing", "surfing", "kitesurfing", "windsurfing", "wakeboarding",
	"wakesurfing", "water_skiing", "water_tubing", "diving", "snorkeling",
	"boating", "sailing", "fishing", "rafting", "training",
	"fitness_equipment", "strength_training", "hiit", "dance", "jump_rope",
	"meditation", "yoga", "pilates", "soccer", "basketball",
	"american_football", "rugby", "lacrosse", "hockey", "volleyball",
	"tennis", "racket", "cricket", "baseball", "golf", "disc_golf",
	"rock_climbing", "flying", "hang_gliding", "sky_diving", "jumpmaster",
	"horseback_riding", "hunting", "motorcycling", "driving", "boxing",
	"mixed_martial_arts", "tactical", "multisport", "transition",
	"archery", "geocaching", "mobility", "shooting", "skateboarding",
	"generic", "unknown", "skiing", "canoeing",
}

var ModifierFilterParams = map[string]map[string]string{
	"is_electric": {
		"type":        "boolean",
		"description": "Filter E-Bike/electric activities: true = only electric, false = only non-electric",
	},
	"is_indoor": {
		"type":        "boolean",
		"description": "Filter indoor activities: true = only indoor, false = only outdoor/not indoor",
	},
	"is_virtual": {
		"type":        "boolean",
		"description": "Filter virtual/simulated activities: true = only virtual, false = only non-virtual",
	},
	"is_commute": {
		"type":        "boolean",
		"description": "Filter commute activities: true = only commute, false = only non-commute",
	},
}

type Tool struct {
	Name           string
	Description    string
	InputSchema    json.RawMessage
	ExecuteRead    func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error)
	ExecutePreview func(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error)
	ExecuteWrite   func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error)

	Permission           Permission
	RequiresConfirmation bool
	AuditMode            AuditMode
	SummaryForUser       func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error)
	RequiresAdmin        bool
	RateLimit            int
	IsUserOwned          bool
}

type WriteExecutionDeps struct {
	*Deps
	Tx pgx.Tx
}

type PreviewExecutionDeps struct {
	Readonly *pgxpool.Pool
	SQLAgent *agent.SQLAgent
}

type Deps struct {
	DB          *database.DB
	Readonly    *pgxpool.Pool
	MLBaseURL   string
	BRouterURL  string
	LLM         *agent.LLMResolver
	SQLAgent    *agent.SQLAgent
	Preferences *agent.PreferenceLoader
}

type Registry struct {
	tools map[string]*Tool
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

func (r *Registry) Register(t *Tool) {
	if r.tools == nil {
		r.tools = make(map[string]*Tool)
	}
	if t == nil {
		panic("agent tool registry: cannot register nil tool")
	}
	if t.Name == "" {
		panic("agent tool registry: cannot register unnamed tool")
	}
	r.tools[t.Name] = t
}

func (r *Registry) ExecuteRead(ctx context.Context, deps *Deps, userID, toolName string, args json.RawMessage) (any, error) {
	t, ok := r.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	if t.Permission != PermissionRead {
		return nil, fmt.Errorf("tool is not read-only: %s", toolName)
	}
	if t.ExecuteRead == nil {
		return nil, fmt.Errorf("tool has no read executor: %s", toolName)
	}
	return t.ExecuteRead(ctx, deps, userID, args)
}

func (r *Registry) AddEnabledWriteTools(filter []string, prefs *agent.ChatPreferences) []string {
	out := append([]string(nil), filter...)
	if prefs == nil || !prefs.ChatWriteToolsEnabled {
		return out
	}

	present := make(map[string]bool, len(out))
	for _, name := range out {
		present[name] = true
	}
	allowed := allowedWriteToolSet(prefs)
	names := r.sortedNames()
	for _, name := range names {
		t := r.tools[name]
		if t == nil || t.Permission == PermissionRead || !allowed[name] || present[name] {
			continue
		}
		out = append(out, name)
		present[name] = true
	}
	return out
}

func (r *Registry) Definitions(filter []string, prefs *agent.ChatPreferences) []openai.ChatCompletionToolParam {
	filterSet := namesSet(filter)
	allowedWrites := allowedWriteToolSet(prefs)
	defs := make([]openai.ChatCompletionToolParam, 0, len(r.tools))
	for _, name := range r.sortedNames() {
		t := r.tools[name]
		if t == nil || !nameAllowed(filterSet, name) {
			continue
		}
		if t.Permission != PermissionRead {
			if prefs == nil || !prefs.ChatWriteToolsEnabled || !allowedWrites[name] {
				continue
			}
		}
		defs = append(defs, t.definition())
	}
	return defs
}

func (r *Registry) MCPDefinitions(filter []string) []MCPTool {
	filterSet := namesSet(filter)
	defs := make([]MCPTool, 0, len(r.tools))
	for _, name := range r.sortedNames() {
		t := r.tools[name]
		if t == nil || !nameAllowed(filterSet, name) {
			continue
		}
		defs = append(defs, MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: append(json.RawMessage(nil), t.InputSchema...),
		})
	}
	return defs
}

func (r *Registry) ValidateSchemas() error {
	for _, name := range r.sortedNames() {
		if err := validateSchema(name, r.tools[name].InputSchema); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) sortedNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (t *Tool) definition() openai.ChatCompletionToolParam {
	params, err := functionParameters(t.Name, t.InputSchema)
	if err != nil {
		panic(err)
	}
	return openai.ChatCompletionToolParam{
		Type: constant.Function("function"),
		Function: shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  params,
		},
	}
}

func functionParameters(toolName string, schema json.RawMessage) (shared.FunctionParameters, error) {
	if err := validateSchema(toolName, schema); err != nil {
		return nil, err
	}
	var params map[string]any
	if err := json.Unmarshal(schema, &params); err != nil {
		return nil, fmt.Errorf("parse input schema for %s: %w", toolName, err)
	}
	return shared.FunctionParameters(params), nil
}

func validateSchema(toolName string, schema json.RawMessage) error {
	if len(bytes.TrimSpace(schema)) == 0 {
		return fmt.Errorf("input schema for %s is empty", toolName)
	}
	var doc any
	if err := json.Unmarshal(schema, &doc); err != nil {
		return fmt.Errorf("parse input schema for %s: %w", toolName, err)
	}
	compiler := jsonschema.NewCompiler()
	url := fmt.Sprintf("agent/tools/%s.schema.json", toolName)
	if err := compiler.AddResource(url, doc); err != nil {
		return fmt.Errorf("add input schema for %s: %w", toolName, err)
	}
	if _, err := compiler.Compile(url); err != nil {
		return fmt.Errorf("compile input schema for %s: %w", toolName, err)
	}
	return nil
}

func namesSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func nameAllowed(set map[string]bool, name string) bool {
	return set == nil || set[name]
}

func allowedWriteToolSet(prefs *agent.ChatPreferences) map[string]bool {
	allowed := make(map[string]bool)
	if prefs == nil {
		return allowed
	}
	for _, name := range prefs.ChatAllowedWriteTools {
		allowed[name] = true
	}
	return allowed
}
