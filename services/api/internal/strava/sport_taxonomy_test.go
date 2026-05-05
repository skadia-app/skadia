package strava

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestNormalizeStravaSportFields(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric bool
		wantIndoor   bool
		wantVirtual  bool
	}{
		{input: "Ride", wantSport: "cycling"},
		{input: "Radfahrt", wantSport: "cycling"},
		{input: "Radfahren", wantSport: "cycling"},
		{input: "E-Bike Ride", wantSport: "cycling", wantElectric: true},
		{input: "EBikeRide", wantSport: "cycling", wantElectric: true},
		{input: "E-Bike-Radfahrt", wantSport: "cycling", wantElectric: true},
		{input: "E-Bike", wantSport: "cycling", wantElectric: true},
		{input: "Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain"},
		{input: "MountainBikeRide", wantSport: "cycling", wantSubType: "mountain"},
		{input: "Mountainbiking", wantSport: "cycling", wantSubType: "mountain"},
		{input: "Mountainbike-Fahrt", wantSport: "cycling", wantSubType: "mountain"},
		{input: "E-Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain", wantElectric: true},
		{input: "EMountainBikeRide", wantSport: "cycling", wantSubType: "mountain", wantElectric: true},
		{input: "Gravel Ride", wantSport: "cycling", wantSubType: "gravel_cycling"},
		{input: "GravelRide", wantSport: "cycling", wantSubType: "gravel_cycling"},
		{input: "Gravelbike", wantSport: "cycling", wantSubType: "gravel_cycling"},
		{input: "Gravel-Radfahrt", wantSport: "cycling", wantSubType: "gravel_cycling"},
		{input: "Virtual Ride", wantSport: "cycling", wantVirtual: true},
		{input: "VirtualRide", wantSport: "cycling", wantVirtual: true},
		{input: "Virtuelles Radfahren", wantSport: "cycling", wantVirtual: true},
		{input: "Indoor Cycling", wantSport: "cycling", wantSubType: "indoor_cycling", wantIndoor: true},
		{input: "Indoor-Radfahren", wantSport: "cycling", wantSubType: "indoor_cycling", wantIndoor: true},
		{input: "Spinning", wantSport: "cycling", wantSubType: "indoor_cycling", wantIndoor: true},
		{input: "Handcycle", wantSport: "cycling"},
		{input: "Velomobile", wantSport: "cycling"},
		{input: "Run", wantSport: "running"},
		{input: "Lauf", wantSport: "running"},
		{input: "Laufen", wantSport: "running"},
		{input: "Trail Run", wantSport: "running", wantSubType: "trail"},
		{input: "TrailRun", wantSport: "running", wantSubType: "trail"},
		{input: "Trailrunning", wantSport: "running", wantSubType: "trail"},
		{input: "Trailrun", wantSport: "running", wantSubType: "trail"},
		{input: "Treadmill", wantSport: "running", wantSubType: "treadmill", wantIndoor: true},
		{input: "Treadmill-Lauf", wantSport: "running", wantSubType: "treadmill", wantIndoor: true},
		{input: "Laufband", wantSport: "running", wantSubType: "treadmill", wantIndoor: true},
		{input: "Virtual Run", wantSport: "running", wantVirtual: true},
		{input: "VirtualRun", wantSport: "running", wantVirtual: true},
		{input: "Virtuelles Laufen", wantSport: "running", wantVirtual: true},
		{input: "Hike", wantSport: "hiking"},
		{input: "Wandern", wantSport: "hiking"},
		{input: "Wanderung", wantSport: "hiking"},
		{input: "Walk", wantSport: "walking"},
		{input: "Spaziergang", wantSport: "walking"},
		{input: "Swim", wantSport: "swimming"},
		{input: "Schwimmen", wantSport: "swimming"},
		{input: "Open Water Swim", wantSport: "swimming", wantSubType: "open_water"},
		{input: "Freiwasserschwimmen", wantSport: "swimming", wantSubType: "open_water"},
		{input: "Stand Up Paddling", wantSport: "stand_up_paddleboarding"},
		{input: "StandUpPaddling", wantSport: "stand_up_paddleboarding"},
		{input: "Stand-up-Paddeln", wantSport: "stand_up_paddleboarding"},
		{input: "SUP", wantSport: "stand_up_paddleboarding"},
		{input: "Kayaking", wantSport: "kayaking"},
		{input: "Kajak fahren", wantSport: "kayaking"},
		{input: "Kajak", wantSport: "kayaking"},
		{input: "Rowing", wantSport: "rowing"},
		{input: "Rudern", wantSport: "rowing"},
		{input: "Canoeing", wantSport: "kayaking"},
		{input: "Kanufahren", wantSport: "kayaking"},
		{input: "Virtual Row", wantSport: "rowing", wantVirtual: true},
		{input: "VirtualRow", wantSport: "rowing", wantVirtual: true},
		{input: "Virtuelles Rudern", wantSport: "rowing", wantVirtual: true},
		{input: "Sail", wantSport: "sailing"},
		{input: "Surfing", wantSport: "surfing"},
		{input: "Kitesurf", wantSport: "kitesurfing"},
		{input: "Windsurf", wantSport: "windsurfing"},
		{input: "Alpine Ski", wantSport: "alpine_skiing"},
		{input: "AlpineSki", wantSport: "alpine_skiing"},
		{input: "Skifahren", wantSport: "alpine_skiing"},
		{input: "Ski alpin", wantSport: "alpine_skiing"},
		{input: "Nordic Ski", wantSport: "cross_country_skiing"},
		{input: "NordicSki", wantSport: "cross_country_skiing"},
		{input: "Langlauf", wantSport: "cross_country_skiing"},
		{input: "Skilanglauf", wantSport: "cross_country_skiing"},
		{input: "Backcountry Ski", wantSport: "backcountry_skiing"},
		{input: "BackcountrySki", wantSport: "backcountry_skiing"},
		{input: "Snowboard", wantSport: "snowboarding"},
		{input: "Snowboarden", wantSport: "snowboarding"},
		{input: "Snowshoe", wantSport: "snowshoeing"},
		{input: "Schneeschuhwandern", wantSport: "snowshoeing"},
		{input: "Ice Skate", wantSport: "ice_skating"},
		{input: "IceSkate", wantSport: "ice_skating"},
		{input: "Roller Ski", wantSport: "cross_country_skiing"},
		{input: "RollerSki", wantSport: "cross_country_skiing"},
		{input: "Weight Training", wantSport: "strength_training", wantIndoor: true},
		{input: "WeightTraining", wantSport: "strength_training", wantIndoor: true},
		{input: "Krafttraining", wantSport: "strength_training", wantIndoor: true},
		{input: "Gewichtheben", wantSport: "strength_training", wantIndoor: true},
		{input: "Yoga", wantSport: "yoga", wantIndoor: true},
		{input: "Pilates", wantSport: "pilates", wantIndoor: true},
		{input: "Crossfit", wantSport: "training", wantIndoor: true},
		{input: "HIIT", wantSport: "hiit", wantIndoor: true},
		{input: "HighIntensityIntervalTraining", wantSport: "hiit", wantIndoor: true},
		{input: "Workout", wantSport: "training", wantIndoor: true},
		{input: "PhysicalTherapy", wantSport: "mobility", wantIndoor: true},
		{input: "Elliptical", wantSport: "fitness_equipment", wantSubType: "elliptical", wantIndoor: true},
		{input: "Stair-Stepper", wantSport: "fitness_equipment", wantSubType: "stair_climbing", wantIndoor: true},
		{input: "StairStepper", wantSport: "fitness_equipment", wantSubType: "stair_climbing", wantIndoor: true},
		{input: "Dance", wantSport: "dance", wantIndoor: true},
		{input: "Rock Climbing", wantSport: "rock_climbing"},
		{input: "RockClimbing", wantSport: "rock_climbing"},
		{input: "Klettern", wantSport: "rock_climbing"},
		{input: "Felsenklettern", wantSport: "rock_climbing"},
		{input: "Bouldern", wantSport: "rock_climbing", wantSubType: "bouldering"},
		{input: "Tennis", wantSport: "tennis"},
		{input: "Badminton", wantSport: "racket", wantSubType: "badminton"},
		{input: "Table Tennis", wantSport: "racket", wantSubType: "table_tennis"},
		{input: "TableTennis", wantSport: "racket", wantSubType: "table_tennis"},
		{input: "Squash", wantSport: "racket", wantSubType: "squash"},
		{input: "Pickleball", wantSport: "racket", wantSubType: "pickleball"},
		{input: "Racquetball", wantSport: "racket", wantSubType: "racquetball"},
		{input: "Padel", wantSport: "racket", wantSubType: "padel"},
		{input: "Football (Soccer)", wantSport: "soccer"},
		{input: "Soccer", wantSport: "soccer"},
		{input: "Golf", wantSport: "golf"},
		{input: "Basketball", wantSport: "basketball"},
		{input: "Volleyball", wantSport: "volleyball"},
		{input: "Cricket", wantSport: "cricket"},
		{input: "Inline Skate", wantSport: "inline_skating"},
		{input: "InlineSkate", wantSport: "inline_skating"},
		{input: "Skateboard", wantSport: "skateboarding"},
		{input: "Wheelchair", wantSport: "wheelchair_push"},
		{input: "Triathlon", wantSport: "multisport"},
		{input: "Multisport", wantSport: "multisport"},
		{input: "Sonstiges", wantSport: "generic"},
		{input: "Andere", wantSport: "generic"},
		{input: "Definitely Unknown Strava Label", wantSport: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeStravaSportFields(tt.input)
			if err != nil {
				t.Fatalf("normalizeStravaSportFields(%q): %v", tt.input, err)
			}
			if got.SportType != tt.wantSport {
				t.Errorf("SportType: got %q, want %q", got.SportType, tt.wantSport)
			}
			if got.SubType != tt.wantSubType {
				t.Errorf("SubType: got %q, want %q", got.SubType, tt.wantSubType)
			}
			if got.IsElectric != tt.wantElectric {
				t.Errorf("IsElectric: got %v, want %v", got.IsElectric, tt.wantElectric)
			}
			if got.IsIndoor != tt.wantIndoor {
				t.Errorf("IsIndoor: got %v, want %v", got.IsIndoor, tt.wantIndoor)
			}
			if got.IsVirtual != tt.wantVirtual {
				t.Errorf("IsVirtual: got %v, want %v", got.IsVirtual, tt.wantVirtual)
			}
		})
	}
}

func TestMergeActivityDataPropagatesParsedModifiers(t *testing.T) {
	base := &model.ActivityData{
		SportType: "cycling",
		SubType:   "",
	}
	parsed := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "indoor_cycling",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
	}

	mergeActivityData(base, parsed)

	if base.SubType != "indoor_cycling" {
		t.Fatalf("SubType = %q, want indoor_cycling", base.SubType)
	}
	if !base.IsElectric {
		t.Error("IsElectric was not propagated")
	}
	if !base.IsIndoor {
		t.Error("IsIndoor was not propagated")
	}
	if !base.IsVirtual {
		t.Error("IsVirtual was not propagated")
	}
}

func TestMergeActivityDataKeepsStravaElectricEvidence(t *testing.T) {
	base := &model.ActivityData{
		SportType:  "cycling",
		IsElectric: true,
	}
	parsed := &model.ActivityData{
		SportType: "cycling",
		SubType:   "road",
	}

	mergeActivityData(base, parsed)

	if base.SubType != "road" {
		t.Fatalf("SubType = %q, want road from parsed file", base.SubType)
	}
	if !base.IsElectric {
		t.Fatal("IsElectric = false, want Strava CSV electric evidence preserved")
	}
}

func TestMergeActivityDataUsesTaxonomyBroadFallbackRefinements(t *testing.T) {
	base := &model.ActivityData{
		SportType: "paddling",
	}
	parsed := &model.ActivityData{
		SportType: "kayaking",
	}

	mergeActivityData(base, parsed)

	if base.SportType != "kayaking" {
		t.Fatalf("SportType = %q, want kayaking", base.SportType)
	}
}

func TestMergeActivityDataKeepsUserCorrectedDifferentFamily(t *testing.T) {
	base := &model.ActivityData{
		SportType: "swimming",
	}
	parsed := &model.ActivityData{
		SportType: "diving",
	}

	mergeActivityData(base, parsed)

	if base.SportType != "swimming" {
		t.Fatalf("SportType = %q, want swimming", base.SportType)
	}
}
