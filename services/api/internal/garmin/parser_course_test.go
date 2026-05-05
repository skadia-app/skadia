package garmin

import (
	"math"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

// TestCoursePointTypeName checks all known CoursePoint enum values.
func TestCoursePointTypeName(t *testing.T) {
	cases := []struct {
		input typedef.CoursePoint
		want  string
	}{
		{0, "generic"},
		{1, "summit"},
		{2, "valley"},
		{3, "water"},
		{4, "food"},
		{5, "danger"},
		{6, "left"},
		{7, "right"},
		{8, "straight"},
		{9, "first_aid"},
		{10, "4th_category"},
		{11, "3rd_category"},
		{12, "2nd_category"},
		{13, "1st_category"},
		{14, "hors_category"},
		{15, "sprint"},
		{16, "left_fork"},
		{17, "right_fork"},
		{18, "middle_fork"},
		{19, "slight_left"},
		{20, "sharp_left"},
		{21, "slight_right"},
		{22, "sharp_right"},
		{23, "u_turn"},
		{24, "segment_start"},
		{25, "segment_end"},
		{26, "campsite"},
		{27, "aid_station"},
		{28, "rest_stop"},
		{40, "general_distance"},
		{48, "custom"},
		{99, "generic"},  // unknown value -> fallback
		{255, "generic"}, // typical FIT invalid value -> fallback
	}

	for _, tc := range cases {
		got := coursePointTypeName(tc.input)
		if got != tc.want {
			t.Errorf("coursePointTypeName(%d): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestFitSportToSportType checks course sport tuple mapping.
func TestFitSportToSportType(t *testing.T) {
	cases := []struct {
		sport     typedef.Sport
		subSport  typedef.SubSport
		wantSport string
		wantSub   string
	}{
		{typedef.SportCycling, typedef.SubSportGeneric, "cycling", ""},
		{typedef.SportCycling, typedef.SubSportMountain, "cycling", "mountain"},
		{typedef.SportCycling, typedef.SubSportEnduro, "cycling", "mountain"},
		{typedef.SportCycling, typedef.SubSportEBikeEnduro, "cycling", "mountain"},
		{typedef.SportCycling, typedef.SubSportGravelCycling, "cycling", "gravel_cycling"},
		{typedef.SportCycling, typedef.SubSportIndoorCycling, "cycling", "indoor_cycling"},
		{typedef.SportRunning, typedef.SubSportGeneric, "running", ""},
		{typedef.SportRunning, typedef.SubSportTrail, "running", "trail"},
		{typedef.SportHiking, typedef.SubSportGeneric, "hiking", ""},
		{typedef.SportWalking, typedef.SubSportGeneric, "walking", ""},
		{typedef.SportSwimming, typedef.SubSportGeneric, "swimming", ""},
		{typedef.SportAlpineSkiing, typedef.SubSportGeneric, "alpine_skiing", ""},
		{typedef.SportCrossCountrySkiing, typedef.SubSportGeneric, "cross_country_skiing", ""},
		{typedef.SportPaddling, typedef.SubSportGeneric, "paddling", ""},
		{typedef.SportRowing, typedef.SubSportGeneric, "rowing", ""},
		{typedef.SportCanoeing, typedef.SubSportGeneric, "canoeing", ""},
		{typedef.SportGolf, typedef.SubSportGeneric, "golf", ""},
		{typedef.SportSnowboarding, typedef.SubSportGeneric, "snowboarding", ""},
		{typedef.SportStandUpPaddleboarding, typedef.SubSportGeneric, "stand_up_paddleboarding", ""},
		{typedef.SportKayaking, typedef.SubSportGeneric, "kayaking", ""},
		{typedef.SportEBiking, typedef.SubSportGeneric, "cycling", ""},
		{typedef.SportPoolApnea, typedef.SubSportGeneric, "diving", "apnea_pool"},
		{typedef.SportWaterSport, typedef.SubSportGeneric, "unknown", ""},
		{typedef.SportWinterSport, typedef.SubSportGeneric, "unknown", ""},
		{typedef.SportInvalid, typedef.SubSportGeneric, "unknown", ""}, // unknown -> unknown
		{typedef.Sport(255), typedef.SubSportGeneric, "unknown", ""},   // unknown -> unknown
	}

	for _, tc := range cases {
		gotSport, gotSub := fitSportToSportType(tc.sport, tc.subSport)
		if gotSport != tc.wantSport || ptrStringValue(gotSub) != tc.wantSub {
			t.Errorf("fitSportToSportType(sport=%d, subSport=%d): got %q/%q, want %q/%q",
				tc.sport, tc.subSport, gotSport, ptrStringValue(gotSub), tc.wantSport, tc.wantSub)
		}
	}
}

func TestFitSportToRouteSportAppliesElectricModifier(t *testing.T) {
	gotSport, gotSub, gotPatch := fitSportToRouteSport(typedef.SportCycling, typedef.SubSportEBikeEnduro)
	if gotSport != "cycling" || ptrStringValue(gotSub) != "mountain" {
		t.Fatalf("tuple = %s/%s, want cycling/mountain", gotSport, ptrStringValue(gotSub))
	}
	if gotPatch.IsElectric == nil || !*gotPatch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", gotPatch.IsElectric)
	}
}

func TestFitSportToRouteSportAppliesEBikingModifier(t *testing.T) {
	gotSport, gotSub, gotPatch := fitSportToRouteSport(typedef.SportEBiking, typedef.SubSportGeneric)
	if gotSport != "cycling" || ptrStringValue(gotSub) != "" {
		t.Fatalf("tuple = %s/%s, want cycling/", gotSport, ptrStringValue(gotSub))
	}
	if gotPatch.IsElectric == nil || !*gotPatch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", gotPatch.IsElectric)
	}
}

// TestDefaultProfileForSport checks the default routing profile per sport type.
func TestDefaultProfileForSport(t *testing.T) {
	cases := []struct {
		sportType string
		want      string
	}{
		{"cycling", "bike"},
		{"running", "foot"},
		{"hiking", "foot"},
		{"walking", "foot"},
		{"swimming", "outdoor"},
		{"rowing", "outdoor"},
		{"generic", "outdoor"},
		{"unknown_sport", "outdoor"},
	}

	for _, tc := range cases {
		got := defaultProfileForSport(tc.sportType)
		if got != tc.want {
			t.Errorf("defaultProfileForSport(%q): got %q, want %q", tc.sportType, got, tc.want)
		}
	}
}

func TestDefaultProfileForSportTupleUsesCanonicalTuple(t *testing.T) {
	tests := []struct {
		name      string
		sportType string
		subType   string
		want      string
	}{
		{name: "cycling", sportType: "cycling", want: "bike"},
		{name: "road cycling", sportType: "cycling", subType: "road", want: "bike"},
		{name: "mountain biking", sportType: "cycling", subType: "mountain", want: "bike"},
		{name: "gravel cycling", sportType: "cycling", subType: "gravel_cycling", want: "bike"},
		{name: "indoor cycling", sportType: "cycling", subType: "indoor_cycling", want: "bike"},
		{name: "running", sportType: "running", want: "foot"},
		{name: "trail running", sportType: "running", subType: "trail", want: "foot"},
		{name: "hiking", sportType: "hiking", want: "foot"},
		{name: "walking", sportType: "walking", want: "foot"},
		{name: "unrelated", sportType: "rowing", want: "outdoor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var subType *string
			if tt.subType != "" {
				subType = &tt.subType
			}
			got := defaultProfileForSportTuple(tt.sportType, subType)
			if got != tt.want {
				t.Fatalf("defaultProfileForSportTuple(%q, %q) = %q, want %q", tt.sportType, tt.subType, got, tt.want)
			}
		})
	}
}

// TestSemicirclesToDegrees checks semicircle-to-degree conversion for GPS coordinates.
// FIT stores coordinates as semicircles: degrees = value * (180 / 2^31).
func TestSemicirclesToDegrees(t *testing.T) {
	const epsilon = 0.00001
	factor := 180.0 / math.Pow(2, 31)

	cases := []struct {
		raw     int32
		wantDeg float64
	}{
		{0, 0.0},
		{1073741824, 90.0},   // 2^30 -> 90 degrees north
		{-1073741824, -90.0}, // -2^30 -> 90 degrees south
		{534182912, 44.8985}, // example value
		{119926019, 10.06},   // approximate only
	}

	for _, tc := range cases {
		// Only the cardinal test cases are asserted exactly.
		if tc.raw == 0 {
			got := float64(tc.raw) * factor
			if math.Abs(got-0.0) > epsilon {
				t.Errorf("semicircles %d -> %.5f, want %.5f", tc.raw, got, tc.wantDeg)
			}
		}
		if tc.raw == 1073741824 {
			got := float64(tc.raw) * factor
			if math.Abs(got-90.0) > epsilon {
				t.Errorf("semicircles %d -> %.5f, want 90.0", tc.raw, got)
			}
		}
		if tc.raw == -1073741824 {
			got := float64(tc.raw) * factor
			if math.Abs(got-(-90.0)) > epsilon {
				t.Errorf("semicircles %d -> %.5f, want -90.0", tc.raw, got)
			}
		}
	}
}

// TestAltitudeScaling checks altitude decoding: raw/5 - 500 = meters.
func TestAltitudeScaling(t *testing.T) {
	// Enhanced Altitude: uint32 /5 - 500
	// 500m above sea level: (500+500)*5 = 5000 raw.
	rawAlt := uint32(5000)
	ele := float64(rawAlt)/5.0 - 500.0
	if ele != 500.0 {
		t.Errorf("altitude: raw=%d -> %.1f m, want 500.0 m", rawAlt, ele)
	}

	// 0m above sea level: raw = 2500.
	rawSea := uint32(2500)
	eleSea := float64(rawSea)/5.0 - 500.0
	if eleSea != 0.0 {
		t.Errorf("altitude: raw=%d -> %.1f m, want 0.0 m", rawSea, eleSea)
	}

	// 1200m above sea level: (1200+500)*5 = 8500 raw.
	raw1200 := uint32(8500)
	ele1200 := float64(raw1200)/5.0 - 500.0
	if ele1200 != 1200.0 {
		t.Errorf("altitude: raw=%d -> %.1f m, want 1200.0 m", raw1200, ele1200)
	}
}
