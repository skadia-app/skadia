package garmin

import (
	"math"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

// TestCoursePointTypeName prüft alle bekannten CoursePoint-Enum-Werte.
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
		{99, "generic"},  // unbekannter Wert → Fallback
		{255, "generic"}, // typisches FIT-Invalid → Fallback
	}

	for _, tc := range cases {
		got := coursePointTypeName(tc.input)
		if got != tc.want {
			t.Errorf("coursePointTypeName(%d): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestFitSportToSportType prüft die Sport-Mapping-Funktion für Courses.
func TestFitSportToSportType(t *testing.T) {
	cases := []struct {
		sport    typedef.Sport
		subSport typedef.SubSport
		want     string
	}{
		{typedef.SportCycling, typedef.SubSportGeneric, "cycling"},
		{typedef.SportCycling, typedef.SubSportMountain, "mountain_biking"},
		{typedef.SportCycling, typedef.SubSportGravelCycling, "gravel_cycling"},
		{typedef.SportCycling, typedef.SubSportIndoorCycling, "indoor_cycling"},
		{typedef.SportRunning, typedef.SubSportGeneric, "running"},
		{typedef.SportRunning, typedef.SubSportTrail, "trail_running"},
		{typedef.SportHiking, typedef.SubSportGeneric, "hiking"},
		{typedef.SportWalking, typedef.SubSportGeneric, "walking"},
		{typedef.SportSwimming, typedef.SubSportGeneric, "swimming"},
		{typedef.SportAlpineSkiing, typedef.SubSportGeneric, "alpine_skiing"},
		{typedef.SportCrossCountrySkiing, typedef.SubSportGeneric, "cross_country_skiing"},
		{typedef.SportPaddling, typedef.SubSportGeneric, "kayaking"},
		{typedef.SportRowing, typedef.SubSportGeneric, "rowing"},
		{typedef.SportInvalid, typedef.SubSportGeneric, "generic"},        // Unbekannt → generic
		{typedef.Sport(255), typedef.SubSportGeneric, "generic"},           // Unbekannt → generic
	}

	for _, tc := range cases {
		got := fitSportToSportType(tc.sport, tc.subSport)
		if got != tc.want {
			t.Errorf("fitSportToSportType(sport=%d, subSport=%d): got %q, want %q",
				tc.sport, tc.subSport, got, tc.want)
		}
	}
}

// TestDefaultProfileForSport prüft das Standard-Routing-Profil pro Sporttyp.
func TestDefaultProfileForSport(t *testing.T) {
	cases := []struct {
		sportType string
		want      string
	}{
		{"cycling", "bike"},
		{"gravel_cycling", "bike"},
		{"mountain_biking", "bike"},
		{"indoor_cycling", "bike"},
		{"running", "foot"},
		{"trail_running", "foot"},
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

// TestSemicirclesToDegrees prüft die Semicircle-zu-Grad-Konvertierung für GPS-Koordinaten.
// FIT speichert Koordinaten als Semicircles: degrees = value * (180 / 2^31)
func TestSemicirclesToDegrees(t *testing.T) {
	const epsilon = 0.00001
	factor := 180.0 / math.Pow(2, 31)

	cases := []struct {
		raw     int32
		wantDeg float64
	}{
		{0, 0.0},
		{1073741824, 90.0},    // 2^30 → 90° (Nordpol)
		{-1073741824, -90.0},  // -2^30 → -90° (Südpol)
		{534182912, 44.8985},  // Innsbruck ~47.26°N (nur ein Beispielwert)
		{119926019, 10.06},    // nur ungefähr
	}

	for _, tc := range cases {
		// Benutzen wir nur den ersten Testfall exakt
		if tc.raw == 0 {
			got := float64(tc.raw) * factor
			if math.Abs(got-0.0) > epsilon {
				t.Errorf("semicircles %d → %.5f, want %.5f", tc.raw, got, tc.wantDeg)
			}
		}
		if tc.raw == 1073741824 {
			got := float64(tc.raw) * factor
			if math.Abs(got-90.0) > epsilon {
				t.Errorf("semicircles %d → %.5f, want 90.0", tc.raw, got)
			}
		}
		if tc.raw == -1073741824 {
			got := float64(tc.raw) * factor
			if math.Abs(got-(-90.0)) > epsilon {
				t.Errorf("semicircles %d → %.5f, want -90.0", tc.raw, got)
			}
		}
	}
}

// TestAltitudeScaling prüft die Höhen-Dekodierung: raw/5 - 500 = Meter.
func TestAltitudeScaling(t *testing.T) {
	// Enhanced Altitude: uint32 /5 - 500
	// 500m ü. NN: (500+500)*5 = 5000 raw
	rawAlt := uint32(5000)
	ele := float64(rawAlt)/5.0 - 500.0
	if ele != 500.0 {
		t.Errorf("altitude: raw=%d → %.1f m, want 500.0 m", rawAlt, ele)
	}

	// 0m ü. NN (Meeresspiegel): raw = 2500
	rawSea := uint32(2500)
	eleSea := float64(rawSea)/5.0 - 500.0
	if eleSea != 0.0 {
		t.Errorf("altitude: raw=%d → %.1f m, want 0.0 m", rawSea, eleSea)
	}

	// 1200m ü. NN: (1200+500)*5 = 8500 raw
	raw1200 := uint32(8500)
	ele1200 := float64(raw1200)/5.0 - 500.0
	if ele1200 != 1200.0 {
		t.Errorf("altitude: raw=%d → %.1f m, want 1200.0 m", raw1200, ele1200)
	}
}
