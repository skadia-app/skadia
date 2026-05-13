package agent

import "testing"

func TestMatchTemplate(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		wantName   string
		wantParams []any
	}{
		{name: "last activities default", message: "letzte Aktivität", wantName: "last_activities", wantParams: []any{5}},
		{name: "last activities count", message: "letzte 3 Aktivitäten", wantName: "last_activities", wantParams: []any{3}},
		{name: "last activities count clamped", message: "letzte 500 Aktivitäten", wantName: "last_activities", wantParams: []any{50}},
		{name: "weekly summary", message: "Wochenzusammenfassung bitte", wantName: "weekly_summary", wantParams: []any{}},
		{name: "hrv trend", message: "Wie ist mein HRV Trend?", wantName: "hrv_trend", wantParams: []any{}},
		{name: "readiness", message: "Wie erholt bin ich?", wantName: "readiness", wantParams: []any{}},
		{name: "english recent", message: "recent 2 activities", wantName: "last_activities", wantParams: []any{2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchTemplate(tt.message)
			if got == nil {
				t.Fatal("MatchTemplate returned nil")
			}
			if got.Name != tt.wantName {
				t.Fatalf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if len(got.Params) != len(tt.wantParams) {
				t.Fatalf("Params = %#v, want %#v", got.Params, tt.wantParams)
			}
			for i := range got.Params {
				if got.Params[i] != tt.wantParams[i] {
					t.Fatalf("Params = %#v, want %#v", got.Params, tt.wantParams)
				}
			}
		})
	}
}

func TestMatchTemplateNoMatch(t *testing.T) {
	if got := MatchTemplate("Was ist mein Lieblingskaffee?"); got != nil {
		t.Fatalf("MatchTemplate = %+v, want nil", got)
	}
}
