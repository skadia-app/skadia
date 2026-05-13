package agent

import (
	"strings"
	"testing"
	"time"
)

func TestAssembleSystemPromptContainsBasePromptSkillsAndFewShots(t *testing.T) {
	skills, err := LoadSkills("")
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	profile := GetProfile("small", "", nil)

	prompt := AssembleSystemPrompt(profile, []*Skill{skills["gear"], skills["activities"]}, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"Du bist ein Sport-Datenassistent. Heute ist 08.03.2026.",
		"## Kontext: Ausrüstung, Verschleiß, km-Stand, Wartung, Kosten",
		"## Werkzeug-Auswahl",
		"Subjektive Routing-Wünsche",
		"list_routing_profiles",
		"## Beispiele",
		"→ Schritt 1: gear_list",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q\n%s", want, prompt)
		}
	}
}

func TestAssembleSystemPromptOmitsToolGuideAndFewShotsForLargeProfile(t *testing.T) {
	profile := GetProfile("large", "", nil)
	prompt := AssembleSystemPrompt(profile, nil, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC))

	if !strings.Contains(prompt, "Grundlagenausdauer") {
		t.Fatalf("large prompt missing rich section")
	}
	if strings.Contains(prompt, "Werkzeug-Auswahl") {
		t.Fatalf("large prompt contains tool guide")
	}
	if strings.Contains(prompt, "## Beispiele") {
		t.Fatalf("large prompt contains examples")
	}
}
