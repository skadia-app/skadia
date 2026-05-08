package sporttaxgen

import (
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

const selectableSportTuplesSource = "selectable_sport_tuples"

type RuleProjection struct {
	TupleSelectors  []RuleTupleSelectorConfig  `yaml:"tuple_selectors"`
	Modifiers       []RuleModifierConfig       `yaml:"modifiers"`
	TraitConditions []RuleTraitConditionConfig `yaml:"trait_conditions"`
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

type RuleTraitConditionConfig struct {
	Key          string               `yaml:"key"`
	Labels       LocaleMap            `yaml:"labels"`
	Field        string               `yaml:"field"`
	Operators    []string             `yaml:"operators"`
	Source       string               `yaml:"source"`
	OptionLabels map[string]LocaleMap `yaml:"option_labels"`
}

type ExpandedRuleProjection struct {
	TupleSelectors  []ExpandedRuleTupleSelector
	TupleOptions    []ExpandedRuleTupleOption
	Modifiers       []ExpandedRuleModifier
	TraitConditions []ExpandedRuleTraitCondition
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

type ExpandedRuleTraitCondition struct {
	Key       string
	Label     LocaleMap
	Field     string
	Operators []string
	Source    string
	Options   []ExpandedRuleTraitOption
}

type ExpandedRuleTraitOption struct {
	Key   string
	Label LocaleMap
}

func LoadRuleProjection(r io.Reader, taxonomy Taxonomy, modifiers ModifierRegistry) (RuleProjection, error) {
	var projection RuleProjection
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&projection); err != nil {
		return RuleProjection{}, err
	}
	if len(projection.Modifiers) > 0 {
		return RuleProjection{}, fmt.Errorf("rule projection modifiers are derived from modifier registry")
	}
	projection.Modifiers = ruleModifierConfigsFromRegistry(modifiers)
	if err := validateRuleProjection(projection, taxonomy, modifiers); err != nil {
		return RuleProjection{}, err
	}
	return projection, nil
}

func ExpandRuleProjection(taxonomy Taxonomy, projection RuleProjection, modifiers ModifierRegistry) (ExpandedRuleProjection, error) {
	projection.Modifiers = ruleModifierConfigsFromRegistry(modifiers)
	if err := validateRuleProjection(projection, taxonomy, modifiers); err != nil {
		return ExpandedRuleProjection{}, err
	}
	out := ExpandedRuleProjection{
		TupleSelectors:  make([]ExpandedRuleTupleSelector, 0, len(projection.TupleSelectors)),
		TupleOptions:    expandSelectableRuleTupleOptions(taxonomy, projection.Modifiers),
		Modifiers:       make([]ExpandedRuleModifier, 0, len(projection.Modifiers)),
		TraitConditions: make([]ExpandedRuleTraitCondition, 0, len(projection.TraitConditions)),
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
	for _, condition := range projection.TraitConditions {
		out.TraitConditions = append(out.TraitConditions, ExpandedRuleTraitCondition{
			Key:       condition.Key,
			Label:     cloneLocaleMap(condition.Labels),
			Field:     condition.Field,
			Operators: append([]string(nil), condition.Operators...),
			Source:    condition.Source,
			Options:   expandRuleTraitOptions(taxonomy, condition.Source, condition.OptionLabels),
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

func (p ExpandedRuleProjection) HasTraitValue(field, value string) bool {
	for _, condition := range p.TraitConditions {
		if condition.Field != field {
			continue
		}
		for _, option := range condition.Options {
			if option.Key == value {
				return true
			}
		}
	}
	return false
}

func (p ExpandedRuleProjection) TraitValueLabelDE(field, value string) string {
	for _, condition := range p.TraitConditions {
		if condition.Field != field {
			continue
		}
		for _, option := range condition.Options {
			if option.Key == value {
				return option.Label["de"]
			}
		}
	}
	return ""
}

func validateRuleProjection(projection RuleProjection, taxonomy Taxonomy, modifiers ModifierRegistry) error {
	if len(projection.TupleSelectors) == 0 {
		return fmt.Errorf("missing tuple_selectors")
	}
	if len(projection.TraitConditions) == 0 {
		return fmt.Errorf("missing trait_conditions")
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
	wantModifierKeys := modifiers.ModifierKeys()
	gotModifierKeys := make([]string, 0, len(projection.Modifiers))
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
		def, ok := modifiers.ModifierByKey(modifier.Key)
		if !ok {
			return fmt.Errorf("modifier %q: missing registry definition", modifier.Key)
		}
		if modifier.Labels["de"] != def.Labels["de"] {
			return fmt.Errorf("modifier %q: label %q must come from registry label %q", modifier.Key, modifier.Labels["de"], def.Labels["de"])
		}
		if modifier.ConditionApplicableFrom != def.ApplicabilitySource || modifier.ActionApplicableFrom != def.ApplicabilitySource {
			return fmt.Errorf("modifier %q: applicability source must come from registry", modifier.Key)
		}
		gotModifierKeys = append(gotModifierKeys, modifier.Key)
	}
	if !stringSlicesEqual(gotModifierKeys, wantModifierKeys) {
		return fmt.Errorf("rule projection modifier keys = %v, want registry rule modifier keys %v", gotModifierKeys, wantModifierKeys)
	}
	if len(selectableRuleTupleOptions(taxonomy)) == 0 {
		return fmt.Errorf("rule projection requires at least one selectable sport tuple")
	}
	if err := validateRuleTraitConditions(projection.TraitConditions); err != nil {
		return err
	}
	return nil
}

// ruleModifierConfigsFromRegistry returns one config per modifier in the registry.
// All modifiers participate in rule conditions and actions; rule applicability is
// now owned by the rules subsystem (rules.RulesApplicableModifierKeys).
func ruleModifierConfigsFromRegistry(modifiers ModifierRegistry) []RuleModifierConfig {
	out := make([]RuleModifierConfig, 0, len(modifiers.Modifiers))
	for _, modifier := range modifiers.Modifiers {
		out = append(out, RuleModifierConfig{
			Key:                     modifier.Key,
			Labels:                  cloneLocaleMap(modifier.Labels),
			ConditionApplicableFrom: modifier.ApplicabilitySource,
			ActionApplicableFrom:    modifier.ApplicabilitySource,
			RenderExisting:          true,
		})
	}
	return out
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

func validateRuleTraitConditions(conditions []RuleTraitConditionConfig) error {
	keys := map[string]struct{}{}
	fields := map[string]struct{}{}
	for i, condition := range conditions {
		if condition.Key == "" {
			return fmt.Errorf("trait_conditions[%d]: missing key", i)
		}
		if _, ok := keys[condition.Key]; ok {
			return fmt.Errorf("duplicate trait condition key %q", condition.Key)
		}
		keys[condition.Key] = struct{}{}
		if condition.Labels["de"] == "" {
			return fmt.Errorf("trait condition %q: missing labels.de", condition.Key)
		}
		if condition.Field == "" {
			return fmt.Errorf("trait condition %q: missing field", condition.Key)
		}
		if _, ok := fields[condition.Field]; ok {
			return fmt.Errorf("duplicate trait condition field %q", condition.Field)
		}
		fields[condition.Field] = struct{}{}
		if condition.Field != condition.Key {
			return fmt.Errorf("trait condition %q: key must match field %q", condition.Key, condition.Field)
		}
		if !stringSlicesEqual(condition.Operators, []string{"in"}) {
			return fmt.Errorf("trait condition %q: unsupported operators %v", condition.Key, condition.Operators)
		}
		if !ruleTraitSourceMatchesField(condition.Field, condition.Source) {
			return fmt.Errorf("trait condition %q: unsupported source %q", condition.Key, condition.Source)
		}
	}
	return nil
}

func ruleTraitSourceMatchesField(field, source string) bool {
	switch field {
	case "sport_environment":
		return source == "traits.environments"
	case "sport_equipment_family":
		return source == "traits.equipment_families"
	case "sport_movement_family":
		return source == "traits.movement_families"
	default:
		return false
	}
}

func expandRuleTraitOptions(taxonomy Taxonomy, source string, labels map[string]LocaleMap) []ExpandedRuleTraitOption {
	seen := map[string]struct{}{}
	add := func(values []string) {
		for _, value := range values {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
		}
	}
	for _, sport := range taxonomy.Sports {
		if sport.Selectable != nil && *sport.Selectable {
			add(ruleTraitValuesForSource(sport.Traits, source))
		}
		for _, subType := range sport.SubTypes {
			if subType.Selectable == nil || !*subType.Selectable {
				continue
			}
			add(ruleTraitValuesForSource(EffectiveSubTypeTraits(sport, subType), source))
		}
	}
	ordered := orderedRuleTraitValues(source, seen)
	out := make([]ExpandedRuleTraitOption, 0, len(ordered))
	for _, value := range ordered {
		label := labels[value]
		if label == nil || label["de"] == "" {
			label = LocaleMap{"de": value}
		}
		out = append(out, ExpandedRuleTraitOption{Key: value, Label: label})
	}
	return out
}

func orderedRuleTraitValues(source string, seen map[string]struct{}) []string {
	allowed := ruleTraitVocabularyForSource(source)
	out := make([]string, 0, len(seen))
	for _, value := range allowed {
		if _, ok := seen[value]; ok {
			out = append(out, value)
			delete(seen, value)
		}
	}
	unknown := make([]string, 0, len(seen))
	for value := range seen {
		unknown = append(unknown, value)
	}
	sort.Strings(unknown)
	return append(out, unknown...)
}

func ruleTraitVocabularyForSource(source string) []string {
	switch source {
	case "traits.environments":
		return TraitEnvironmentValues
	case "traits.equipment_families":
		return TraitEquipmentFamilyValues
	case "traits.movement_families":
		return TraitMovementFamilyValues
	default:
		return nil
	}
}

func ruleTraitValuesForSource(traits Traits, source string) []string {
	switch source {
	case "traits.environments":
		return traits.Environments
	case "traits.equipment_families":
		return traits.EquipmentFamilies
	case "traits.movement_families":
		return traits.MovementFamilies
	default:
		return nil
	}
}

func selectableRuleTupleOptions(taxonomy Taxonomy) []ExpandedRuleTupleOption {
	out := []ExpandedRuleTupleOption{}
	for _, sport := range taxonomy.Sports {
		if ruleTupleParentSelectable(sport) {
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

func ruleTupleParentSelectable(sport Sport) bool {
	if sport.SportType == "unknown" {
		return true
	}
	return sport.Selectable != nil && *sport.Selectable
}

func ruleTupleSubTypeLabel(sport Sport, subType SubType) string {
	return subType.Labels["de"]
}

func modifierApplicabilityForSource(sport Sport, source string) bool {
	if sport.ModifierApplicability == nil {
		return false
	}
	const prefix = "modifier_applicability."
	if len(source) <= len(prefix) || source[:len(prefix)] != prefix {
		return false
	}
	return (*sport.ModifierApplicability)[source[len(prefix):]]
}

func mustFindProjectionSport(taxonomy Taxonomy, sportType string) Sport {
	for _, sport := range taxonomy.Sports {
		if sport.SportType == sportType {
			return sport
		}
	}
	panic("validated rule tuple sport missing")
}
