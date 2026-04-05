package automation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

//go:fix inline
func ptr(s string) *string { return new(s) }

func TestShouldEvaluateRule(t *testing.T) {
	tests := []struct {
		name         string
		rule         model.AutomationRule
		triggerEvent string
		want         bool
	}{
		{
			"active on_import matches on_import",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_import"},
			"on_import", true,
		},
		{
			"active on_enrichment skipped by on_import filter",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_enrichment_complete"},
			"on_import", false,
		},
		{
			"empty trigger matches all (manual apply)",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_enrichment_complete"},
			"", true,
		},
		{
			"inactive rule always skipped",
			model.AutomationRule{IsActive: false, TriggerEvent: "on_import"},
			"on_import", false,
		},
		{
			"electric_sync always skipped",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_import", SystemType: new("electric_sync")},
			"on_import", false,
		},
		{
			"indoor_sync always skipped",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_import", SystemType: new("indoor_sync")},
			"", false,
		},
		{
			"round_trip system rule NOT skipped",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_import", SystemType: new("round_trip")},
			"on_import", true,
		},
		{
			"frost system rule NOT skipped",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_import", SystemType: new("frost")},
			"on_import", true,
		},
		{
			"on_manual_apply only matches empty trigger",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_manual_apply"},
			"on_import", false,
		},
		{
			"on_manual_apply matches manual trigger",
			model.AutomationRule{IsActive: true, TriggerEvent: "on_manual_apply"},
			"on_manual_apply", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEvaluateRule(tt.rule, tt.triggerEvent); got != tt.want {
				t.Errorf("shouldEvaluateRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateRule(t *testing.T) {
	actMap := map[string]any{
		"sport_type": "running",
		"distance_m": float64(25000),
		"tags":       []string{"outdoor"},
	}

	tests := []struct {
		name       string
		conditions string
		want       bool
	}{
		{
			"simple eq match",
			`[{"field":"sport_type","op":"eq","value":"running"}]`,
			true,
		},
		{
			"simple eq no match",
			`[{"field":"sport_type","op":"eq","value":"cycling"}]`,
			false,
		},
		{
			"numeric gte match",
			`[{"field":"distance_m","op":"gte","value":20000}]`,
			true,
		},
		{
			"between match",
			`[{"field":"distance_m","op":"between","value":"21000,42195"}]`,
			true,
		},
		{
			"has_tag eq match",
			`[{"field":"has_tag","op":"eq","value":"outdoor"}]`,
			true,
		},
		{
			"has_tag neq match (tag absent)",
			`[{"field":"has_tag","op":"neq","value":"indoor"}]`,
			true,
		},
		{
			"has_tag neq no match (tag present)",
			`[{"field":"has_tag","op":"neq","value":"outdoor"}]`,
			false,
		},
		{
			"empty conditions = vacuously true (match-all)",
			`[]`,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := model.AutomationRule{
				Conditions: json.RawMessage(tt.conditions),
			}
			if got := EvaluateRule(rule, actMap); got != tt.want {
				t.Errorf("EvaluateRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
	// Missing key — empty string (use {city|fallback} to provide a default)
	if result != "" {
		t.Errorf("got %q, expected empty string for missing key", result)
	}
}

func TestResolveNameTemplate_MissingDistanceKey(t *testing.T) {
	actMap := map[string]any{}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "" {
		t.Errorf("got %q, expected empty string for missing key", result)
	}
}

func TestResolveNameTemplate_MissingSportKey(t *testing.T) {
	actMap := map[string]any{}
	result := resolveNameTemplate("{sport}", actMap)
	if result != "" {
		t.Errorf("got %q, expected empty string for missing key", result)
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
	// toFloat handles int as well as float64
	actMap := map[string]any{
		"distance_m": 10000, // int, not float64
	}
	result := resolveNameTemplate("{distance_km}", actMap)
	if result != "10 km" {
		t.Errorf("got %q, want %q", result, "10 km")
	}
}

func TestResolveNameTemplate(t *testing.T) {
	actMap := map[string]any{
		"start_city":       "München",
		"distance_m":       float64(42500),
		"sport_type":       "running",
		"elevation_gain_m": float64(1200),
		"started_at":       time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
	}
	tests := []struct{ tmpl, want string }{
		// Legacy aliases
		{"{city}", "München"},
		{"{distance_km}", "43 km"},
		{"{sport}", "running"},
		// New tokens
		{"{weekday}", "Sonntag"},
		{"{elevation_gain}", "1200"},
		{"{start_city}", "München"},
		// NULL fallback
		{"{start_state|unbekannt}", "unbekannt"},
		{"{start_city|egal}", "München"},
		// Combined
		{"{weekday}slauf in {city}", "Sonntagslauf in München"},
	}
	for _, tt := range tests {
		t.Run(tt.tmpl, func(t *testing.T) {
			got := resolveNameTemplate(tt.tmpl, actMap)
			if got != tt.want {
				t.Errorf("resolveNameTemplate(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}
