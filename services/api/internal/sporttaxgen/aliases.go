package sporttaxgen

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type SourceAliases struct {
	Source  string        `yaml:"source"`
	Aliases []SourceAlias `yaml:"aliases"`
}

type SourceAlias struct {
	Raw       string               `yaml:"raw"`
	RawSub    string               `yaml:"raw_sub"`
	SportType string               `yaml:"sport_type"`
	SubType   string               `yaml:"sub_type"`
	Modifiers SourceAliasModifiers `yaml:"modifiers"`
}

type SourceAliasModifiers map[string]*bool

func LoadSourceAliases(r io.Reader, filenameStem string, taxonomy Taxonomy, modifiers ModifierRegistry) (SourceAliases, error) {
	var source SourceAliases
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&source); err != nil {
		return SourceAliases{}, err
	}
	if source.Source == "" {
		return SourceAliases{}, fmt.Errorf("source aliases %q: missing source", filenameStem)
	}
	if source.Source != filenameStem {
		return SourceAliases{}, fmt.Errorf("source aliases %q: source %q does not match filename stem", filenameStem, source.Source)
	}
	if len(source.Aliases) == 0 {
		return SourceAliases{}, fmt.Errorf("source aliases %q: aliases must not be empty", filenameStem)
	}

	sports := taxonomySportIndex(taxonomy)
	seen := make(map[string]struct{}, len(source.Aliases))
	for i := range source.Aliases {
		alias := &source.Aliases[i]
		alias.Raw = strings.TrimSpace(alias.Raw)
		alias.RawSub = strings.TrimSpace(alias.RawSub)
		alias.SportType = strings.TrimSpace(alias.SportType)
		alias.SubType = strings.TrimSpace(alias.SubType)

		if alias.Raw == "" {
			return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: missing raw", filenameStem, i)
		}
		key := alias.Raw + "\x00" + alias.RawSub
		if _, ok := seen[key]; ok {
			return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: duplicate raw/raw_sub %q/%q", filenameStem, i, alias.Raw, alias.RawSub)
		}
		seen[key] = struct{}{}

		sport, ok := sports[alias.SportType]
		if !ok {
			return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: unknown sport_type %q", filenameStem, i, alias.SportType)
		}
		if alias.SubType != "" && !aliasSportHasSubType(sport, alias.SubType) {
			return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: unknown sub_type %q for sport_type %q", filenameStem, i, alias.SubType, alias.SportType)
		}
		if alias.Modifiers != nil {
			if len(alias.Modifiers) == 0 {
				return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: empty modifiers", filenameStem, i)
			}
			for key, value := range alias.Modifiers {
				if value == nil {
					return SourceAliases{}, fmt.Errorf("source aliases %q aliases[%d]: modifier %q must be true or false", filenameStem, i, key)
				}
				if err := modifiers.ValidateModifierKeyForCapability(key, "source_alias", fmt.Sprintf("source aliases %q aliases[%d]", filenameStem, i)); err != nil {
					return SourceAliases{}, err
				}
			}
		}
	}
	return source, nil
}

func LoadSourceAliasFiles(files map[string]io.Reader, taxonomy Taxonomy, modifiers ModifierRegistry) ([]SourceAliases, error) {
	sources := make([]SourceAliases, 0, len(files))
	filenameStems := make([]string, 0, len(files))
	for filenameStem := range files {
		filenameStems = append(filenameStems, filenameStem)
	}
	sort.Strings(filenameStems)
	for _, filenameStem := range filenameStems {
		file := files[filenameStem]
		source, err := LoadSourceAliases(file, filenameStem, taxonomy, modifiers)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func taxonomySportIndex(taxonomy Taxonomy) map[string]Sport {
	sports := make(map[string]Sport, len(taxonomy.Sports))
	for _, sport := range taxonomy.Sports {
		sports[sport.SportType] = sport
	}
	return sports
}

func aliasSportHasSubType(sport Sport, subType string) bool {
	for _, candidate := range sport.SubTypes {
		if candidate.SubType == subType {
			return true
		}
	}
	return false
}
