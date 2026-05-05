package strava

import "testing"

func TestParseBool(t *testing.T) {
	tests := []struct {
		input string
		want  bool
		ok    bool
	}{
		{input: "true", want: true, ok: true},
		{input: "FALSE", want: false, ok: true},
		{input: "ja", want: true, ok: true},
		{input: "nein", want: false, ok: true},
		{input: "", want: false, ok: false},
		{input: "maybe", want: false, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := parseBool(tt.input)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseBool(%q) = (%v, %v), want (%v, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}
