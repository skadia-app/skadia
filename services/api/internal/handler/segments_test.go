package handler

import "testing"

func TestNormalizeSurfaceLabel(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"", ""},
		{"Asphalt", "Asphalt"},
		{"Pflaster", "Pflaster"},
		{"Schotter", "Schotter (grob)"},        // historic: promoted to grob
		{"Schotter (fein)", "Schotter (fein)"},
		{"Schotter (grob)", "Schotter (grob)"},
		{"Erde", "Naturweg"},                   // historic
		{"Naturweg", "Naturweg"},
		{"Naturweg (fest)", "Naturweg (fest)"}, // user-only sub-type: pass through
		{"Holz", "Holz"},
		{"Sportbelag", "Sportbelag"},
		{"Schnee", "Schnee"},
		{"Wechselnd", "Wechselnd"},             // editor-only: pass through
		{"   Asphalt  ", "Asphalt"},            // trim whitespace
		{"Unbekannt", "Unbekannt"},
		{"NonsenseValue", "NonsenseValue"},     // unknown stays raw (UI falls back)
	}
	for _, c := range cases {
		got := normalizeSurfaceLabel(c.input)
		if got != c.want {
			t.Errorf("normalizeSurfaceLabel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
