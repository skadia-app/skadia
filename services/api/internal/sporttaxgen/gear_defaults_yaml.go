package sporttaxgen

// gearDefaultsPredicateYAML is the YAML wire shape for a single predicate
// entry inside applicable_sport_predicates. gopkg.in/yaml.v3 does not honour
// json struct tags, so explicit yaml tags are needed (sport_type / sub_type).
type gearDefaultsPredicateYAML struct {
	SportType string  `yaml:"sport_type"`
	SubType   *string `yaml:"sub_type"`
}

func (p gearDefaultsPredicateYAML) ToSportPredicateEntry() SportPredicateEntry {
	return SportPredicateEntry{SportType: p.SportType, SubType: p.SubType}
}

// gearDefaultsPredicateEntryYAML is the YAML wire shape for one gear-type
// block inside applicable_sport_predicates.
type gearDefaultsPredicateEntryYAML struct {
	GearTypeKey string                      `yaml:"gear_type_key"`
	Predicates  []gearDefaultsPredicateYAML `yaml:"predicates"`
}
