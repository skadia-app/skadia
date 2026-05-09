package strava

import (
	"slices"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/gear"
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
			name:  "unknown labels do not create defaults",
			input: "Mystery Sport",
			want:  nil,
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

func TestMapStravaDefaultSportPredicatesPreservesSubtypeAndModifiers(t *testing.T) {
	got := mapStravaDefaultSportPredicates("Ride|Mountain Bike Ride|E-Mountain Bike Ride|Virtual Ride")
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(got), got)
	}
	assertGearPredicate(t, got[0], "cycling", "", false, false)
	assertGearPredicate(t, got[1], "cycling", "mountain", false, false)
	assertGearPredicate(t, got[2], "cycling", "mountain", true, false)
	assertGearPredicate(t, got[3], "cycling", "", false, true)
}

func assertGearPredicate(t *testing.T, got gear.GearDefaultPredicate, sportType, subType string, electric, virtual bool) {
	t.Helper()
	if got.SportType != sportType {
		t.Fatalf("SportType = %q, want %q", got.SportType, sportType)
	}
	gotSubType := ""
	if got.SubType != nil {
		gotSubType = *got.SubType
	}
	if gotSubType != subType {
		t.Fatalf("SubType = %q, want %q", gotSubType, subType)
	}
	gotElectric := false
	gotVirtual := false
	if got.Modifiers != nil {
		if got.Modifiers.IsElectric != nil {
			gotElectric = *got.Modifiers.IsElectric
		}
		if got.Modifiers.IsVirtual != nil {
			gotVirtual = *got.Modifiers.IsVirtual
		}
	}
	if gotElectric != electric || gotVirtual != virtual {
		t.Fatalf("modifiers electric=%v virtual=%v, want electric=%v virtual=%v", gotElectric, gotVirtual, electric, virtual)
	}
}
