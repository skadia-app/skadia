package automation

import "testing"

func TestResolveNameTemplate_CitySubstitution(t *testing.T) {
	actMap := map[string]any{
		"start_city": "Zürich",
	}
	result := resolveNameTemplate("Runde in {city}", actMap)
	if result != "Runde in Zürich" {
		t.Errorf("got %q, want %q", result, "Runde in Zürich")
	}
}

func TestResolveNameTemplate_DistanceSubstitution(t *testing.T) {
	actMap := map[string]any{
		"distance_m": float64(15000),
	}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "15 km" {
		t.Errorf("got %q, want %q", result, "15 km")
	}
}

func TestResolveNameTemplate_DistanceRounding(t *testing.T) {
	actMap := map[string]any{
		"distance_m": float64(21097),
	}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "21 km" {
		t.Errorf("got %q, want %q", result, "21 km")
	}
}

func TestResolveNameTemplate_SportSubstitution(t *testing.T) {
	actMap := map[string]any{
		"sport_type": "running",
	}
	result := resolveNameTemplate("{sport}", actMap)
	if result != "running" {
		t.Errorf("got %q, want %q", result, "running")
	}
}

func TestResolveNameTemplate_MultipleTokens(t *testing.T) {
	actMap := map[string]any{
		"start_city": "Bern",
		"distance_m": float64(5000),
		"sport_type": "cycling",
	}
	result := resolveNameTemplate("{sport} in {city} ({distance_km})", actMap)
	if result != "cycling in Bern (5 km)" {
		t.Errorf("got %q, want %q", result, "cycling in Bern (5 km)")
	}
}

func TestResolveNameTemplate_MissingCityKey(t *testing.T) {
	actMap := map[string]any{}
	result := resolveNameTemplate("{city}", actMap)
	// Missing key — placeholder stays
	if result != "{city}" {
		t.Errorf("got %q, expected placeholder preserved", result)
	}
}

func TestResolveNameTemplate_MissingDistanceKey(t *testing.T) {
	actMap := map[string]any{}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "{distance_km}" {
		t.Errorf("got %q, expected placeholder preserved", result)
	}
}

func TestResolveNameTemplate_MissingSportKey(t *testing.T) {
	actMap := map[string]any{}
	result := resolveNameTemplate("{sport}", actMap)
	if result != "{sport}" {
		t.Errorf("got %q, expected placeholder preserved", result)
	}
}

func TestResolveNameTemplate_NoPlaceholders(t *testing.T) {
	actMap := map[string]any{"start_city": "Basel"}
	result := resolveNameTemplate("Morgenrunde", actMap)
	if result != "Morgenrunde" {
		t.Errorf("got %q, want %q", result, "Morgenrunde")
	}
}

func TestResolveNameTemplate_EmptyTemplate(t *testing.T) {
	actMap := map[string]any{
		"start_city": "Luzern",
		"distance_m": float64(8000),
	}
	result := resolveNameTemplate("", actMap)
	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestResolveNameTemplate_WrongType(t *testing.T) {
	// If distance_m is not float64 (e.g. int), substitution is skipped
	actMap := map[string]any{
		"distance_m": 10000, // int, not float64
	}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "{distance_km}" {
		t.Errorf("got %q, expected placeholder preserved for wrong type", result)
	}
}
