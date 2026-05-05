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
			Labels: LocaleMap{"de": "Laufen"},
			Color:  Color{Token: "run", LightHex: "#EF4444"},
		}},
		Sports: []Sport{{
			SportType:       "running",
			CategoryKey:     "run",
			Labels:          LocaleMap{"de": "Laufen"},
			AutoTitles:      LocaleMap{"de": "Lauf"},
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

	rendered, err := RenderTS(taxonomy, []byte("source"))
	if err != nil {
		t.Fatalf("RenderTS() error = %v", err)
	}
	got := string(rendered)
	for _, want := range []string{"autoTitle: 'Lauf'", "movingThreshold: 0.5"} {
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
