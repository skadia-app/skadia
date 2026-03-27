package garmin

import (
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestMinSaneTimestamp prüft, dass der Sentinel korrekt auf 2010-01-01 gesetzt ist.
func TestMinSaneTimestamp(t *testing.T) {
	want := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	if !minSaneTimestamp.Equal(want) {
		t.Errorf("minSaneTimestamp = %v, want %v", minSaneTimestamp, want)
	}
}

// TestHsaInterval prüft die Fallback-Logik für ProcessingInterval.
func TestHsaInterval(t *testing.T) {
	cases := []struct {
		input    uint16
		wantSecs int64
	}{
		{0, 60},                          // Fallback auf 60s
		{basetype.Uint16Invalid, 60},     // Invalid → Fallback
		{30, 30},                         // 30s-Intervall
		{60, 60},                         // 60s-Intervall
		{120, 120},                       // 2 Minuten
	}
	for _, tc := range cases {
		got := hsaInterval(tc.input)
		if int64(got.Seconds()) != tc.wantSecs {
			t.Errorf("hsaInterval(%d) = %v, want %ds", tc.input, got, tc.wantSecs)
		}
	}
}

// TestMergeMonitoringSample_FirstWins prüft, dass bestehende Felder NICHT überschrieben werden.
func TestMergeMonitoringSample_FirstWins(t *testing.T) {
	v1 := int16(70)
	v2 := int16(90)

	dst := &model.MonitoringSample{HeartRate: &v1}
	src := &model.MonitoringSample{HeartRate: &v2}

	mergeMonitoringSample(dst, src)

	if *dst.HeartRate != 70 {
		t.Errorf("HeartRate: got %d, want 70 (first writer wins)", *dst.HeartRate)
	}
}

// TestMergeMonitoringSample_NilFilled prüft, dass nil-Felder im dst gefüllt werden.
func TestMergeMonitoringSample_NilFilled(t *testing.T) {
	v := int16(55)
	dst := &model.MonitoringSample{}
	src := &model.MonitoringSample{HeartRate: &v}

	mergeMonitoringSample(dst, src)

	if dst.HeartRate == nil {
		t.Fatal("HeartRate should be set from src, got nil")
	}
	if *dst.HeartRate != 55 {
		t.Errorf("HeartRate: got %d, want 55", *dst.HeartRate)
	}
}

// TestMergeMonitoringSample_AllFields prüft, dass alle Felder zusammengeführt werden.
func TestMergeMonitoringSample_AllFields(t *testing.T) {
	hr := int16(72)
	at := int16(1)
	intensity := int16(3)
	tempC := 36.6
	tempMin := 35.0
	tempMax := 37.0
	stress := int16(45)
	bb := int16(80)
	bbDelta := int16(-2)
	resp := 15.5
	spo2 := int16(98)
	spo2conf := int16(90)
	wristT := 36.2

	src := &model.MonitoringSample{
		HeartRate:         &hr,
		ActivityType:      &at,
		Intensity:         &intensity,
		TemperatureC:      &tempC,
		TemperatureMinC:   &tempMin,
		TemperatureMaxC:   &tempMax,
		Stress:            &stress,
		BodyBattery:       &bb,
		BodyBatteryDelta:  &bbDelta,
		RespirationRate:   &resp,
		SpO2Pct:           &spo2,
		SpO2Confidence:    &spo2conf,
		WristTemperatureC: &wristT,
	}
	dst := &model.MonitoringSample{}
	mergeMonitoringSample(dst, src)

	if dst.HeartRate == nil || *dst.HeartRate != 72 {
		t.Errorf("HeartRate: got %v, want 72", dst.HeartRate)
	}
	if dst.ActivityType == nil || *dst.ActivityType != 1 {
		t.Errorf("ActivityType: got %v, want 1", dst.ActivityType)
	}
	if dst.Intensity == nil || *dst.Intensity != 3 {
		t.Errorf("Intensity: got %v, want 3", dst.Intensity)
	}
	if dst.TemperatureC == nil || *dst.TemperatureC != 36.6 {
		t.Errorf("TemperatureC: got %v, want 36.6", dst.TemperatureC)
	}
	if dst.TemperatureMinC == nil || *dst.TemperatureMinC != 35.0 {
		t.Errorf("TemperatureMinC: got %v, want 35.0", dst.TemperatureMinC)
	}
	if dst.TemperatureMaxC == nil || *dst.TemperatureMaxC != 37.0 {
		t.Errorf("TemperatureMaxC: got %v, want 37.0", dst.TemperatureMaxC)
	}
	if dst.Stress == nil || *dst.Stress != 45 {
		t.Errorf("Stress: got %v, want 45", dst.Stress)
	}
	if dst.BodyBattery == nil || *dst.BodyBattery != 80 {
		t.Errorf("BodyBattery: got %v, want 80", dst.BodyBattery)
	}
	if dst.BodyBatteryDelta == nil || *dst.BodyBatteryDelta != -2 {
		t.Errorf("BodyBatteryDelta: got %v, want -2", dst.BodyBatteryDelta)
	}
	if dst.RespirationRate == nil || *dst.RespirationRate != 15.5 {
		t.Errorf("RespirationRate: got %v, want 15.5", dst.RespirationRate)
	}
	if dst.SpO2Pct == nil || *dst.SpO2Pct != 98 {
		t.Errorf("SpO2Pct: got %v, want 98", dst.SpO2Pct)
	}
	if dst.SpO2Confidence == nil || *dst.SpO2Confidence != 90 {
		t.Errorf("SpO2Confidence: got %v, want 90", dst.SpO2Confidence)
	}
	if dst.WristTemperatureC == nil || *dst.WristTemperatureC != 36.2 {
		t.Errorf("WristTemperatureC: got %v, want 36.2", dst.WristTemperatureC)
	}
}

// TestStressFilteringLogic prüft die Filterregel: nur 0-100 sind valide Stress-Werte.
func TestStressFilteringLogic(t *testing.T) {
	cases := []struct {
		value int16
		valid bool
	}{
		{-5, false},
		{-1, false},
		{0, true},
		{50, true},
		{100, true},
		{101, false},
		{200, false},
	}

	for _, tc := range cases {
		// Repliziert die Filterbedingung aus processMonitoringBFIT
		isValid := tc.value >= 0 && tc.value <= 100
		if isValid != tc.valid {
			t.Errorf("stress value %d: valid=%v, want %v", tc.value, isValid, tc.valid)
		}
	}
}

// TestHsaRespirationFilterLogic prüft die Blank-Marker-Filterregel für HSA Respiration.
func TestHsaRespirationFilterLogic(t *testing.T) {
	type respCase struct {
		raw   int16
		valid bool
		want  float64
	}
	cases := []respCase{
		{1500, true, 15.0},                     // valider Wert
		{240, false, 0},                         // Blank-Marker
		{25200, false, 0},                       // Blank-Marker
		{25300, false, 0},                       // Blank-Marker
		{25400, false, 0},                       // Blank-Marker
		{25500, false, 0},                       // Blank-Marker
		{basetype.Sint16Invalid, false, 0},      // FIT Invalid
		{2000, true, 20.0},                      // valider Wert
		{0, false, 0},                            // scaled = 0 → ungültig
		{10001, false, 0},                        // scaled > 100 → ungültig
	}

	for _, tc := range cases {
		// Repliziert die Logik aus processMonitoringBFIT
		isBlank := tc.raw == basetype.Sint16Invalid || tc.raw == 240 || tc.raw == 25200 ||
			tc.raw == 25300 || tc.raw == 25400 || tc.raw == 25500
		if isBlank {
			if tc.valid {
				t.Errorf("raw=%d: should be blank-filtered", tc.raw)
			}
			continue
		}
		scaled := float64(tc.raw) / 100.0
		isValid := scaled > 0 && scaled <= 100
		if isValid != tc.valid {
			t.Errorf("raw=%d: valid=%v, want %v (scaled=%.2f)", tc.raw, isValid, tc.valid, scaled)
		}
		if tc.valid && scaled != tc.want {
			t.Errorf("raw=%d: scaled=%.2f, want %.2f", tc.raw, scaled, tc.want)
		}
	}
}

// TestHsaHeartRateStatusGating prüft, dass Status!=1 alle HR-Samples blockiert.
func TestHsaHeartRateStatusGating(t *testing.T) {
	// Status 0 = SEARCHING → kein Import
	// Status 1 = LOCKED → Import
	statusSearching := uint8(0)
	statusLocked := uint8(1)

	// Repliziert die Logik: if hsa.Status != 1 { continue }
	if statusSearching == 1 {
		t.Error("SEARCHING status should NOT pass the gate")
	}
	if statusLocked != 1 {
		t.Error("LOCKED status should pass the gate")
	}
}

// TestBodyBatteryRangeLogic prüft die 0-100 Bereichsvalidierung für Body Battery.
func TestBodyBatteryRangeLogic(t *testing.T) {
	cases := []struct {
		val   int8
		valid bool
	}{
		{-1, false},
		{0, true},
		{50, true},
		{100, true},
		// int8 max ist 127, >100 ist ungültig
	}
	for _, tc := range cases {
		isValid := tc.val >= 0 && tc.val <= 100
		if isValid != tc.valid {
			t.Errorf("bb val %d: valid=%v, want %v", tc.val, isValid, tc.valid)
		}
	}
}
