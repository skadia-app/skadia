package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillsParsesMovedYAMLs(t *testing.T) {
	skills, err := LoadSkills(repoSkillsDir(t))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	wantNames := []string{
		"activities",
		"gear",
		"geo",
		"health",
		"photos",
		"routing",
		"segments",
		"training",
	}
	if len(skills) != len(wantNames) {
		t.Fatalf("loaded %d skills, want %d", len(skills), len(wantNames))
	}
	for _, name := range wantNames {
		if skills[name] == nil {
			t.Fatalf("missing skill %q", name)
		}
	}

	gear := skills["gear"]
	if gear.Description != "Ausrüstung, Verschleiß, km-Stand, Wartung, Kosten" {
		t.Fatalf("gear description = %q", gear.Description)
	}
	if len(gear.Tools) != 8 {
		t.Fatalf("gear tools = %d, want 8", len(gear.Tools))
	}
	if len(gear.FewShotExamples) != 4 {
		t.Fatalf("gear few-shot examples = %d, want 4", len(gear.FewShotExamples))
	}
}

func TestLoadSkillsUsesDefaultSkillsDir(t *testing.T) {
	skills, err := LoadSkills("")
	if err != nil {
		t.Fatalf("LoadSkills default dir: %v", err)
	}
	if len(skills) != 8 {
		t.Fatalf("loaded %d default skills, want 8", len(skills))
	}
}

func TestSkillGearContextStringMatchesPythonFormatting(t *testing.T) {
	skills, err := LoadSkills(repoSkillsDir(t))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	want := strings.Join([]string{
		"## Kontext: Ausrüstung, Verschleiß, km-Stand, Wartung, Kosten",
		"",
		"### Schema",
		"gear: id UUID, user_id, name TEXT, type TEXT (e.g. bike.mtb/bike.gravel/shoes.running),",
		"  scope TEXT (bike/shoes/other), sub_type TEXT, brand TEXT, model TEXT,",
		"  total_distance_m REAL, total_time_s BIGINT, total_elevation_gain_m REAL,",
		"  total_activities INT, purchased_at DATE, retired_at DATE, is_retired BOOL,",
		"  status TEXT (active/stored/retired),",
		"  applicable_sport_predicates JSONB (sport bindings, e.g. [{sport_type:cycling}]),",
		"  attributes JSONB (type-specific fields per schema, e.g. frame_size, weight_g),",
		"  modifiers JSONB (boolean flags, e.g. {is_electric: true})",
		"gear_components: id, gear_id, name TEXT, type TEXT (chain/tire/cassette/brake_pad/…),",
		"  brand TEXT, model TEXT, total_distance_m REAL, total_elevation_gain_m REAL,",
		"  total_time_s BIGINT, installed_at DATE, removed_at DATE, is_retired BOOL, status TEXT",
		"gear_service_events: id, component_id, performed_at DATE,",
		"  description TEXT, notes TEXT",
		"",
		"### Hinweise",
		"- Laufschuhe: Lebensdauer 600-1000 km (je nach Modell/Untergrund)",
		"- Fahrradkette: ~3000-5000 km, Kassette: ~10.000 km",
		"- Bremsbeläge: ~5000-10.000 km (Scheibenbremse)",
		"- Reifen (Rennrad): ~3000-6000 km, MTB: ~2000-4000 km",
		"- Status: active = in Gebrauch, stored = eingelagert, retired = ausgemustert",
		"- Mehrdeutige Namen: Wenn GET /api/gear?name=X mit 409 und gear.name_ambiguous",
		"  antwortet, enthält die Response eine candidates-Liste. Dann beim User nachfragen",
		"  welches Gerät gemeint ist und mit der konkreten gear_id wiederholen.",
		"- gear_list filtert per status, scope, oder type-Präfix (z.B. 'bike' trifft alle bike.*-Typen). Ohne status-Filter werden nur nicht-ausgemusterte Gegenstände zurückgegeben (is_retired=false); include_retired=true für alle.",
		"- gear_summary aggregiert km/Höhenmeter über mehrere Ausrüstungsgegenstände.",
		"- gear_component_detail zeigt Komponenten (Kette, Reifen, …) mit km-Stand.",
		"- gear_service_events zeigt Wartungshistorie pro Komponente oder Gerät.",
		"- query_gear_maintenance fasst Komponenten, Wartungsintervalle, letzte Service-Events und fällige/überfällige Hinweise zusammen.",
	}, "\n")

	if got := skills["gear"].ToContextString(); got != want {
		t.Fatalf("gear context mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSkillGearFewShotsMatchPythonFormattingWithMultiStepExamples(t *testing.T) {
	skills, err := LoadSkills(repoSkillsDir(t))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	want := strings.Join([]string{
		"## Beispiele",
		"Frage: \"Wie viele km haben meine Laufschuhe?\"",
		"→ get_gear_stats({})",
		"→ 3 aktive Laufschuhe: Nike Pegasus 41 (623 km — bald wechseln!), Hoka Mach 6 (234 km), Brooks Ghost 16 (89 km).",
		"",
		"Frage: \"Was haben meine Laufschuhe gekostet?\"",
		"→ gear_list({'scope': 'shoes', 'status': 'active'})",
		"→ get_gear_documents({'gear_id': '<id from previous step>'})",
		"",
		"Frage: \"Wieviele km bin ich mit allen Rädern letztes Jahr gefahren?\"",
		"→ gear_summary({\"scope\": \"bike\", \"since\": \"2025-01-01\", \"until\": \"2026-01-01\"})",
		"→ Mit allen Rädern 2025: 12.340 km auf 287 Aktivitäten.",
		"",
	}, "\n")

	if got := skills["gear"].FormatFewShots(3); got != want {
		t.Fatalf("gear few shots mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestLoadSkillsReturnsErrorForMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("name: [\n"), 0o600); err != nil {
		t.Fatalf("write malformed yaml: %v", err)
	}

	if _, err := LoadSkills(dir); err == nil {
		t.Fatal("LoadSkills malformed YAML error = nil")
	}
}

func repoSkillsDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		candidate := filepath.Join(wd, "config", "agent", "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatalf("could not find config/agent/skills from %s", wd)
		}
		wd = parent
	}
}
