package enrichment

import (
	"slices"
	"strings"
	"testing"
)

// TestMapWeatherCodeUmlauts tests that weather codes return correct German umlauts
func TestMapWeatherCodeUmlauts(t *testing.T) {
	tests := []struct {
		code        int
		expectedTag string
		description string
	}{
		// Sunny conditions
		{0, "sonnig", "clear sky"},
		{1, "sonnig", "mostly clear"},

		// Cloudy conditions - CRITICAL: Umlauts must be correct
		{2, "bewölkt", "partly cloudy (must have ö)"},
		{3, "bewölkt", "overcast (must have ö)"},

		// Fog
		{45, "nebel", "foggy"},
		{48, "nebel", "foggy with rime"},

		// Rain variants
		{51, "regen", "light drizzle"},
		{61, "regen", "slight rain"},
		{63, "regen", "moderate rain"},

		// Snow variants
		{71, "schnee", "slight snow"},
		{73, "schnee", "moderate snow"},
		{77, "schnee", "snow grains"},

		// Showers
		{80, "regen", "rain showers"},
		{85, "schnee", "snow showers"},

		// Thunderstorm
		{95, "gewitter", "thunderstorm"},
		{99, "gewitter", "thunderstorm with hail"},
	}

	for _, tt := range tests {
		_, tags := mapWeatherCode(tt.code)
		if len(tags) == 0 {
			t.Errorf("%s (code %d): should return tags", tt.description, tt.code)
			continue
		}

		found := slices.Contains(tags, tt.expectedTag)
		if !found {
			t.Errorf("%s (code %d): expected tag '%s' not found in %v", tt.description, tt.code, tt.expectedTag, tags)
		}
	}
}

// TestMapWeatherCodeNoAsciiBrokenUmlauts tests that no broken ASCII umlauts are used
func TestMapWeatherCodeNoAsciiBrokenUmlauts(t *testing.T) {
	for code := 0; code <= 99; code++ {
		condition, tags := mapWeatherCode(code)
		if strings.Contains(strings.ToLower(condition), "ueberwiegend") {
			t.Errorf("code %d: found broken umlaut 'ueberwiegend' in condition '%s', should use 'überwiegend'", code, condition)
		}
		for _, tag := range tags {
			if strings.Contains(tag, "bewoelkt") {
				t.Errorf("code %d: found broken umlaut 'bewoelkt' in tag '%s', should use 'bewölkt'", code, tag)
			}
		}
	}
}

// TestWeatherTagConsistency tests that weather tags are consistent across all conditions
func TestWeatherTagConsistency(t *testing.T) {
	validTags := map[string]bool{
		"sonnig":   true,
		"bewölkt":  true,
		"regen":    true,
		"schnee":   true,
		"nebel":    true,
		"gewitter": true,
	}

	for code := 0; code <= 99; code++ {
		_, tags := mapWeatherCode(code)
		for _, tag := range tags {
			if !validTags[tag] {
				t.Errorf("code %d: unexpected tag '%s'", code, tag)
			}
		}
	}
}

// TestMapWeatherCodeReturnsCorrectConditions tests that conditions and tags align
func TestMapWeatherCodeReturnsCorrectConditions(t *testing.T) {
	tests := []struct {
		code              int
		expectedCondition string
		expectedAnyTag    string
	}{
		{0, "Klar", "sonnig"},
		{1, "Überwiegend klar", "sonnig"},
		{2, "Teilweise bewölkt", "bewölkt"},
		{3, "Bedeckt", "bewölkt"},
		{45, "Nebel", "nebel"},
		{61, "Regen", "regen"},
		{71, "Schneefall", "schnee"},
		{95, "Gewitter", "gewitter"},
	}

	for _, tt := range tests {
		condition, tags := mapWeatherCode(tt.code)
		if condition != tt.expectedCondition {
			t.Errorf("code %d: expected condition '%s', got '%s'", tt.code, tt.expectedCondition, condition)
		}

		if len(tags) == 0 {
			t.Errorf("code %d: should return at least one tag", tt.code)
			continue
		}

		found := slices.Contains(tags, tt.expectedAnyTag)
		if !found {
			t.Errorf("code %d: expected tag '%s' not found in %v", tt.code, tt.expectedAnyTag, tags)
		}
	}
}
