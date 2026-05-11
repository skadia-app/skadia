package sporttaxgen

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SportPredicateEntry is the sporttaxgen-local representation of a sport
// predicate (sport_type + optional sub_type). It mirrors sport.Predicate but
// lives here to avoid an import cycle: sport tests import sporttaxgen, and
// sporttaxgen must not import sport in production code.
type SportPredicateEntry struct {
	SportType string
	SubType   *string
}

// ApplicableSportPredicateEntry maps a gear type key to its default sport
// predicates as loaded from gear_defaults.yaml.
type ApplicableSportPredicateEntry struct {
	GearTypeKey string
	Predicates  []SportPredicateEntry
}

// gearDefaultProjectionConfigYAML is the raw YAML shape before conversion.
// It uses explicit yaml tags (sport_type / sub_type) since yaml.v3 does not
// honour json tags; the outer type GearDefaultProjectionConfig stores the
// already-converted sport.Predicate values.
type gearDefaultProjectionConfigYAML struct {
	ProjectionSources          []string                          `yaml:"projection_sources"`
	ApplicableSportPredicates  []gearDefaultsPredicateEntryYAML `yaml:"applicable_sport_predicates"`
}

type GearDefaultProjectionConfig struct {
	ProjectionSources         []string
	ApplicableSportPredicates []ApplicableSportPredicateEntry
}

type GearDefaultProjection struct {
	StorageOptions []GearDefaultSportOption
	ManualOptions  []GearDefaultSportOption
	Sources        []GearDefaultSourceProjection
}

type GearDefaultSportOption struct {
	SportType string
	LabelDE   string
}

type GearDefaultSourceProjection struct {
	Source  string
	Aliases []GearDefaultAlias
}

type GearDefaultAlias struct {
	Raw       string
	SportType string
}

func LoadGearDefaultProjectionConfig(r io.Reader, sources []SourceAliases) (GearDefaultProjectionConfig, error) {
	var raw gearDefaultProjectionConfigYAML
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return GearDefaultProjectionConfig{}, err
	}
	if len(raw.ProjectionSources) == 0 {
		return GearDefaultProjectionConfig{}, fmt.Errorf("gear default projection: projection_sources must not be empty")
	}
	knownSources := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		knownSources[source.Source] = struct{}{}
	}
	seen := make(map[string]struct{}, len(raw.ProjectionSources))
	for i, source := range raw.ProjectionSources {
		source = strings.TrimSpace(source)
		if source == "" {
			return GearDefaultProjectionConfig{}, fmt.Errorf("gear default projection: projection_sources[%d] is empty", i)
		}
		if _, ok := seen[source]; ok {
			return GearDefaultProjectionConfig{}, fmt.Errorf("gear default projection: duplicate projection source %q", source)
		}
		seen[source] = struct{}{}
		if _, ok := knownSources[source]; !ok {
			return GearDefaultProjectionConfig{}, fmt.Errorf("gear default projection: unknown projection source %q", source)
		}
		raw.ProjectionSources[i] = source
	}

	// Convert YAML wire types to SportPredicateEntry values.
	entries := make([]ApplicableSportPredicateEntry, 0, len(raw.ApplicableSportPredicates))
	for _, rawEntry := range raw.ApplicableSportPredicates {
		preds := make([]SportPredicateEntry, 0, len(rawEntry.Predicates))
		for _, p := range rawEntry.Predicates {
			preds = append(preds, p.ToSportPredicateEntry())
		}
		entries = append(entries, ApplicableSportPredicateEntry{
			GearTypeKey: rawEntry.GearTypeKey,
			Predicates:  preds,
		})
	}

	return GearDefaultProjectionConfig{
		ProjectionSources:         raw.ProjectionSources,
		ApplicableSportPredicates: entries,
	}, nil
}

func BuildGearDefaultProjection(taxonomy Taxonomy, sources []SourceAliases, config GearDefaultProjectionConfig) (GearDefaultProjection, error) {
	if len(config.ProjectionSources) == 0 {
		return GearDefaultProjection{}, fmt.Errorf("gear default projection: projection_sources must not be empty")
	}
	projectionSources := make(map[string]struct{}, len(config.ProjectionSources))
	for _, source := range config.ProjectionSources {
		projectionSources[source] = struct{}{}
	}
	sports := taxonomySportIndex(taxonomy)
	storageOptions := make([]GearDefaultSportOption, 0, len(taxonomy.Sports))
	manualOptions := make([]GearDefaultSportOption, 0, len(taxonomy.Sports))
	for _, sport := range taxonomy.Sports {
		if sport.SportType == "unknown" {
			continue
		}
		option := GearDefaultSportOption{
			SportType: sport.SportType,
			LabelDE:   sportLabelDE(sport),
		}
		storageOptions = append(storageOptions, option)
		if sport.Selectable != nil && *sport.Selectable {
			manualOptions = append(manualOptions, option)
		}
	}
	sortGearDefaultSportOptions(storageOptions)
	sortGearDefaultSportOptions(manualOptions)

	projectedSources := make([]GearDefaultSourceProjection, 0, len(sources))
	for _, source := range sources {
		if _, ok := projectionSources[source.Source]; !ok {
			continue
		}
		seen := map[string]string{}
		aliases := make([]GearDefaultAlias, 0, len(source.Aliases))
		for _, alias := range source.Aliases {
			if strings.TrimSpace(alias.RawSub) != "" {
				continue
			}
			raw := strings.TrimSpace(alias.Raw)
			if raw == "" {
				continue
			}
			if _, ok := sports[alias.SportType]; !ok {
				return GearDefaultProjection{}, fmt.Errorf("gear defaults %q raw %q: unknown sport_type %q", source.Source, raw, alias.SportType)
			}
			if previous, ok := seen[raw]; ok {
				if previous != alias.SportType {
					return GearDefaultProjection{}, fmt.Errorf("gear defaults %q raw %q maps to both %q and %q", source.Source, raw, previous, alias.SportType)
				}
				continue
			}
			seen[raw] = alias.SportType
			aliases = append(aliases, GearDefaultAlias{Raw: raw, SportType: alias.SportType})
		}
		sort.SliceStable(aliases, func(i, j int) bool { return aliases[i].Raw < aliases[j].Raw })
		projectedSources = append(projectedSources, GearDefaultSourceProjection{Source: source.Source, Aliases: aliases})
	}
	sort.SliceStable(projectedSources, func(i, j int) bool { return projectedSources[i].Source < projectedSources[j].Source })

	return GearDefaultProjection{StorageOptions: storageOptions, ManualOptions: manualOptions, Sources: projectedSources}, nil
}

func sortGearDefaultSportOptions(options []GearDefaultSportOption) {
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].LabelDE == options[j].LabelDE {
			return options[i].SportType < options[j].SportType
		}
		return options[i].LabelDE < options[j].LabelDE
	})
}

func sportLabelDE(sport Sport) string {
	if sport.Labels != nil && sport.Labels["de"] != "" {
		return sport.Labels["de"]
	}
	return sport.SportType
}
