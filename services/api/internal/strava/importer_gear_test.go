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
		{
			name:  "virtual labels collapse to top level sports",
			input: "Virtual Ride|Virtual Run|Virtual Row",
			want:  []string{"cycling", "running", "rowing"},
		},
		{
			name:  "electric bike labels collapse to cycling",
			input: "E-Bike Ride|E-Mountain Bike Ride",
			want:  []string{"cycling"},
		},
		{
			name:  "german comma separated labels map through projection",
			input: "Radfahren,Lauf,Rudern",
			want:  []string{"cycling", "running", "rowing"},
		},
		{
			name:  "whitespace and duplicate labels preserve one top level value",
			input: " Ride | Ride | Mountain Bike Ride ",
			want:  []string{"cycling"},
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
