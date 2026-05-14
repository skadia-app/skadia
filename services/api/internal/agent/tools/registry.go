package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/geosearch"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrWriteToolRequiresEnvelope = errors.New("write tool requires execution envelope")

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
	AuditRead    AuditMode = "read"
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
	PrepareWrite   func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (*PreparedWrite, error)
	ExecuteWrite   func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error)

	Permission           Permission
	RequiresConfirmation bool
	AuditMode            AuditMode
	SummaryForUser       func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (string, error)
	RequiresAdmin        bool
	// RateLimitPerHour applies per user and tool in the agent runtime.
	RateLimitPerHour int
	// Timeout bounds read-tool execution in the agent runtime.
	Timeout      time.Duration
	ExternalRead *ExternalReadRequirement
	IsUserOwned  bool
}

type PreparedWrite struct {
	Summary string
	Preview any
	Args    json.RawMessage
}

type WriteExecutionDeps struct {
	*Deps
	Tx          pgx.Tx
	AfterCommit *[]func(context.Context)
}

type PreviewExecutionDeps struct {
	Readonly *pgxpool.Pool
	SQLAgent *agent.SQLAgent
}

type Deps struct {
	DB                 *database.DB
	Readonly           *pgxpool.Pool
	MLBaseURL          string
	BRouterURL         string
	BRouterSegmentsDir string
	DEMDataDir         string
	RoutingGraphPath   string
	RoutingTilesDir    string
	CoverageBaseDir    string
	GeoSearchExternal  geosearch.ForwardSearcher
	GeoSearchPolicy    geosearch.ExternalPolicy
	GeoSearchCache     *geosearch.Cache
	GeoSearchQuota     *geosearch.Limiter
	RouteGoalSuggester RouteGoalSuggester
	InternalSecret     string
	LLM                *agent.LLMResolver
	SQLAgent           *agent.SQLAgent
	Audit              *agent.AuditWriter
	Confirm            *agent.ConfirmationManager
	Preferences        *agent.PreferenceLoader
	ExternalConsent    *ExternalReadConsentManager
}

type RouteGoalSuggester func(ctx context.Context, userID string, req RouteGoalSuggestionRequest) (RouteGoalSuggestionResponse, error)

type RouteGoalSuggestionRequest struct {
	StartLat             float64
	StartLng             float64
	RoundTrip            bool
	TargetDistanceM      float64
	TargetAscentM        *float64
	AscentTolerancePct   *float64
	SportType            string
	SubType              *string
	Profile              string
	SurfaceTendency      string
	MaxSacScale          *int
	MaxMtbScale          *int
	LandscapePrefs       map[string]int
	Modifiers            RouteGoalSuggestionModifiers
	MaxVariants          *int
	ProfileSportOverride bool
}

type RouteGoalSuggestionModifiers struct {
	IsElectric *bool
	IsIndoor   *bool
	IsVirtual  *bool
	IsCommute  *bool
}

type RouteGoalSuggestionResponse struct {
	Variants          []map[string]any
	CoverageAvailable bool
	PartialSearch     bool
}

type Registry struct {
	tools map[string]*Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

func RegisterAll(reg *Registry) {
	RegisterActivityTools(reg)
	RegisterDocumentTools(reg)
	RegisterEntityTools(reg)
	RegisterGearTools(reg)
	RegisterGeoTools(reg)
	RegisterHealthTools(reg)
	RegisterMiscWriteTools(reg)
	RegisterRouteTools(reg)
	RegisterRoutingProfileWriteTools(reg)
	RegisterActivityContextTools(reg)
	RegisterSegmentTools(reg)
	reg.Register(SearchPhotosByContentTool())
	reg.Register(RunSQLTool())
	RegisterTagTools(reg)
	RegisterRuleWriteTools(reg)
	RegisterRulePreviewTools(reg)
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
		return nil, ErrWriteToolRequiresEnvelope
	}
	if t.ExecuteRead == nil {
		return nil, fmt.Errorf("tool has no read executor: %s", toolName)
	}
	if err := validateArgs(t.Name, t.InputSchema, args); err != nil {
		return nil, err
	}
	return t.ExecuteRead(ctx, deps, userID, args)
}

func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
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

func (r *Registry) Definitions(filter []string, prefs *agent.ChatPreferences) []llmwire.ToolDefinition {
	filterSet := namesSet(filter)
	allowedWrites := allowedWriteToolSet(prefs)
	defs := make([]llmwire.ToolDefinition, 0, len(r.tools))
	names := r.sortedNames()
	if len(filter) > 0 {
		names = filter
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
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

func (t *Tool) definition() llmwire.ToolDefinition {
	if err := validateSchema(t.Name, t.InputSchema); err != nil {
		panic(err)
	}
	return llmwire.ToolDefinition{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: append(json.RawMessage(nil), t.InputSchema...),
	}
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
