package sporttaxgen

import (
	"fmt"
	"io"
	"regexp"

	"gopkg.in/yaml.v2"
)

type Taxonomy struct {
	Categories []Category `yaml:"categories"`
	Sports     []Sport    `yaml:"sports"`
}

type Category struct {
	Key    string    `yaml:"key"`
	Labels LocaleMap `yaml:"labels"`
	Color  Color     `yaml:"color"`
}

type Color struct {
	Token    string `yaml:"token"`
	LightHex string `yaml:"light_hex"`
}

type LocaleMap map[string]string

type Sport struct {
	SportType             string                 `yaml:"sport_type"`
	CategoryKey           string                 `yaml:"category_key"`
	Labels                LocaleMap              `yaml:"labels"`
	AutoTitles            LocaleMap              `yaml:"auto_titles"`
	SegmentGroup          string                 `yaml:"segment_group"`
	TypicallyHasGPS       bool                   `yaml:"typically_has_gps"`
	MovingThreshold       float64                `yaml:"moving_threshold"`
	IsIndoor              bool                   `yaml:"is_indoor"`
	Lifecycle             string                 `yaml:"lifecycle"`
	Selectable            *bool                  `yaml:"selectable"`
	ModifierApplicability *ModifierApplicability `yaml:"modifier_applicability"`
	ModifierDefaults      *ModifierDefaults      `yaml:"modifier_defaults"`
	Traits                Traits                 `yaml:"traits"`
	SubTypes              []SubType              `yaml:"sub_types"`
}

type Traits struct {
	Environments      []string `yaml:"environments"`
	MovementFamilies  []string `yaml:"movement_families"`
	EquipmentFamilies []string `yaml:"equipment_families"`
	ActivityForm      string   `yaml:"activity_form"`
	PropulsionModes   []string `yaml:"propulsion_modes"`
}

type ModifierApplicability struct {
	IsElectric *bool `yaml:"is_electric"`
	IsIndoor   *bool `yaml:"is_indoor"`
	IsVirtual  *bool `yaml:"is_virtual"`
	IsCommute  *bool `yaml:"is_commute"`
}

type ModifierDefaults struct {
	IsIndoor *bool `yaml:"is_indoor"`
}

type SubType struct {
	SubType          string            `yaml:"sub_type"`
	Labels           LocaleMap         `yaml:"labels"`
	Details          LocaleMap         `yaml:"details"`
	AutoTitles       LocaleMap         `yaml:"auto_titles"`
	MovingThreshold  *float64          `yaml:"moving_threshold"`
	Selectable       *bool             `yaml:"selectable"`
	Traits           *Traits           `yaml:"traits"`
	ModifierDefaults *ModifierDefaults `yaml:"modifier_defaults"`
}

var validHex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var TraitEnvironmentValues = []string{"land", "water", "snow", "ice", "air", "indoor", "generic"}
var TraitMovementFamilyValues = []string{"run", "walk", "roll", "pedal", "swim", "paddle", "row", "glide", "climb", "lift", "strike", "throw", "dance", "mind_body", "combat", "ride", "drive", "fly", "multi", "fish", "generic"}
var TraitEquipmentFamilyValues = []string{"bike", "wheelchair", "skate", "ski", "snowboard", "sled", "board", "paddle", "boat", "sail", "breathing_gear", "fishing_gear", "machine", "weights", "rope", "ball", "racket", "stick", "disc", "climbing_gear", "aircraft", "animal", "vehicle", "weapon", "generic"}
var TraitActivityFormValues = []string{"single", "family", "composite", "transition", "mode", "fallback", "system"}
var TraitPropulsionModeValues = []string{"human", "gravity", "wind", "motor", "towed", "animal", "generic"}

func LoadTaxonomy(r io.Reader) (Taxonomy, error) {
	var taxonomy Taxonomy
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&taxonomy); err != nil {
		return Taxonomy{}, err
	}
	applyTaxonomyDefaults(&taxonomy)
	if err := validateTaxonomy(taxonomy); err != nil {
		return Taxonomy{}, err
	}
	return taxonomy, nil
}

func applyTaxonomyDefaults(t *Taxonomy) {
	for i := range t.Sports {
		s := &t.Sports[i]
		if s.Lifecycle == "" {
			s.Lifecycle = "canonical"
		}
		if s.Selectable == nil {
			defaultSelectable := s.Lifecycle == "canonical"
			s.Selectable = &defaultSelectable
		}
		for j := range s.SubTypes {
			if s.SubTypes[j].Selectable == nil {
				defaultSelectable := true
				s.SubTypes[j].Selectable = &defaultSelectable
			}
		}
	}
}

func validateTaxonomy(t Taxonomy) error {
	categoryKeys := map[string]struct{}{}
	for i, c := range t.Categories {
		if c.Key == "" {
			return fmt.Errorf("categories[%d]: missing key", i)
		}
		if _, ok := categoryKeys[c.Key]; ok {
			return fmt.Errorf("duplicate category key %q", c.Key)
		}
		categoryKeys[c.Key] = struct{}{}
		if c.Labels["de"] == "" {
			return fmt.Errorf("category %q: missing labels.de", c.Key)
		}
		if c.Color.Token == "" {
			return fmt.Errorf("category %q: missing color.token", c.Key)
		}
		if !validHex.MatchString(c.Color.LightHex) {
			return fmt.Errorf("category %q: invalid color.light_hex %q", c.Key, c.Color.LightHex)
		}
	}

	sportTypes := map[string]struct{}{}
	for i, s := range t.Sports {
		if s.SportType == "" {
			return fmt.Errorf("sports[%d]: missing sport_type", i)
		}
		if _, ok := sportTypes[s.SportType]; ok {
			return fmt.Errorf("duplicate sport_type %q", s.SportType)
		}
		sportTypes[s.SportType] = struct{}{}
		if _, ok := categoryKeys[s.CategoryKey]; !ok {
			return fmt.Errorf("sport %q: unknown category_key %q", s.SportType, s.CategoryKey)
		}
		if s.Labels["de"] == "" {
			return fmt.Errorf("sport %q: missing labels.de", s.SportType)
		}
		if s.AutoTitles["de"] == "" {
			return fmt.Errorf("sport %q: missing auto_titles.de", s.SportType)
		}
		if !isValidSegmentGroup(s.SegmentGroup) {
			return fmt.Errorf("sport %q: invalid segment_group %q", s.SportType, s.SegmentGroup)
		}
		if s.MovingThreshold < 0 {
			return fmt.Errorf("sport %q: negative moving_threshold", s.SportType)
		}
		if !isValidLifecycle(s.Lifecycle) {
			return fmt.Errorf("sport %q: invalid lifecycle %q", s.SportType, s.Lifecycle)
		}
		if s.Selectable == nil {
			return fmt.Errorf("sport %q: missing selectable default", s.SportType)
		}
		if s.ModifierApplicability == nil {
			return fmt.Errorf("sport %q: missing modifier_applicability", s.SportType)
		}
		if s.ModifierApplicability.IsElectric == nil ||
			s.ModifierApplicability.IsIndoor == nil ||
			s.ModifierApplicability.IsVirtual == nil ||
			s.ModifierApplicability.IsCommute == nil {
			return fmt.Errorf("sport %q: incomplete modifier_applicability", s.SportType)
		}
		if err := validateModifierDefaults(s, s.ModifierDefaults, fmt.Sprintf("sport %q", s.SportType)); err != nil {
			return err
		}
		if err := validateSportTraits(s); err != nil {
			return err
		}
		subTypes := map[string]struct{}{}
		for j, st := range s.SubTypes {
			if st.SubType == "" {
				return fmt.Errorf("sport %q sub_types[%d]: missing sub_type", s.SportType, j)
			}
			if _, ok := subTypes[st.SubType]; ok {
				return fmt.Errorf("sport %q: duplicate sub_type %q", s.SportType, st.SubType)
			}
			subTypes[st.SubType] = struct{}{}
			if st.Labels["de"] == "" {
				return fmt.Errorf("sport %q/%q: missing labels.de", s.SportType, st.SubType)
			}
			if st.Details["de"] == "" {
				return fmt.Errorf("sport %q/%q: missing details.de", s.SportType, st.SubType)
			}
			if st.AutoTitles["de"] == "" {
				return fmt.Errorf("sport %q/%q: missing auto_titles.de", s.SportType, st.SubType)
			}
			if st.MovingThreshold != nil && *st.MovingThreshold < 0 {
				return fmt.Errorf("sport %q/%q: negative moving_threshold", s.SportType, st.SubType)
			}
			if st.Selectable == nil {
				return fmt.Errorf("sport %q/%q: missing selectable default", s.SportType, st.SubType)
			}
			if err := validateModifierDefaults(s, st.ModifierDefaults, fmt.Sprintf("sport %q/%q", s.SportType, st.SubType)); err != nil {
				return err
			}
			if err := validateSubTypeTraits(s, st); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateModifierDefaults(s Sport, defaults *ModifierDefaults, context string) error {
	if defaults == nil {
		return nil
	}
	if defaults.IsIndoor == nil {
		return fmt.Errorf("%s: empty modifier_defaults", context)
	}
	if s.ModifierApplicability == nil || s.ModifierApplicability.IsIndoor == nil || !*s.ModifierApplicability.IsIndoor {
		return fmt.Errorf("%s: modifier_defaults.is_indoor requires modifier_applicability.is_indoor=true", context)
	}
	return nil
}

func validateSportTraits(s Sport) error {
	owner := fmt.Sprintf("sport %q", s.SportType)
	if err := validateTraits(owner, s.Traits); err != nil {
		return err
	}
	if s.Traits.ActivityForm == "mode" {
		return fmt.Errorf("%s: activity_form mode is only valid for subtypes", owner)
	}
	if s.Traits.ActivityForm == "fallback" && s.Lifecycle != "broad_fallback" {
		return fmt.Errorf("%s: activity_form fallback requires lifecycle broad_fallback", owner)
	}
	if s.Lifecycle == "broad_fallback" && s.Traits.ActivityForm != "fallback" {
		return fmt.Errorf("%s: lifecycle broad_fallback requires activity_form fallback", owner)
	}
	if s.Traits.ActivityForm == "fallback" && (s.Selectable == nil || *s.Selectable) {
		return fmt.Errorf("%s: activity_form fallback requires selectable false", owner)
	}
	if s.SportType == "generic" || s.SportType == "unknown" {
		if s.Traits.ActivityForm != "system" {
			return fmt.Errorf("%s: generic/unknown sport requires activity_form system", owner)
		}
		if !stringSlicesEqual(s.Traits.Environments, []string{"generic"}) {
			return fmt.Errorf("%s: generic/unknown sport requires environments [generic]", owner)
		}
	} else if s.Traits.ActivityForm == "system" {
		return fmt.Errorf("%s: activity_form system is only valid for generic and unknown", owner)
	}
	if err := validateGenericTraitSemantics(owner, s.Traits); err != nil {
		return err
	}
	return nil
}

func validateSubTypeTraits(parent Sport, subType SubType) error {
	owner := fmt.Sprintf("sport %q/%q", parent.SportType, subType.SubType)
	if subType.Traits != nil {
		if err := validateTraits(owner, *subType.Traits); err != nil {
			return err
		}
	}
	effective := EffectiveSubTypeTraits(parent, subType)
	if err := validateGenericTraitSemantics(owner, effective); err != nil {
		return err
	}
	switch effective.ActivityForm {
	case "fallback":
		return fmt.Errorf("%s: effective activity_form fallback is invalid for subtypes", owner)
	case "family":
		return fmt.Errorf("%s: effective activity_form family is invalid for subtypes", owner)
	case "mode":
		if !isAllowedModeSubType(parent.SportType, subType.SubType) {
			return fmt.Errorf("%s: activity_form mode is not allowed for this subtype", owner)
		}
	}
	return nil
}

func isAllowedModeSubType(sportType, subType string) bool {
	if sportType != "flying" {
		return false
	}
	switch subType {
	case "fly_vfr", "fly_ifr", "fly_navigate", "fly_timer", "fly_altimeter", "fly_wx":
		return true
	default:
		return false
	}
}

func validateTraits(owner string, traits Traits) error {
	if len(traits.Environments) == 0 {
		return fmt.Errorf("%s: missing traits.environments", owner)
	}
	if len(traits.MovementFamilies) == 0 {
		return fmt.Errorf("%s: missing traits.movement_families", owner)
	}
	if traits.ActivityForm == "" {
		return fmt.Errorf("%s: missing traits.activity_form", owner)
	}
	if len(traits.PropulsionModes) == 0 {
		return fmt.Errorf("%s: missing traits.propulsion_modes", owner)
	}
	if err := validateStringList(owner, "traits.environments", traits.Environments, TraitEnvironmentValues); err != nil {
		return err
	}
	if err := validateStringList(owner, "traits.movement_families", traits.MovementFamilies, TraitMovementFamilyValues); err != nil {
		return err
	}
	if err := validateStringList(owner, "traits.equipment_families", traits.EquipmentFamilies, TraitEquipmentFamilyValues); err != nil {
		return err
	}
	if !containsString(TraitActivityFormValues, traits.ActivityForm) {
		return fmt.Errorf("%s: invalid traits.activity_form %q", owner, traits.ActivityForm)
	}
	if err := validateStringList(owner, "traits.propulsion_modes", traits.PropulsionModes, TraitPropulsionModeValues); err != nil {
		return err
	}
	return nil
}

func validateStringList(owner, field string, values, allowed []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !containsString(allowed, value) {
			return fmt.Errorf("%s: invalid %s value %q", owner, field, value)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s: duplicate %s value %q", owner, field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateGenericTraitSemantics(owner string, traits Traits) error {
	if containsString(traits.Environments, "generic") && traits.ActivityForm != "system" {
		return fmt.Errorf("%s: environments [generic] requires activity_form system", owner)
	}
	if traits.ActivityForm == "system" && !stringSlicesEqual(traits.Environments, []string{"generic"}) {
		return fmt.Errorf("%s: activity_form system requires environments [generic]", owner)
	}
	if containsString(traits.PropulsionModes, "generic") {
		switch traits.ActivityForm {
		case "family", "fallback", "mode", "system":
		default:
			return fmt.Errorf("%s: propulsion_modes [generic] requires family, fallback, mode, or system activity_form", owner)
		}
	}
	return nil
}

func EffectiveSubTypeTraits(parent Sport, subType SubType) Traits {
	if subType.Traits != nil {
		return *subType.Traits
	}
	return parent.Traits
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isValidLifecycle(value string) bool {
	switch value {
	case "canonical", "broad_fallback", "deprecated":
		return true
	default:
		return false
	}
}

func isValidSegmentGroup(group string) bool {
	switch group {
	case "bike", "foot", "swim", "ski", "paddle", "generic":
		return true
	default:
		return false
	}
}
