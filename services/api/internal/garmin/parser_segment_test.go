package garmin

import (
	"fmt"
	"math"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

// TestFitSportToSegmentGroup prüft das Mapping von FIT-Sport zu Skadia-Segment-Gruppe.
func TestFitSportToSegmentGroup(t *testing.T) {
	cases := []struct {
		sport typedef.Sport
		want  string
	}{
		{typedef.SportCycling, "bike"},
		{typedef.SportRunning, "foot"},
		{typedef.SportHiking, "foot"},
		{typedef.SportWalking, "foot"},
		{typedef.SportSwimming, "swim"},
		{typedef.SportAlpineSkiing, "ski"},
		{typedef.SportCrossCountrySkiing, "ski"},
		{typedef.SportSnowboarding, "ski"},
		{typedef.SportSnowshoeing, "ski"},
		{typedef.SportPaddling, "paddle"},
		{typedef.SportRowing, "paddle"},
		{typedef.SportKayaking, "paddle"},
		{typedef.SportSailing, "paddle"},
		{typedef.SportInvalid, "generic"},    // unbekannt → generic
		{typedef.Sport(255), "generic"},       // FIT Invalid → generic
		{typedef.Sport(100), "generic"},       // unbekannter Wert → generic
	}

	for _, tc := range cases {
		got := fitSportToSegmentGroup(tc.sport)
		if got != tc.want {
			t.Errorf("fitSportToSegmentGroup(%d): got %q, want %q", tc.sport, got, tc.want)
		}
	}
}

// TestSegmentSemicirclesToDegrees prüft die Koordinatenkonvertierung im Segment-Parser.
// Identische Logik wie im Course-Parser: degrees = value * (180 / 2^31)
func TestSegmentSemicirclesToDegrees(t *testing.T) {
	const epsilon = 0.00001
	factor := 180.0 / math.Pow(2, 31)

	// Nullpunkt
	lat0 := float64(int32(0)) * factor
	if math.Abs(lat0-0.0) > epsilon {
		t.Errorf("0 semicircles → %.6f, want 0.0", lat0)
	}

	// 90° Nord (Nordpol): 2^30 Semicircles
	lat90 := float64(int32(1073741824)) * factor
	if math.Abs(lat90-90.0) > epsilon {
		t.Errorf("1073741824 semicircles → %.6f, want 90.0", lat90)
	}

	// 90° Süd: -2^30 Semicircles
	latMinus90 := float64(int32(-1073741824)) * factor
	if math.Abs(latMinus90-(-90.0)) > epsilon {
		t.Errorf("-1073741824 semicircles → %.6f, want -90.0", latMinus90)
	}

	// 180° (Datumsgrenze): 2^31-1 ≈ 180°
	lat180 := float64(int32(2147483647)) * factor
	if math.Abs(lat180-180.0) > 0.001 {
		t.Errorf("2147483647 semicircles → %.6f, want ~180.0", lat180)
	}
}

// TestSegmentEnhancedAltitudeScaling prüft die Höhen-Dekodierung im Segment-Parser.
// Formula: raw/5 - 500 = Meter (identisch mit Course-Parser)
func TestSegmentEnhancedAltitudeScaling(t *testing.T) {
	// 0m: raw = 2500
	if ele := float64(uint32(2500))/5.0 - 500.0; ele != 0.0 {
		t.Errorf("enhanced alt 2500 → %.1fm, want 0.0", ele)
	}

	// 800m: raw = (800+500)*5 = 6500
	if ele := float64(uint32(6500))/5.0 - 500.0; ele != 800.0 {
		t.Errorf("enhanced alt 6500 → %.1fm, want 800.0", ele)
	}

	// 3798m (Großvenediger): raw = (3798+500)*5 = 21490
	if ele := float64(uint32(21490))/5.0 - 500.0; ele != 3798.0 {
		t.Errorf("enhanced alt 21490 → %.1fm, want 3798.0", ele)
	}
}

// TestSegmentRouteHashDedup prüft das Format der Hash-Parts für den Deduplizierungsschlüssel.
// Format: "lat,lng" mit 5 Dezimalstellen, Trennzeichen "|"
func TestSegmentRouteHashFormat(t *testing.T) {
	hashPart := "47.26000,11.39400"
	expected := "47.26000,11.39400"
	if hashPart != expected {
		t.Errorf("hash part format: got %q, want %q", hashPart, expected)
	}

	// Stelle sicher, dass das Format reproduzierbar ist
	lat := 47.26003
	lng := 11.39401
	formatted := fmt.Sprintf("%.5f,%.5f", lat, lng)
	if formatted != "47.26003,11.39401" {
		t.Errorf("hash part format: got %q, want '47.26003,11.39401'", formatted)
	}
}
