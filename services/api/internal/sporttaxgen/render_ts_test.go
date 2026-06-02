package sporttaxgen

import (
	"strings"
	"testing"
)

func TestRenderTSEmitsTopLevelAutoTitleAndMovingThreshold(t *testing.T) {
	selectable := true
	taxonomy := Taxonomy{
		Categories: []Category{{
			Key:    "run",
			Labels: LocaleMap{"de": "Laufen", "en": "Runs"},
			Color:  Color{Token: "run", LightHex: "#EF4444"},
		}},
		Sports: []Sport{{
			SportType:       "running",
			CategoryKey:     "run",
			Labels:          LocaleMap{"de": "Laufen", "en": "Running"},
			AutoTitles:      LocaleMap{"de": "Lauf", "en": "Run"},
			SegmentGroup:    "foot",
			TypicallyHasGPS: true,
			MovingThreshold: 0.5,
			Lifecycle:       "canonical",
			Selectable:      &selectable,
			Traits: Traits{
				Environments:      []string{"land"},
				MovementFamilies:  []string{"run"},
				EquipmentFamilies: []string{},
				ActivityForm:      "single",
				PropulsionModes:   []string{"human"},
			},
		}},
	}

	modifiers, err := LoadModifiers(strings.NewReader(validModifierRegistryFixture()))
	if err != nil {
		t.Fatalf("LoadModifiers() error = %v", err)
	}
	rendered, err := RenderTS(taxonomy, []byte("source"), modifiers)
	if err != nil {
		t.Fatalf("RenderTS() error = %v", err)
	}
	got := string(rendered)
	for _, want := range []string{
		"labels: { de: 'Laufen', en: 'Running' }",
		"autoTitles: { de: 'Lauf', en: 'Run' }",
		"label: 'Laufen'",
		"autoTitle: 'Lauf'",
		"movingThreshold: 0.5",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTS() missing %q:\n%s", want, got)
		}
	}
}

func TestTSStringEscapesControlCharacters(t *testing.T) {
	got := tsString("a\tb\rc\nd\u2028e\u2029f\\g'h")
	want := `'a\tb\rc\nd\u2028e\u2029f\\g\'h'`
	if got != want {
		t.Fatalf("tsString() = %q, want %q", got, want)
	}
}

func TestRenderTSModifiersEmitsMetadata(t *testing.T) {
	registry, err := LoadModifiers(strings.NewReader(validModifierRegistryFixture()))
	if err != nil {
		t.Fatalf("LoadModifiers() error = %v", err)
	}
	rendered, err := RenderTSModifiers(registry)
	if err != nil {
		t.Fatalf("RenderTSModifiers() error = %v", err)
	}
	got := string(rendered)
	for _, want := range []string{
		"GENERATED_SPORT_MODIFIERS",
		"export type SportModifierKey = typeof GENERATED_SPORT_MODIFIER_KEYS[number]",
		"labels: { de: 'E-Antrieb', en: 'Electric assist', es: 'Asistencia eléctrica', fr: 'Assistance électrique', it: 'Assistenza elettrica' }",
		"label: 'E-Antrieb'",
		"actionName: 'set_electric'",
		"actionParam: 'electric'",
		"shadowFSToken: 'electric'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTSModifiers() missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTSGearDefaultProjectionEmitsLocalizedLabels(t *testing.T) {
	rendered, err := RenderTSGearDefaultProjection(GearDefaultProjection{
		ManualOptions: []GearDefaultSportOption{{
			SportType: "running",
			LabelDE:   "Laufen",
			Labels:    LocaleMap{"de": "Laufen", "en": "Running"},
		}},
	})
	if err != nil {
		t.Fatalf("RenderTSGearDefaultProjection() error = %v", err)
	}
	got := string(rendered)
	for _, want := range []string{
		"value: 'running'",
		"label: 'Laufen'",
		"labels: { de: 'Laufen', en: 'Running' }",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTSGearDefaultProjection() missing %q:\n%s", want, got)
		}
	}
}
