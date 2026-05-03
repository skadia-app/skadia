package sporttaxgen

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v2"
)

const selectableSportTuplesSource = "selectable_sport_tuples"

var allowedRuleModifierSources = map[string]struct{}{
	"modifier_applicability.is_electric": {},
	"modifier_applicability.is_indoor":   {},
	"modifier_applicability.is_virtual":  {},
	"modifier_applicability.is_commute":  {},
}

type RuleProjection struct {
	TupleSelectors []RuleTupleSelectorConfig `yaml:"tuple_selectors"`
	Modifiers      []RuleModifierConfig      `yaml:"modifiers"`
}

type RuleTupleSelectorConfig struct {
	Key       string    `yaml:"key"`
	Labels    LocaleMap `yaml:"labels"`
	Field     string    `yaml:"field"`
	Operators []string  `yaml:"operators"`
	Action    string    `yaml:"action"`
	Source    string    `yaml:"source"`
}

type RuleModifierConfig struct {
	Key                     string    `yaml:"key"`
	Labels                  LocaleMap `yaml:"labels"`
	ConditionApplicableFrom string    `yaml:"condition_applicable_from"`
	ActionApplicableFrom    string    `yaml:"action_applicable_from"`
	RenderExisting          bool      `yaml:"render_existing"`
}

type ExpandedRuleProjection struct {
	TupleSelectors []ExpandedRuleTupleSelector
	TupleOptions   []ExpandedRuleTupleOption
	Modifiers      []ExpandedRuleModifier
}

type ExpandedRuleTupleSelector struct {
	Key       string
	Label     LocaleMap
	Field     string
	Operators []string
	Action    string
	Source    string
}

type ExpandedRuleTupleOption struct {
	SportType string
	SubType   string
	Label     LocaleMap
	Modifiers []ExpandedRuleModifierApplicability
}

type ExpandedRuleModifier struct {
	Key            string
	Label          LocaleMap
	RenderExisting bool
}

type ExpandedRuleModifierApplicability struct {
	Key                 string
	Label               LocaleMap
	ConditionApplicable bool
	ActionApplicable    bool
	RenderExisting      bool
}

func LoadRuleProjection(r io.Reader, taxonomy Taxonomy) (RuleProjection, error) {
	var projection RuleProjection
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&projection); err != nil {
		return RuleProjection{}, err
	}
	if err := validateRuleProjection(projection, taxonomy); err != nil {
		return RuleProjection{}, err
	}
	return projection, nil
}

func ExpandRuleProjection(taxonomy Taxonomy, projection RuleProjection) (ExpandedRuleProjection, error) {
	if err := validateRuleProjection(projection, taxonomy); err != nil {
		return ExpandedRuleProjection{}, err
	}
	out := ExpandedRuleProjection{
		TupleSelectors: make([]ExpandedRuleTupleSelector, 0, len(projection.TupleSelectors)),
		TupleOptions:   expandSelectableRuleTupleOptions(taxonomy, projection.Modifiers),
		Modifiers:      make([]ExpandedRuleModifier, 0, len(projection.Modifiers)),
	}
	for _, selector := range projection.TupleSelectors {
		out.TupleSelectors = append(out.TupleSelectors, ExpandedRuleTupleSelector{
			Key:       selector.Key,
			Label:     cloneLocaleMap(selector.Labels),
			Field:     selector.Field,
			Operators: append([]string(nil), selector.Operators...),
			Action:    selector.Action,
			Source:    selector.Source,
		})
	}
	for _, modifier := range projection.Modifiers {
		out.Modifiers = append(out.Modifiers, ExpandedRuleModifier{
			Key:            modifier.Key,
			Label:          cloneLocaleMap(modifier.Labels),
			RenderExisting: modifier.RenderExisting,
		})
	}
	return out, nil
}

func (p ExpandedRuleProjection) HasTupleOption(sportType, subType string) bool {
	for _, option := range p.TupleOptions {
		if option.SportType == sportType && option.SubType == subType {
			return true
		}
	}
	return false
}

func (p ExpandedRuleProjection) ModifierApplicable(sportType, subType, key, surface string) bool {
	for _, option := range p.TupleOptions {
		if option.SportType != sportType || option.SubType != subType {
			continue
		}
		for _, modifier := range option.Modifiers {
			if modifier.Key != key {
				continue
			}
			switch surface {
			case "condition":
				return modifier.ConditionApplicable
			case "action":
				return modifier.ActionApplicable
			default:
				return false
			}
		}
	}
	return false
}

func validateRuleProjection(projection RuleProjection, taxonomy Taxonomy) error {
	if len(projection.TupleSelectors) == 0 {
		return fmt.Errorf("missing tuple_selectors")
	}
	if len(projection.Modifiers) == 0 {
		return fmt.Errorf("missing modifiers")
	}
	selectorKeys := map[string]struct{}{}
	for i, selector := range projection.TupleSelectors {
		if selector.Key == "" {
			return fmt.Errorf("tuple_selectors[%d]: missing key", i)
		}
		if _, ok := selectorKeys[selector.Key]; ok {
			return fmt.Errorf("duplicate tuple selector key %q", selector.Key)
		}
		selectorKeys[selector.Key] = struct{}{}
		if selector.Labels["de"] == "" {
			return fmt.Errorf("tuple selector %q: missing labels.de", selector.Key)
		}
		if selector.Source != selectableSportTuplesSource {
			return fmt.Errorf("tuple selector %q: unsupported source %q", selector.Key, selector.Source)
		}
		hasField := selector.Field != ""
		hasAction := selector.Action != ""
		if hasField == hasAction {
			return fmt.Errorf("tuple selector %q: exactly one of field or action is required", selector.Key)
		}
		if hasField {
			if selector.Field != "sport_tuple" {
				return fmt.Errorf("tuple selector %q: unsupported field %q", selector.Key, selector.Field)
			}
			if !stringSlicesEqual(selector.Operators, []string{"in"}) {
				return fmt.Errorf("tuple selector %q: unsupported operators %v", selector.Key, selector.Operators)
			}
		}
		if hasAction {
			if selector.Action != "set_sport_type" {
				return fmt.Errorf("tuple selector %q: unsupported action %q", selector.Key, selector.Action)
			}
			if len(selector.Operators) != 0 {
				return fmt.Errorf("tuple selector %q: action selector must not define operators", selector.Key)
			}
		}
	}

	modifierKeys := map[string]struct{}{}
	for i, modifier := range projection.Modifiers {
		if modifier.Key == "" {
			return fmt.Errorf("modifiers[%d]: missing key", i)
		}
		if _, ok := modifierKeys[modifier.Key]; ok {
			return fmt.Errorf("duplicate modifier key %q", modifier.Key)
		}
		modifierKeys[modifier.Key] = struct{}{}
		if modifier.Labels["de"] == "" {
			return fmt.Errorf("modifier %q: missing labels.de", modifier.Key)
		}
		if err := validateRuleModifierSource(modifier.Key, modifier.ConditionApplicableFrom); err != nil {
			return err
		}
		if err := validateRuleModifierSource(modifier.Key, modifier.ActionApplicableFrom); err != nil {
			return err
		}
	}
	if len(selectableRuleTupleOptions(taxonomy)) == 0 {
		return fmt.Errorf("rule projection requires at least one selectable sport tuple")
	}
	return nil
}

func validateRuleModifierSource(key, source string) error {
	if _, ok := allowedRuleModifierSources[source]; !ok {
		return fmt.Errorf("modifier %q: unsupported applicability source %q", key, source)
	}
	if !strings.HasSuffix(source, "."+key) {
		return fmt.Errorf("modifier %q: applicability source %q does not match modifier key", key, source)
	}
	return nil
}

func expandSelectableRuleTupleOptions(taxonomy Taxonomy, modifiers []RuleModifierConfig) []ExpandedRuleTupleOption {
	options := selectableRuleTupleOptions(taxonomy)
	for i := range options {
		sport := mustFindProjectionSport(taxonomy, options[i].SportType)
		applicability := make([]ExpandedRuleModifierApplicability, 0, len(modifiers))
		for _, modifier := range modifiers {
			applicability = append(applicability, ExpandedRuleModifierApplicability{
				Key:                 modifier.Key,
				Label:               cloneLocaleMap(modifier.Labels),
				ConditionApplicable: modifierApplicabilityForSource(sport, modifier.ConditionApplicableFrom),
				ActionApplicable:    modifierApplicabilityForSource(sport, modifier.ActionApplicableFrom),
				RenderExisting:      modifier.RenderExisting,
			})
		}
		options[i].Modifiers = applicability
	}
	return options
}

func selectableRuleTupleOptions(taxonomy Taxonomy) []ExpandedRuleTupleOption {
	out := []ExpandedRuleTupleOption{}
	for _, sport := range taxonomy.Sports {
		if sport.Selectable != nil && *sport.Selectable {
			out = append(out, ExpandedRuleTupleOption{
				SportType: sport.SportType,
				Label:     LocaleMap{"de": sport.Labels["de"]},
			})
		}
		for _, subType := range sport.SubTypes {
			if subType.Selectable == nil || !*subType.Selectable {
				continue
			}
			out = append(out, ExpandedRuleTupleOption{
				SportType: sport.SportType,
				SubType:   subType.SubType,
				Label:     LocaleMap{"de": ruleTupleSubTypeLabel(sport, subType)},
			})
		}
	}
	return out
}

func ruleTupleSubTypeLabel(sport Sport, subType SubType) string {
	return subType.Labels["de"]
}

func modifierApplicabilityForSource(sport Sport, source string) bool {
	if sport.ModifierApplicability == nil {
		return false
	}
	switch source {
	case "modifier_applicability.is_electric":
		return sport.ModifierApplicability.IsElectric != nil && *sport.ModifierApplicability.IsElectric
	case "modifier_applicability.is_indoor":
		return sport.ModifierApplicability.IsIndoor != nil && *sport.ModifierApplicability.IsIndoor
	case "modifier_applicability.is_virtual":
		return sport.ModifierApplicability.IsVirtual != nil && *sport.ModifierApplicability.IsVirtual
	case "modifier_applicability.is_commute":
		return sport.ModifierApplicability.IsCommute != nil && *sport.ModifierApplicability.IsCommute
	default:
		return false
	}
}

func mustFindProjectionSport(taxonomy Taxonomy, sportType string) Sport {
	for _, sport := range taxonomy.Sports {
		if sport.SportType == sportType {
			return sport
		}
	}
	panic("validated rule tuple sport missing")
}
