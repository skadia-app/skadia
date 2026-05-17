package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	Capability   *ToolCapability
}

type ToolCapability struct {
	Area              string
	PlannerSelectable bool
	Limits            []string
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
	reg.Register(AgentCapabilityManifestTool(reg))
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
	if t.Capability == nil {
		capability := inferToolCapability(t)
		t.Capability = &capability
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
	return append([]string(nil), filter...)
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

func (r *Registry) CapabilityManifest() []map[string]any {
	if r == nil {
		return nil
	}
	type areaEntry struct {
		Description string
		ReadTools   []string
		WriteTools  []string
		Limits      []string
	}
	areas := map[string]*areaEntry{}
	for _, name := range r.sortedNames() {
		tool := r.tools[name]
		if tool == nil || tool.Capability == nil || !tool.Capability.PlannerSelectable {
			continue
		}
		area := tool.Capability.Area
		if area == "" {
			area = "other"
		}
		entry := areas[area]
		if entry == nil {
			entry = &areaEntry{Description: capabilityAreaDescription(area)}
			areas[area] = entry
		}
		if tool.Permission == PermissionRead {
			entry.ReadTools = append(entry.ReadTools, name)
		} else {
			entry.WriteTools = append(entry.WriteTools, name)
		}
		for _, limit := range tool.Capability.Limits {
			if !stringInSlice(entry.Limits, limit) {
				entry.Limits = append(entry.Limits, limit)
			}
		}
	}
	order := []string{"activities", "routing", "segments", "gear", "documents_photos", "health_training", "automation_tags", "geo", "capabilities", "other"}
	out := make([]map[string]any, 0, len(areas))
	for _, area := range order {
		entry := areas[area]
		if entry == nil {
			continue
		}
		out = append(out, map[string]any{
			"area":        area,
			"description": entry.Description,
			"read_tools":  entry.ReadTools,
			"write_tools": entry.WriteTools,
			"limits":      entry.Limits,
		})
	}
	return out
}

func inferToolCapability(tool *Tool) ToolCapability {
	area := capabilityAreaForToolName(tool.Name)
	limits := []string(nil)
	if tool.Permission != PermissionRead {
		limits = append(limits, "Write tools require user-enabled permissions and per-action confirmation.")
	}
	if tool.ExternalRead != nil {
		limits = append(limits, "External reads require the configured policy and quota to allow the request.")
	}
	if tool.RateLimitPerHour > 0 {
		limits = append(limits, fmt.Sprintf("Rate-limited to %d calls per user and hour.", tool.RateLimitPerHour))
	}
	return ToolCapability{
		Area:              area,
		PlannerSelectable: area != "other",
		Limits:            limits,
	}
}

func capabilityAreaForToolName(name string) string {
	switch {
	case name == "get_agent_capability_manifest":
		return "capabilities"
	case strings.Contains(name, "route") || strings.Contains(name, "routing") || strings.Contains(name, "weather") || strings.Contains(name, "coverage") || name == "geocode_place" || name == "resolve_user_location":
		return "routing"
	case strings.Contains(name, "segment"):
		return "segments"
	case strings.Contains(name, "gear") || strings.Contains(name, "maintenance"):
		return "gear"
	case strings.Contains(name, "document") || strings.Contains(name, "photo"):
		return "documents_photos"
	case strings.Contains(name, "health") || strings.Contains(name, "training") || strings.Contains(name, "analytics"):
		return "health_training"
	case strings.Contains(name, "tag") || strings.Contains(name, "automation_rule") || strings.Contains(name, "rule"):
		return "automation_tags"
	case strings.Contains(name, "geo") || strings.Contains(name, "poi") || strings.Contains(name, "trail") || strings.Contains(name, "terrain"):
		return "geo"
	case strings.Contains(name, "activity") || strings.Contains(name, "activities") || strings.Contains(name, "record") || strings.Contains(name, "period") || strings.Contains(name, "stats") || strings.Contains(name, "time_series"):
		return "activities"
	default:
		return "other"
	}
}

func capabilityAreaDescription(area string) string {
	switch area {
	case "activities":
		return "Owned activity search, details, records, statistics, time series, comparisons, and activity selections."
	case "routing":
		return "Place resolution, air-line distance, route coverage diagnostics, route previews, goal route suggestions, terrain, and weather context."
	case "segments":
		return "Segment existence, efforts, records, and segment filters."
	case "gear":
		return "Gear lookup, stats, activity assignment, documents, components, service history, and maintenance status."
	case "documents_photos":
		return "Search extracted document text and photo content when the user has enabled those data surfaces."
	case "health_training":
		return "Health status, training load, analytics summaries, and trend-oriented aggregate reads."
	case "automation_tags":
		return "Tag and automation-rule management with previewed writes."
	case "geo":
		return "Stored geographic features and activity geographic context."
	case "capabilities":
		return "Assistant tool and service capability discovery."
	default:
		return "Other registered tools."
	}
}

func stringInSlice(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
