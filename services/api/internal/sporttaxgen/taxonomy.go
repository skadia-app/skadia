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
	SubTypes              []SubType              `yaml:"sub_types"`
}

type ModifierApplicability struct {
	IsElectric *bool `yaml:"is_electric"`
	IsIndoor   *bool `yaml:"is_indoor"`
	IsVirtual  *bool `yaml:"is_virtual"`
	IsCommute  *bool `yaml:"is_commute"`
}

type SubType struct {
	SubType         string    `yaml:"sub_type"`
	Labels          LocaleMap `yaml:"labels"`
	Details         LocaleMap `yaml:"details"`
	AutoTitles      LocaleMap `yaml:"auto_titles"`
	MovingThreshold *float64  `yaml:"moving_threshold"`
	Selectable      *bool     `yaml:"selectable"`
}

var validHex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

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
		}
	}
	return nil
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
