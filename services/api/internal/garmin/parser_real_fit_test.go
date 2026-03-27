package garmin

import (
	"bytes"
	"os"
	"testing"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/kit/datetime"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

// TestRealMonitoringB_BodyBattery parses a real 56KB monitoring_b FIT file from a
// Garmin export and validates that Body Battery values are correctly extracted from
// undocumented msg 227 field 3 (int8, 0-100).
func TestRealMonitoringB_BodyBattery(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_type32_monitoring_b_large.fit")
	if err != nil {
		t.Skip("test fixture not available:", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	var stressCount, bbCount, bbDeltaCount, hrCount, respCount int
	var bbMin, bbMax int8

	bbMin = 127
	bbMax = -128

	for i := range fit.Messages {
		msg := &fit.Messages[i]

		switch msg.Num {
		case typedef.MesgNumStressLevel: // 227
			sl := mesgdef.NewStressLevel(msg)

			// Stress: check filtering
			if sl.StressLevelValue != basetype.Sint16Invalid && sl.StressLevelValue >= 0 && sl.StressLevelValue <= 100 {
				stressCount++
			}

			// Body Battery: undocumented field 3, int8
			if bbVal := msg.FieldValueByNum(3); bbVal.Valid(basetype.Sint8) {
				bb := bbVal.Int8()
				if bb >= 0 && bb <= 100 {
					bbCount++
					if bb < bbMin {
						bbMin = bb
					}
					if bb > bbMax {
						bbMax = bb
					}
				}
			}

			// Body Battery Delta: undocumented field 2, int8
			if deltaVal := msg.FieldValueByNum(2); deltaVal.Valid(basetype.Sint8) {
				bbDeltaCount++
			}

		case typedef.MesgNumMonitoring: // 55
			mon := mesgdef.NewMonitoring(msg)
			if mon.HeartRate != basetype.Uint8Invalid && mon.HeartRate > 0 {
				hrCount++
			}

		case typedef.MesgNumRespirationRate: // 297
			rr := mesgdef.NewRespirationRate(msg)
			if rr.RespirationRate != basetype.Sint16Invalid &&
				rr.RespirationRate != -300 && rr.RespirationRate != -200 && rr.RespirationRate != -100 {
				respCount++
			}
		}
	}

	t.Logf("Real monitoring_b parse results:")
	t.Logf("  Stress samples (0-100): %d", stressCount)
	t.Logf("  Body Battery samples:   %d (range %d-%d)", bbCount, bbMin, bbMax)
	t.Logf("  Body Battery deltas:    %d", bbDeltaCount)
	t.Logf("  Heart Rate samples:     %d", hrCount)
	t.Logf("  Respiration samples:    %d", respCount)

	// A 56KB monitoring_b file from a full day MUST have data
	if stressCount == 0 {
		t.Error("expected non-zero stress samples from real monitoring_b file")
	}
	if bbCount == 0 {
		t.Error("expected non-zero Body Battery samples from real monitoring_b file (msg 227 field 3)")
	}
	if bbMin < 0 || bbMax > 100 {
		t.Errorf("Body Battery out of range: min=%d, max=%d (expected 0-100)", bbMin, bbMax)
	}
	if hrCount == 0 {
		t.Error("expected non-zero heart rate samples from real monitoring_b file")
	}
}

// TestRealMonitoringB_HSAMessages checks if HSA messages (306, 307, 308, 314, 409)
// are present in modern Garmin monitoring files. Older devices only have legacy messages.
func TestRealMonitoringB_HSAMessages(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_type32_monitoring_b_large.fit")
	if err != nil {
		t.Skip("test fixture not available:", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	msgCounts := map[typedef.MesgNum]int{}
	for i := range fit.Messages {
		msgCounts[fit.Messages[i].Num]++
	}

	t.Logf("Message type distribution in real monitoring_b:")
	hsaTypes := map[typedef.MesgNum]string{
		55: "Monitoring", 227: "StressLevel", 297: "RespirationRate",
		211: "MonitoringHrData",
		306: "HsaStressData", 307: "HsaRespirationData", 308: "HsaHeartRateData",
		314: "HsaBodyBatteryData", 305: "HsaSpo2Data", 409: "HsaWristTemperatureData",
	}
	for num, name := range hsaTypes {
		if count, ok := msgCounts[num]; ok {
			t.Logf("  msg %3d (%s): %d", num, name, count)
		}
	}

	// At minimum, a monitoring_b file must have msg 55 or msg 227
	if msgCounts[55] == 0 && msgCounts[227] == 0 {
		t.Error("expected at least msg 55 (Monitoring) or msg 227 (StressLevel)")
	}
}

// TestRealHRVNight_TrainingReadiness parses a real HRV night FIT file and checks
// for msg 369 (Training Readiness, proprietary). Not all HRV night files have it —
// it depends on device firmware version.
func TestRealHRVNight_TrainingReadiness(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_type44_hrv_night_large.fit")
	if err != nil {
		t.Skip("test fixture not available:", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	var hrv281Count, tr369Count int
	var trScore, trSleepScore int16
	var hasScore, hasSleepScore bool

	for i := range fit.Messages {
		msg := &fit.Messages[i]

		switch msg.Num {
		case 281: // Proprietary HRV night values
			rawTS := msg.FieldValueByNum(253).Uint32()
			rawVal := msg.FieldValueByNum(0).Uint16()
			if rawTS != basetype.Uint32Invalid && rawVal != basetype.Uint16Invalid {
				hrv281Count++
			}

		case 369: // Proprietary Training Readiness
			rawTS := msg.FieldValueByNum(253).Uint32()
			if rawTS == basetype.Uint32Invalid {
				continue
			}
			tr369Count++
			ts := datetime.ToTime(rawTS)
			t.Logf("  Training Readiness timestamp: %s", ts.Format("2006-01-02 15:04:05"))

			// Field 0: score
			if v := msg.FieldValueByNum(0); v.Valid(basetype.Sint16) {
				trScore = v.Int16()
				hasScore = true
				t.Logf("  Score: %d", trScore)
			}
			// Field 5: sleep_score
			if v := msg.FieldValueByNum(5); v.Valid(basetype.Sint16) {
				trSleepScore = v.Int16()
				hasSleepScore = true
				t.Logf("  SleepScore: %d", trSleepScore)
			}
			// Field 8: recovery_time_factor_pct
			if v := msg.FieldValueByNum(8); v.Valid(basetype.Sint16) {
				t.Logf("  RecoveryTimeFactorPct: %d", v.Int16())
			}
			// Field 10: acwr_pct
			if v := msg.FieldValueByNum(10); v.Valid(basetype.Sint16) {
				t.Logf("  AcwrPct: %d", v.Int16())
			}
			// Field 12: stress_history_factor_pct
			if v := msg.FieldValueByNum(12); v.Valid(basetype.Sint16) {
				t.Logf("  StressHistoryFactorPct: %d", v.Int16())
			}
			// Field 14: hrv_factor_pct
			if v := msg.FieldValueByNum(14); v.Valid(basetype.Sint16) {
				t.Logf("  HRVFactorPct: %d", v.Int16())
			}
			// Field 16: sleep_history_factor_pct
			if v := msg.FieldValueByNum(16); v.Valid(basetype.Sint16) {
				t.Logf("  SleepHistoryFactorPct: %d", v.Int16())
			}
		}
	}

	t.Logf("Real HRV night parse results:")
	t.Logf("  HRV night values (msg 281): %d", hrv281Count)
	t.Logf("  Training Readiness (msg 369): %d", tr369Count)

	// HRV night values OR Training Readiness must be present
	if hrv281Count == 0 && tr369Count == 0 {
		t.Error("expected at least msg 281 (HRV) or msg 369 (Training Readiness) from real HRV night file")
	}

	// Training Readiness is optional (firmware-dependent), but log presence
	if tr369Count > 0 {
		t.Logf("Training Readiness FOUND — device supports msg 369")
		if hasScore && (trScore < 0 || trScore > 100) {
			t.Errorf("Training Readiness score out of range: %d", trScore)
		}
		if hasSleepScore && (trSleepScore < 0 || trSleepScore > 100) {
			t.Errorf("Training Readiness sleep_score out of range: %d", trSleepScore)
		}
	} else {
		t.Log("Training Readiness NOT in this file (older device/firmware) — not an error")
	}
}

// TestRealSleep_Levels parses a real sleep FIT file and validates SleepLevel extraction.
func TestRealSleep_Levels(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_type49_sleep.fit")
	if err != nil {
		t.Skip("test fixture not available:", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	var sleepLevelCount, assessmentCount, eventCount int
	levelDist := map[uint8]int{}

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumSleepLevel: // 275
			sl := mesgdef.NewSleepLevel(msg)
			if sl.SleepLevel != typedef.SleepLevelInvalid && sl.SleepLevel <= 4 {
				sleepLevelCount++
				levelDist[uint8(sl.SleepLevel)]++
			}
		case typedef.MesgNumSleepAssessment: // 346
			assessmentCount++
		case typedef.MesgNumEvent: // 21
			evtType := msg.FieldValueByNum(0)
			if evtType.Valid(basetype.Uint8) && evtType.Uint8() == 74 {
				eventCount++
			}
		}
	}

	t.Logf("Real sleep file parse results:")
	t.Logf("  SleepLevel entries: %d", sleepLevelCount)
	t.Logf("  SleepAssessment: %d", assessmentCount)
	t.Logf("  Sleep Events (74): %d", eventCount)
	names := map[uint8]string{0: "unmeasurable", 1: "awake", 2: "light", 3: "deep", 4: "rem"}
	for level, count := range levelDist {
		t.Logf("    Level %d (%s): %d", level, names[level], count)
	}

	if sleepLevelCount == 0 {
		t.Error("expected non-zero SleepLevel entries from real sleep file")
	}
}

// TestRealSpO2_Snapshots parses a real SpO2 FIT file.
func TestRealSpO2_Snapshots(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_type58_spo2.fit")
	if err != nil {
		t.Skip("test fixture not available:", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	var std269, hsa305, proprietary318 int
	for i := range fit.Messages {
		switch fit.Messages[i].Num {
		case typedef.MesgNumSpo2Data:
			std269++
		case 305:
			hsa305++
		case 318:
			proprietary318++
		}
	}

	t.Logf("Real SpO2 file message distribution:")
	t.Logf("  Standard Spo2Data (269): %d", std269)
	t.Logf("  HSA Spo2Data (305): %d", hsa305)
	t.Logf("  Proprietary (318): %d (skipped by parser)", proprietary318)

	if std269 == 0 && hsa305 == 0 {
		t.Log("No standard SpO2 messages — file may only contain proprietary msg 318")
	}
}
