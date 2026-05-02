package strava

import (
	"slices"
	"testing"
)

func TestMapStravaDefaultSportsUsesTopLevelCanonicalSports(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "cycling variants dedupe to cycling",
			input: "Ride|Mountain Bike Ride|E-Bike Ride",
			want:  []string{"cycling"},
		},
		{
			name:  "running variants dedupe to running",
			input: "Run, Trail Run",
			want:  []string{"running"},
		},
		{
			name:  "racket subtypes collapse to racket for gear storage",
			input: "Badminton|Padel|Table Tennis",
			want:  []string{"racket"},
		},
		{
			name:  "equipment subtypes collapse to fitness equipment for gear storage",
			input: "Elliptical|Stair-Stepper",
			want:  []string{"fitness_equipment"},
		},
		{
			name:  "unknown labels preserve generic fallback",
			input: "Mystery Sport",
			want:  []string{"generic"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapStravaDefaultSports(tt.input)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("mapStravaDefaultSports(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}
