package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type createRoutingProfileVariantArgs struct {
	BaseProfileKey string         `json:"base_profile_key"`
	Intent         string         `json:"intent,omitempty"`
	Changes        map[string]any `json:"changes"`
	NameHint       string         `json:"name_hint,omitempty"`
	ActivateFor    string         `json:"activate_for,omitempty"`
}

type preparedRoutingProfileVariant struct {
	Name            string                `json:"name"`
	BaseProfile     string                `json:"base_profile"`
	Config          map[string]any        `json:"config"`
	Changes         map[string]any        `json:"changes"`
	Diff            []map[string]any      `json:"profile_diff"`
	Predicates      []sport.Predicate     `json:"-"`
	ModifierFilters sport.ModifierFilters `json:"-"`
}

func RegisterRoutingProfileWriteTools(reg *Registry) {
	reg.Register(CreateRoutingProfileVariantTool())
}

func CreateRoutingProfileVariantTool() *Tool {
	return &Tool{
		Name:                 "create_routing_profile_variant",
		Description:          "Prepare and create a new user-owned routing profile variant after confirmation. Existing profiles are never overwritten.",
		InputSchema:          json.RawMessage(`{"type":"object","required":["base_profile_key","changes"],"properties":{"base_profile_key":{"type":"string","minLength":1},"intent":{"type":"string"},"changes":{"type":"object","minProperties":1,"additionalProperties":{"type":["string","number","boolean"]}},"name_hint":{"type":"string"},"activate_for":{"type":"string"}},"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeCreateRoutingProfileVariant,
		PrepareWrite:         prepareCreateRoutingProfileVariant,
		ExecuteWrite:         executeCreateRoutingProfileVariant,
	}
}

func summarizeCreateRoutingProfileVariant(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (string, error) {
	prepared, err := buildRoutingProfileVariant(ctx, deps, userID, raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Routing-Profilvariante '%s' aus '%s' erstellen", prepared.Name, prepared.BaseProfile), nil
}

func prepareCreateRoutingProfileVariant(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (*PreparedWrite, error) {
	prepared, err := buildRoutingProfileVariant(ctx, deps, userID, raw)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(createRoutingProfileVariantArgs{
		BaseProfileKey: prepared.BaseProfile,
		Changes:        prepared.Changes,
		NameHint:       prepared.Name,
	})
	if err != nil {
		return nil, err
	}
	return &PreparedWrite{
		Summary: fmt.Sprintf("Routing-Profilvariante '%s' erstellen", prepared.Name),
		Preview: map[string]any{
			"name":         prepared.Name,
			"base_profile": prepared.BaseProfile,
			"profile_diff": prepared.Diff,
			"note":         "Die Variante wird als neues Profil angelegt; bestehende Profile werden nicht überschrieben.",
		},
		Args: normalized,
	}, nil
}

func executeCreateRoutingProfileVariant(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	if deps == nil || deps.Tx == nil {
		return nil, fmt.Errorf("transaction is required")
	}
	prepared, err := buildRoutingProfileVariant(ctx, deps.Deps, userID, raw)
	if err != nil {
		return nil, err
	}
	configJSON, err := json.Marshal(prepared.Config)
	if err != nil {
		return nil, err
	}
	if prepared.Predicates == nil {
		prepared.Predicates = []sport.Predicate{}
	}
	predicatesJSON, _ := json.Marshal(prepared.Predicates)
	modifierFiltersJSON, _ := json.Marshal(prepared.ModifierFilters)
	sportTypes := sport.SportTypesFromPredicates(prepared.Predicates)
	var id string
	err = deps.Tx.QueryRow(ctx, `
INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'user')
RETURNING id::text`,
		userID, prepared.Name, prepared.BaseProfile, sportTypes, predicatesJSON, modifierFiltersJSON, configJSON,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create routing profile variant: %w", err)
	}
	return map[string]any{
		"status":        "ok",
		"profile_id":    id,
		"name":          prepared.Name,
		"base_profile":  prepared.BaseProfile,
		"profile_diff":  prepared.Diff,
		"activate_note": "Aktivierung als Standardprofil ist absichtlich nicht Teil dieser Aktion.",
	}, nil
}

func buildRoutingProfileVariant(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (preparedRoutingProfileVariant, error) {
	var args createRoutingProfileVariantArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return preparedRoutingProfileVariant{}, fmt.Errorf("decode create_routing_profile_variant args: %w", err)
	}
	if strings.TrimSpace(args.BaseProfileKey) == "" {
		return preparedRoutingProfileVariant{}, fmt.Errorf("base_profile_key is required")
	}
	if len(args.Changes) == 0 {
		return preparedRoutingProfileVariant{}, fmt.Errorf("at least one profile change is required")
	}
	base, err := resolveVariantBaseProfile(ctx, deps, userID, args.BaseProfileKey)
	if err != nil {
		return preparedRoutingProfileVariant{}, err
	}
	allowed := routingProfileOverrideKeySet()
	config := base.Config
	if config == nil {
		config = map[string]any{}
	}
	config["profile"] = base.BaseProfile
	config["name"] = routeProfileVariantName(args.NameHint, args.Intent, base.BaseProfile)
	diff := make([]map[string]any, 0, len(args.Changes))
	changes := make(map[string]any, len(args.Changes))
	keys := make([]string, 0, len(args.Changes))
	for key := range args.Changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return preparedRoutingProfileVariant{}, fmt.Errorf("unsupported profile override %q; call list_routing_profiles first", key)
		}
		value := args.Changes[key]
		old := config[key]
		config[key] = value
		changes[key] = value
		diff = append(diff, map[string]any{"field": key, "old": defaultIfNil(old, "profile default"), "new": value})
	}
	cfgBytes, _ := json.Marshal(config)
	var cfg routing.ProfileConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return preparedRoutingProfileVariant{}, fmt.Errorf("invalid profile changes: %w", err)
	}
	if err := routing.ValidateProfileConfig(cfg); err != nil {
		return preparedRoutingProfileVariant{}, err
	}
	if strings.TrimSpace(args.ActivateFor) != "" {
		diff = append(diff, map[string]any{"field": "activate_for", "old": "unchanged", "new": "not applied", "reason": "default activation is a separate confirmed action"})
	}
	return preparedRoutingProfileVariant{
		Name:            config["name"].(string),
		BaseProfile:     base.BaseProfile,
		Config:          config,
		Changes:         changes,
		Diff:            diff,
		Predicates:      base.Predicates,
		ModifierFilters: base.ModifierFilters,
	}, nil
}

type routingProfileVariantBase struct {
	BaseProfile     string
	Config          map[string]any
	Predicates      []sport.Predicate
	ModifierFilters sport.ModifierFilters
}

func resolveVariantBaseProfile(ctx context.Context, deps *Deps, userID, key string) (routingProfileVariantBase, error) {
	if deps != nil && deps.DB != nil {
		profiles, err := listRoutingProfilesForTool(ctx, deps, userID)
		if err != nil {
			return routingProfileVariantBase{}, err
		}
		for _, profile := range profiles {
			if profile.Key == key || profile.ID == key || profile.Label == key {
				baseProfile := profile.BaseProfile
				if isValidBadRouterProfile(profile.BaseProfile) {
					baseProfile = profile.BaseProfile
				} else {
					mapped, err := brouterPreviewProfileToBadRouter(profile.BaseProfile)
					if err != nil {
						return routingProfileVariantBase{}, err
					}
					baseProfile = mapped
				}
				return routingProfileVariantBaseFromProfile(ctx, deps.DB, profile.ID, baseProfile)
			}
		}
	}
	if isValidBadRouterProfile(key) {
		return routingProfileVariantBase{BaseProfile: key, Config: map[string]any{"profile": key}}, nil
	}
	baseProfile, err := brouterPreviewProfileToBadRouter(key)
	if err != nil {
		return routingProfileVariantBase{}, err
	}
	return routingProfileVariantBase{BaseProfile: baseProfile, Config: map[string]any{"profile": baseProfile}}, nil
}

func routingProfileVariantBaseFromProfile(ctx context.Context, db *database.DB, profileID, baseProfile string) (routingProfileVariantBase, error) {
	out := routingProfileVariantBase{BaseProfile: baseProfile, Config: map[string]any{"profile": baseProfile}}
	if db == nil || strings.TrimSpace(profileID) == "" {
		return out, nil
	}
	profile, err := db.GetRoutingProfile(ctx, profileID)
	if err != nil {
		return routingProfileVariantBase{}, err
	}
	if profile == nil {
		return out, nil
	}
	if len(profile.Config) > 0 {
		if err := json.Unmarshal(profile.Config, &out.Config); err != nil {
			return routingProfileVariantBase{}, fmt.Errorf("unmarshal base routing profile config: %w", err)
		}
	}
	out.BaseProfile = baseProfile
	out.Config["profile"] = baseProfile
	out.Predicates = append([]sport.Predicate(nil), profile.Predicates...)
	out.ModifierFilters = profile.ModifierFilters
	return out, nil
}

func brouterPreviewProfileToBadRouter(profile string) (string, error) {
	switch profile {
	case "hiking-mountain", "hiking-sac2", "walking", "running":
		return "hiking", nil
	case "cycling-mountain":
		return "mtb", nil
	case "cycling-gravel":
		return "gravel", nil
	case "cycling-road":
		return "road", nil
	case "trekking":
		return "trekking", nil
	default:
		return "", fmt.Errorf("routing profile %q cannot be used as a variant base", profile)
	}
}

func isValidBadRouterProfile(profile string) bool {
	for _, valid := range routing.ValidProfileNames {
		if profile == valid {
			return true
		}
	}
	return false
}

func routingProfileOverrideKeySet() map[string]bool {
	out := map[string]bool{}
	for _, key := range routing.ProfileConfigOverrideParamKeys() {
		out[key] = true
	}
	return out
}

func routeProfileVariantName(nameHint, intent, baseProfile string) string {
	name := strings.TrimSpace(nameHint)
	if name != "" {
		return name
	}
	intent = strings.TrimSpace(intent)
	if intent != "" {
		return fmt.Sprintf("%s Variante - %s", routeProfileTitle(baseProfile), intent)
	}
	return routeProfileTitle(baseProfile) + " Variante"
}

func routeProfileTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	words := strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}

func defaultIfNil(value any, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}
