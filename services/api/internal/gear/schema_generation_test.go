package gear_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

// repoRoot walks up from the test file's package directory to find the repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "config", "sport-taxonomy", "sports.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

// loadTestSchema loads and validates config/gear-schema.yaml from the repo root.
func loadTestSchema(t *testing.T) *geargen.Schema {
	t.Helper()
	root := repoRoot(t)
	schemaPath := filepath.Join(root, "config", "gear-schema.yaml")
	taxonomyDir := filepath.Join(root, "config", "sport-taxonomy")
	modifiersPath := filepath.Join(root, "config", "sport-taxonomy", "modifiers.yaml")

	s, err := geargen.Load(schemaPath)
	if err != nil {
		t.Fatalf("Load gear schema: %v", err)
	}
	if err := geargen.Validate(s, taxonomyDir, modifiersPath); err != nil {
		t.Fatalf("Validate gear schema: %v", err)
	}
	return s
}

// gearTypeKeys returns a set of all gear type keys present in the schema.
func gearTypeKeys(s *geargen.Schema) map[string]struct{} {
	m := make(map[string]struct{}, len(s.GearTypes))
	for _, gt := range s.GearTypes {
		m[gt.Key] = struct{}{}
	}
	return m
}

func assertGearTypeExists(t *testing.T, keys map[string]struct{}, key string) {
	t.Helper()
	if _, ok := keys[key]; !ok {
		t.Errorf("gear_type %q not found in schema", key)
	}
}

func assertFiveLocaleLabels(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for _, locale := range []string{"de", "en", "fr", "it", "es"} {
		if got[locale] != want[locale] {
			t.Errorf("labels[%q] = %q, want %q", locale, got[locale], want[locale])
		}
	}
}

func TestGearSchemaAllLabelsHaveFiveLocales(t *testing.T) {
	s := loadTestSchema(t)

	var missing []string
	check := func(path string, labels map[string]string) {
		for _, locale := range []string{"de", "en", "fr", "it", "es"} {
			if labels[locale] == "" {
				missing = append(missing, path+" labels."+locale)
			}
		}
	}
	var checkAttribute func(path string, attr geargen.Attribute)
	checkAttribute = func(path string, attr geargen.Attribute) {
		check(path, attr.Labels)
		for _, sub := range attr.Subfields {
			checkAttribute(path+".subfields["+sub.Key+"]", sub)
		}
	}

	for _, role := range s.AssignmentRoles {
		check("assignment_roles["+role.Key+"]", role.Labels)
	}
	for _, set := range s.OptionSets {
		for _, option := range set.Options {
			check("option_sets["+set.Key+"].options["+option.Key+"]", option.Labels)
		}
	}
	for _, group := range s.UnitGroups {
		check("unit_groups["+group.Key+"]", group.Labels)
	}
	for _, attr := range s.Attributes {
		checkAttribute("attributes["+attr.Key+"]", attr)
	}
	for _, gearType := range s.GearTypes {
		check("gear_types["+gearType.Key+"]", gearType.Labels)
		for _, slot := range gearType.Slots {
			check("gear_types["+gearType.Key+"].slots["+slot.Key+"]", slot.Labels)
		}
	}
	for _, componentType := range s.ComponentTypes {
		check("component_types["+componentType.Key+"]", componentType.Labels)
	}

	if len(missing) > 0 {
		t.Fatalf("gear schema labels missing %d locale values:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func TestGearSchemaRejectsKnownBadLocalizedLabels(t *testing.T) {
	s := loadTestSchema(t)

	knownBad := map[string]map[string]struct{}{
		"en": {
			"Cantilevers":             {},
			"Dropable":                {},
			"Flip flop":               {},
			"Trommelbremse":           {},
			"Vollgummi":               {},
			"Gummi":                   {},
			"Trekkingstiefel":         {},
			"Schwere Trekkingstiefel": {},
			"Snowboard-Boots":         {},
			"Sattelstützenklemmung":   {},
			"Rahmengröße cm":          {},
			"Dropper-Betätigung":      {},
			"display":                 {},
		},
		"fr": {
			"Porte-à-faux":            {},
			"Dropable":                {},
			"Porte-voix":              {},
			"Bascule":                 {},
			"Trommelbremse":           {},
			"Vollgummi":               {},
			"Gummi":                   {},
			"Trekkingstiefel":         {},
			"Schwere Trekkingstiefel": {},
			"Snowboard-Boots":         {},
			"Sattelstützenklemmung":   {},
			"Rahmengröße cm":          {},
			"Dropper-Betätigung":      {},
			"afficher":                {},
			"Filature":                {},
		},
		"it": {
			"Rilasciabile":            {},
			"Altoparlante":            {},
			"Infradito":               {},
			"Trommelbremse":           {},
			"Vollgummi":               {},
			"Gummi":                   {},
			"Trekkingstiefel":         {},
			"Schwere Trekkingstiefel": {},
			"Snowboard-Boots":         {},
			"Sattelstützenklemmung":   {},
			"Rahmengröße cm":          {},
			"Dropper-Betätigung":      {},
			"visualizzazione":         {},
			"Filatura":                {},
		},
		"es": {
			"Voladizos":               {},
			"Dropable":                {},
			"Megáfono":                {},
			"chanclas":                {},
			"Trommelbremse":           {},
			"Vollgummi":               {},
			"Gummi":                   {},
			"Trekkingstiefel":         {},
			"Schwere Trekkingstiefel": {},
			"Snowboard-Boots":         {},
			"Sattelstützenklemmung":   {},
			"Rahmengröße cm":          {},
			"Dropper-Betätigung":      {},
			"mostrar":                 {},
			"girando":                 {},
		},
	}
	knownBadAllLocales := map[string]struct{}{
		"Gurtzgröße": {},
	}

	var failures []string
	check := func(path string, labels map[string]string) {
		for locale, value := range labels {
			if _, ok := knownBadAllLocales[value]; ok {
				failures = append(failures, path+" labels."+locale+" = "+value)
			}
		}
		for locale, blocked := range knownBad {
			if _, ok := blocked[labels[locale]]; ok {
				failures = append(failures, path+" labels."+locale+" = "+labels[locale])
			}
		}
	}
	var checkAttribute func(path string, attr geargen.Attribute)
	checkAttribute = func(path string, attr geargen.Attribute) {
		check(path, attr.Labels)
		for _, sub := range attr.Subfields {
			checkAttribute(path+".subfields["+sub.Key+"]", sub)
		}
	}

	for _, role := range s.AssignmentRoles {
		check("assignment_roles["+role.Key+"]", role.Labels)
	}
	for _, set := range s.OptionSets {
		for _, option := range set.Options {
			check("option_sets["+set.Key+"].options["+option.Key+"]", option.Labels)
		}
	}
	for _, group := range s.UnitGroups {
		check("unit_groups["+group.Key+"]", group.Labels)
	}
	for _, attr := range s.Attributes {
		checkAttribute("attributes["+attr.Key+"]", attr)
	}
	for _, gearType := range s.GearTypes {
		check("gear_types["+gearType.Key+"]", gearType.Labels)
		for _, slot := range gearType.Slots {
			check("gear_types["+gearType.Key+"].slots["+slot.Key+"]", slot.Labels)
		}
	}
	for _, componentType := range s.ComponentTypes {
		check("component_types["+componentType.Key+"]", componentType.Labels)
	}

	if len(failures) > 0 {
		t.Fatalf("gear schema contains known bad localized labels:\n%s", strings.Join(failures, "\n"))
	}
}

func generatedAttributeLabels(t *testing.T, key string) map[string]string {
	t.Helper()
	attr, ok := gearpkg.LookupAttribute(key)
	if !ok {
		t.Fatalf("generated attribute %q missing", key)
	}
	return attr.Labels
}

func generatedOptionLabels(t *testing.T, optionSetKey, optionKey string) map[string]string {
	t.Helper()
	optionSet, ok := gearpkg.LookupOptionSet(optionSetKey)
	if !ok {
		t.Fatalf("generated option_set %q missing", optionSetKey)
	}
	for _, opt := range optionSet.Options {
		if opt.Key == optionKey {
			return opt.Labels
		}
	}
	t.Fatalf("generated option %q missing in option_set %q", optionKey, optionSetKey)
	return nil
}

func generatedGearTypeLabels(t *testing.T, key string) map[string]string {
	t.Helper()
	gearType, ok := gearpkg.LookupGearType(key)
	if !ok {
		t.Fatalf("generated gear_type %q missing", key)
	}
	return gearType.Labels
}

func generatedComponentTypeLabels(t *testing.T, key string) map[string]string {
	t.Helper()
	componentType, ok := gearpkg.LookupComponentType(key)
	if !ok {
		t.Fatalf("generated component_type %q missing", key)
	}
	return componentType.Labels
}

func generatedSlotLabels(t *testing.T, gearTypeKey, slotKey string) map[string]string {
	t.Helper()
	gearType, ok := gearpkg.LookupGearType(gearTypeKey)
	if !ok {
		t.Fatalf("generated gear_type %q missing", gearTypeKey)
	}
	for _, slot := range gearType.Slots {
		if slot.Key == slotKey {
			return slot.Labels
		}
	}
	t.Fatalf("generated slot %q missing in gear_type %q", slotKey, gearTypeKey)
	return nil
}

func TestGeneratedSchemaIncludesRepresentativeLocalizedLabels(t *testing.T) {
	if len(gearpkg.Schema.AssignmentRoles) == 0 {
		t.Fatal("generated Schema.AssignmentRoles is empty")
	}
	assertFiveLocaleLabels(t, gearpkg.Schema.AssignmentRoles[0].Labels, map[string]string{
		"de": "Hauptausrüstung",
		"en": "Primary gear",
		"fr": "Équipement principal",
		"it": "Attrezzatura principale",
		"es": "Equipamiento principal",
	})

	yesNo, ok := gearpkg.LookupOptionSet("yes_no")
	if !ok || len(yesNo.Options) == 0 {
		t.Fatal("generated option_set yes_no missing or empty")
	}
	assertFiveLocaleLabels(t, yesNo.Options[0].Labels, map[string]string{
		"de": "Ja",
		"en": "Yes",
		"fr": "Oui",
		"it": "Sì",
		"es": "Sí",
	})

	bike, ok := gearpkg.LookupGearType("bike")
	if !ok {
		t.Fatal("generated gear_type bike missing")
	}
	assertFiveLocaleLabels(t, bike.Labels, map[string]string{
		"de": "Fahrrad",
		"en": "Bike",
		"fr": "Vélo",
		"it": "Bici",
		"es": "Bicicleta",
	})

	frame, ok := gearpkg.LookupComponentType("bike.frame")
	if !ok {
		t.Fatal("generated component_type bike.frame missing")
	}
	assertFiveLocaleLabels(t, frame.Labels, map[string]string{
		"de": "Rahmen",
		"en": "Frame",
		"fr": "Cadre",
		"it": "Telaio",
		"es": "Cuadro",
	})
}

func TestGeneratedSchemaIncludesLocalizedLabelsForBikeRunServiceGaps(t *testing.T) {
	assertFiveLocaleLabels(t, generatedAttributeLabels(t, "bike.wheel_size"), map[string]string{
		"de": "Laufradgröße",
		"en": "Wheel size",
		"fr": "Taille de roue",
		"it": "Misura ruota",
		"es": "Tamaño de rueda",
	})
	assertFiveLocaleLabels(t, generatedAttributeLabels(t, "bike.cable_routing"), map[string]string{
		"de": "Kabelführung",
		"en": "Cable routing",
		"fr": "Passage des câbles",
		"it": "Passaggio cavi",
		"es": "Guiado de cables",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.fender_presence", "both"), map[string]string{
		"de": "Vorne und hinten",
		"en": "Front and rear",
		"fr": "Avant et arrière",
		"it": "Anteriore e posteriore",
		"es": "Delantero y trasero",
	})
	assertFiveLocaleLabels(t, generatedComponentTypeLabels(t, "bike.chain"), map[string]string{
		"de": "Kette",
		"en": "Chain",
		"fr": "Chaîne",
		"it": "Catena",
		"es": "Cadena",
	})
	assertFiveLocaleLabels(t, generatedSlotLabels(t, "bike.mtb", "shock"), map[string]string{
		"de": "Dämpfer",
		"en": "Shock",
		"fr": "Amortisseur",
		"it": "Ammortizzatore",
		"es": "Amortiguador",
	})
	assertFiveLocaleLabels(t, generatedGearTypeLabels(t, "shoes.running"), map[string]string{
		"de": "Laufschuhe",
		"en": "Running shoes",
		"fr": "Chaussures de running",
		"it": "Scarpe da running",
		"es": "Zapatillas de running",
	})
	assertFiveLocaleLabels(t, generatedGearTypeLabels(t, "bike.consumable.chain_lube"), map[string]string{
		"de": "Kettenschmiermittel",
		"en": "Chain lubricant",
		"fr": "Lubrifiant de chaîne",
		"it": "Lubrificante catena",
		"es": "Lubricante de cadena",
	})
}

func TestGeneratedSchemaIncludesLocalizedLabelsForCommonGearDetailGaps(t *testing.T) {
	assertFiveLocaleLabels(t, generatedAttributeLabels(t, "bike.rear_axle_standard"), map[string]string{
		"de": "Hinterbau-Achsstandard",
		"en": "Rear axle standard",
		"fr": "Standard d'axe arrière",
		"it": "Standard asse posteriore",
		"es": "Estándar de eje trasero",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "material.component", "plastic"), map[string]string{
		"de": "Kunststoff",
		"en": "Plastic",
		"fr": "Plastique",
		"it": "Plastica",
		"es": "Plástico",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.frame_type", "bike.city.dutch"), map[string]string{
		"de": "Hollandrad",
		"en": "Dutch bike",
		"fr": "Vélo hollandais",
		"it": "Bici olandese",
		"es": "Bicicleta holandesa",
	})
	assertFiveLocaleLabels(t, generatedComponentTypeLabels(t, "bike.stem"), map[string]string{
		"de": "Vorbau",
		"en": "Stem",
		"fr": "Potence",
		"it": "Attacco manubrio",
		"es": "Potencia",
	})
	assertFiveLocaleLabels(t, generatedSlotLabels(t, "bike.gravel", "handlebar"), map[string]string{
		"de": "Lenker",
		"en": "Handlebar",
		"fr": "Cintre",
		"it": "Manubrio",
		"es": "Manillar",
	})
}

func TestGeneratedSchemaIncludesLocalizedLabelsForTranslationTraps(t *testing.T) {
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.brake_type", "cantilever"), map[string]string{
		"de": "Cantilever",
		"en": "Cantilever brake",
		"fr": "Frein cantilever",
		"it": "Freno cantilever",
		"es": "Freno cantilever",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.brake_type", "drum"), map[string]string{
		"de": "Trommelbremse",
		"en": "Drum brake",
		"fr": "Frein à tambour",
		"it": "Freno a tamburo",
		"es": "Freno de tambor",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.handlebar_type", "drop"), map[string]string{
		"de": "Dropbar",
		"en": "Drop bar",
		"fr": "Cintre route",
		"it": "Manubrio drop",
		"es": "Manillar drop",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.handlebar_type", "bullhorn"), map[string]string{
		"de": "Bullhorn",
		"en": "Bullhorn bar",
		"fr": "Cintre bullhorn",
		"it": "Manubrio bullhorn",
		"es": "Manillar bullhorn",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.tire_setup", "solid"), map[string]string{
		"de": "Vollgummi",
		"en": "Solid tire",
		"fr": "Pneu plein",
		"it": "Pneumatico pieno",
		"es": "Neumático macizo",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "bike.hub_type", "flip_flop"), map[string]string{
		"de": "Flip-Flop",
		"en": "Flip-flop hub",
		"fr": "Moyeu flip-flop",
		"it": "Mozzo flip-flop",
		"es": "Buje flip-flop",
	})
	assertFiveLocaleLabels(t, generatedOptionLabels(t, "ebike.battery_role", "display"), map[string]string{
		"de": "Display",
		"en": "Display",
		"fr": "Display",
		"it": "Display",
		"es": "Display",
	})
	assertFiveLocaleLabels(t, generatedGearTypeLabels(t, "bike.indoor.spinning"), map[string]string{
		"de": "Spinning",
		"en": "Spinning",
		"fr": "Spinning",
		"it": "Spinning",
		"es": "Spinning",
	})
	assertFiveLocaleLabels(t, generatedGearTypeLabels(t, "shoes.trekking"), map[string]string{
		"de": "Trekkingstiefel",
		"en": "Trekking boots",
		"fr": "Chaussures de trekking",
		"it": "Scarponi da trekking",
		"es": "Botas de trekking",
	})
	assertFiveLocaleLabels(t, generatedGearTypeLabels(t, "snowboard_boots"), map[string]string{
		"de": "Snowboard-Boots",
		"en": "Snowboard boots",
		"fr": "Boots de snowboard",
		"it": "Scarponi da snowboard",
		"es": "Botas de snowboard",
	})
	assertFiveLocaleLabels(t, generatedComponentTypeLabels(t, "bike.display"), map[string]string{
		"de": "Display",
		"en": "Display",
		"fr": "Display",
		"it": "Display",
		"es": "Display",
	})
}

// TestBikeHierarchyComplete verifies the full bike hierarchy including
// cyclocross and handcycle are present.
func TestBikeHierarchyComplete(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"bike",
		"bike.road",
		"bike.mtb",
		"bike.gravel",
		"bike.gravel.cyclocross",
		"bike.city",
		"bike.trekking",
		"bike.cargo",
		"bike.tandem",
		"bike.recumbent",
		"bike.kids",
		"bike.balance",
		"bike.indoor",
		"bike.handcycle",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestHelmetConsolidatedAsSingleType verifies helmet is a single sport-orthogonal
// type with subcategory and safety_standard attributes.
func TestHelmetConsolidatedAsSingleType(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	// Only one helmet type (not helmet.bike, helmet.ski, etc.)
	assertGearTypeExists(t, keys, "helmet")
	for k := range keys {
		if strings.HasPrefix(k, "helmet.") {
			t.Errorf("unexpected helmet subtype %q: helmet must be a single consolidated type", k)
		}
	}

	// Helmet must carry subcategory and safety_standard attribute keys.
	var helmetType *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "helmet" {
			helmetType = &s.GearTypes[i]
			break
		}
	}
	if helmetType == nil {
		t.Fatal("helmet gear type not found")
	}

	attrSet := make(map[string]struct{}, len(helmetType.AttributeKeys))
	for _, ak := range helmetType.AttributeKeys {
		attrSet[ak] = struct{}{}
	}
	for _, required := range []string{"helmet.subcategory", "helmet.safety_standard"} {
		if _, ok := attrSet[required]; !ok {
			t.Errorf("helmet missing attribute_key %q", required)
		}
	}

	// helmet.subcategory must be a search_enum option set with the required values.
	var subcatAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "helmet.subcategory" {
			subcatAttr = &s.Attributes[i]
			break
		}
	}
	if subcatAttr == nil {
		t.Fatal("attribute helmet.subcategory not found")
	}
	if subcatAttr.ValueType != "search_enum" {
		t.Errorf("helmet.subcategory value_type = %q, want search_enum", subcatAttr.ValueType)
	}

	// Locate the option set and verify required options.
	var optSet *geargen.OptionSet
	for i := range s.OptionSets {
		if s.OptionSets[i].Key == subcatAttr.OptionSetKey {
			optSet = &s.OptionSets[i]
			break
		}
	}
	if optSet == nil {
		t.Fatalf("option_set %q not found", subcatAttr.OptionSetKey)
	}
	optKeys := make(map[string]struct{}, len(optSet.Options))
	for _, o := range optSet.Options {
		optKeys[o.Key] = struct{}{}
	}
	for _, required := range []string{"road_bike", "mtb", "full_face_dh", "ski", "climb", "equestrian"} {
		if _, ok := optKeys[required]; !ok {
			t.Errorf("helmet.subcategory option_set missing key %q", required)
		}
	}

	// helmet.safety_standard must be enum_with_freetext.
	var safetyAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "helmet.safety_standard" {
			safetyAttr = &s.Attributes[i]
			break
		}
	}
	if safetyAttr == nil {
		t.Fatal("attribute helmet.safety_standard not found")
	}
	if safetyAttr.ValueType != "enum_with_freetext" {
		t.Errorf("helmet.safety_standard value_type = %q, want enum_with_freetext", safetyAttr.ValueType)
	}
}

// TestGogglesConsolidated verifies goggles is a single sport-orthogonal type
// with a subcategory attribute.
func TestGogglesConsolidated(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	assertGearTypeExists(t, keys, "goggles")
	for k := range keys {
		if strings.HasPrefix(k, "goggles.") {
			t.Errorf("unexpected goggles subtype %q: goggles must be a single consolidated type", k)
		}
	}

	// goggles must carry goggles.subcategory attribute.
	var gogglesType *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "goggles" {
			gogglesType = &s.GearTypes[i]
			break
		}
	}
	if gogglesType == nil {
		t.Fatal("goggles gear type not found")
	}

	found := false
	for _, ak := range gogglesType.AttributeKeys {
		if ak == "goggles.subcategory" {
			found = true
		}
	}
	if !found {
		t.Error("goggles missing attribute_key goggles.subcategory")
	}

	// goggles.subcategory option set must include ski, swim, mx, dh, sun.
	var subcatAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "goggles.subcategory" {
			subcatAttr = &s.Attributes[i]
			break
		}
	}
	if subcatAttr == nil {
		t.Fatal("attribute goggles.subcategory not found")
	}
	var optSet *geargen.OptionSet
	for i := range s.OptionSets {
		if s.OptionSets[i].Key == subcatAttr.OptionSetKey {
			optSet = &s.OptionSets[i]
			break
		}
	}
	if optSet == nil {
		t.Fatalf("option_set %q not found", subcatAttr.OptionSetKey)
	}
	optKeys := make(map[string]struct{}, len(optSet.Options))
	for _, o := range optSet.Options {
		optKeys[o.Key] = struct{}{}
	}
	for _, required := range []string{"ski", "swim", "mx", "dh", "sun"} {
		if _, ok := optKeys[required]; !ok {
			t.Errorf("goggles.subcategory option_set missing key %q", required)
		}
	}
}

// TestShoesHierarchy verifies the shoes hierarchy is present.
func TestShoesHierarchy(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"shoes",
		"shoes.cycling",
		"shoes.cycling.gravel",
		"shoes.running",
		"shoes.hiking",
		"shoes.ski",
		"shoes.climbing",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestConsumableMaterials verifies bike.consumable component types exist
// and use the consumable assignment role.
func TestConsumableMaterials(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"bike.consumable.chain_lube",
		"bike.consumable.brake_pads",
		"bike.consumable.tire_sealant",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}

	// Each consumable must have default_assignment_role_key == "consumable".
	consumableRoleFound := false
	for _, r := range s.AssignmentRoles {
		if r.Key == "consumable" {
			consumableRoleFound = true
		}
	}
	if !consumableRoleFound {
		t.Error("assignment_role consumable not found")
	}

	for _, key := range required {
		for _, gt := range s.GearTypes {
			if gt.Key == key {
				if gt.DefaultAssignmentRoleKey != "consumable" {
					t.Errorf("gear_type %q default_assignment_role_key = %q, want consumable", key, gt.DefaultAssignmentRoleKey)
				}
			}
		}
	}
}

// TestClimbScopeContainsAllExpectedTypes verifies the climb scope gear types are present.
func TestClimbScopeContainsAllExpectedTypes(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"climb.harness",
		"climb.rope",
		"climb.crampons",
		"climb.ice_axe",
		"climb.belay_device",
		"climb.quickdraw",
		"climb.chalkbag",
		"climb.crashpad",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestApparelScopeContainsBaseTypes verifies apparel scope gear types are present
// and that each has a subcategory attribute.
func TestApparelScopeContainsBaseTypes(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"apparel.pants",
		"apparel.jersey",
		"apparel.gloves",
		"apparel.socks",
		"apparel.jacket",
		"apparel.shorts",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}

	// Each apparel type must have apparel.subcategory in its attribute_keys.
	attrKeySet := make(map[string]bool)
	for _, a := range s.Attributes {
		attrKeySet[a.Key] = true
	}
	if !attrKeySet["apparel.subcategory"] {
		t.Error("attribute apparel.subcategory not found in schema attributes")
	}

	typesByKey := make(map[string]*geargen.GearType, len(s.GearTypes))
	for i := range s.GearTypes {
		typesByKey[s.GearTypes[i].Key] = &s.GearTypes[i]
	}
	for _, key := range required {
		gt, ok := typesByKey[key]
		if !ok {
			continue // already reported above
		}
		found := false
		for _, ak := range gt.AttributeKeys {
			if ak == "apparel.subcategory" {
				found = true
			}
		}
		if !found {
			t.Errorf("gear_type %q missing attribute_key apparel.subcategory", key)
		}
	}
}

// TestStep1WorkedExampleEnduroRecommended verifies that the Step 1 worked
// example for soft per-gear_type defaults is present on bike.mtb.enduro and
// references the discrete attribute-value lists declared in gear-schema.yaml.
// Step 2X will fill the remaining bike subtypes; this test guards the shape
// only, not the breadth.
func TestStep1WorkedExampleEnduroRecommended(t *testing.T) {
	s := loadTestSchema(t)
	var enduro *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "bike.mtb.enduro" {
			enduro = &s.GearTypes[i]
			break
		}
	}
	if enduro == nil {
		t.Fatal("bike.mtb.enduro gear_type not found")
	}
	if enduro.Recommended == nil {
		t.Fatal("bike.mtb.enduro.Recommended is nil — worked example missing")
	}
	if len(enduro.Recommended.Attributes) < 1 {
		t.Fatalf("bike.mtb.enduro.Recommended.Attributes is empty, want >=1")
	}
	wantKeys := map[string]bool{"bike.frame_design": false, "bike.wheel_size": false}
	for _, ra := range enduro.Recommended.Attributes {
		if _, ok := wantKeys[ra.Key]; ok {
			wantKeys[ra.Key] = true
			if len(ra.Values) == 0 {
				t.Errorf("Recommended attribute %q has empty Values", ra.Key)
			}
		}
	}
	for k, seen := range wantKeys {
		if !seen {
			t.Errorf("Recommended attribute %q missing from bike.mtb.enduro", k)
		}
	}
}

// TestStep1WorkedExampleShockConditions verifies the worked-example
// applies_to_gear_when on bike.shock_eye_to_eye_mm and visible_when on
// bike.shock_stroke_mm parse and round-trip through the loader. Soft DSL
// only — no runtime enforcement is asserted.
func TestStep1WorkedExampleShockConditions(t *testing.T) {
	s := loadTestSchema(t)
	var eye, stroke *geargen.Attribute
	for i := range s.Attributes {
		switch s.Attributes[i].Key {
		case "bike.shock_eye_to_eye_mm":
			eye = &s.Attributes[i]
		case "bike.shock_stroke_mm":
			stroke = &s.Attributes[i]
		}
	}
	if eye == nil || stroke == nil {
		t.Fatal("shock attributes not found")
	}
	if len(eye.AppliesToGearWhen) != 1 {
		t.Fatalf("bike.shock_eye_to_eye_mm.AppliesToGearWhen len = %d, want 1", len(eye.AppliesToGearWhen))
	}
	c := eye.AppliesToGearWhen[0]
	if c.Ref != "gear.bike.frame_design" || c.Op != "eq" || c.Value != "full_suspension" {
		t.Errorf("AppliesToGearWhen[0] = %+v, want gear.bike.frame_design eq full_suspension", c)
	}
	if len(stroke.VisibleWhen) != 1 {
		t.Fatalf("bike.shock_stroke_mm.VisibleWhen len = %d, want 1", len(stroke.VisibleWhen))
	}
	if c := stroke.VisibleWhen[0]; c.Ref != "bike.shock_eye_to_eye_mm" || c.Op != "is_set" {
		t.Errorf("VisibleWhen[0] = %+v, want bike.shock_eye_to_eye_mm is_set", c)
	}
}

// TestGearSchemaGeneratedGoFileMatchesYAML verifies schema_generated.go is in sync
// with config/gear-schema.yaml.
func TestGearSchemaGeneratedGoFileMatchesYAML(t *testing.T) {
	root := repoRoot(t)
	schemaPath := filepath.Join(root, "config", "gear-schema.yaml")

	s, err := geargen.Load(schemaPath)
	if err != nil {
		t.Fatalf("Load gear schema: %v", err)
	}

	want, err := geargen.RenderGo(s)
	if err != nil {
		t.Fatalf("RenderGo: %v", err)
	}

	generatedPath := filepath.Join(root, "services", "api", "internal", "gear", "schema_generated.go")
	got, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read schema_generated.go: %v", err)
	}

	if !bytesEqual(got, want) {
		line, gotLine, wantLine := firstDiffLineBytes(got, want)
		t.Fatalf(
			"services/api/internal/gear/schema_generated.go is out of sync with config/gear-schema.yaml.\n"+
				"Run ./scripts/generate-gear-schema.sh and commit the result.\n"+
				"First diff at line %d: expected %q, got %q.",
			line, wantLine, gotLine,
		)
	}
}

// TestGearSchemaGeneratedGoFileHeaderIsCanonical verifies the generated file
// starts with the canonical codegen header comment.
func TestGearSchemaGeneratedGoFileHeaderIsCanonical(t *testing.T) {
	root := repoRoot(t)
	generatedPath := filepath.Join(root, "services", "api", "internal", "gear", "schema_generated.go")
	got, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read schema_generated.go: %v", err)
	}
	const wantHeader = "// Code generated by geargen; DO NOT EDIT."
	if !strings.HasPrefix(string(got), wantHeader) {
		t.Errorf("schema_generated.go does not start with canonical header %q", wantHeader)
	}
}

func bytesEqual(a, b []byte) bool {
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

func firstDiffLineBytes(got, want []byte) (line int, gotLine, wantLine string) {
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			return i + 1, gotLines[i], wantLines[i]
		}
	}
	if len(gotLines) != len(wantLines) {
		return min(len(gotLines), len(wantLines)) + 1, "(EOF)", "(EOF)"
	}
	return 0, "", ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
