package sporttaxgen

import (
	"fmt"
	"sort"
	"strings"
)

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

var gearDefaultProjectionSources = map[string]struct{}{
	"strava": {},
}

func BuildGearDefaultProjection(taxonomy Taxonomy, sources []SourceAliases) (GearDefaultProjection, error) {
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
		if _, ok := gearDefaultProjectionSources[source.Source]; !ok {
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
