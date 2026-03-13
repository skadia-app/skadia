package graph

import "testing"

func TestParseMaxspeed(t *testing.T) {
	tests := []struct {
		input string
		want  uint8
	}{
		{"50", 50},
		{"30", 30},
		{"120", 120},
		{"255", 255},
		{"300", 255}, // clamped
		{"30 mph", 48},
		{"60 mph", 97},
		{"DE:urban", 50},
		{"DE:rural", 100},
		{"DE:motorway", 130},
		{"AT:urban", 50},
		{"CH:urban", 50},
		{"walk", 5},
		{"none", 0},
		{"signals", 0},
		{"", 0},
		{"unknown_value", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseMaxspeed(tt.input)
			if got != tt.want {
				t.Errorf("ParseMaxspeed(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
