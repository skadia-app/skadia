package garmin

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/kit/datetime"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// minSaneTimestamp is the earliest timestamp we accept from FIT files.
var minSaneTimestamp = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

// processMonitoringBFIT parses Garmin monitoring FIT file types 15, 28, 32.
// These files contain high-resolution sensor data: heart rate, stress, body battery,
// respiration, SpO2, wrist temperature, and activity type/intensity.
//
// Two-pass approach:
//  1. Collect all messages into typed slices, track HSA presence flags.
//  2. Merge into final []model.MonitoringSample with source priority (HSA > legacy).
func processMonitoringBFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("decode monitoring_b FIT: %w", err)
	}

	// Derive file timestamp from first valid message timestamp for ShadowFS tags.
	fileTS := time.Now().UTC()
	for i := range fit.Messages {
		rawTS := fit.Messages[i].FieldValueByNum(253) // field 253 = timestamp
		if rawTS.Valid(basetype.Uint32) {
			ts := datetime.ToTime(rawTS.Uint32())
			if !ts.IsZero() && !ts.Before(minSaneTimestamp) {
				fileTS = ts
				break
			}
		}
	}
	storeHealthFIT(ctx, db, fs, data, userID, "monitoring_b", fmt.Sprintf("monitoring_b_%s.fit", fileTS.Format("20060102_150405")), fileTS)

	// --- Pass 1: Collect all messages into typed slices ---

	type monSample struct {
		ts   time.Time
		s    model.MonitoringSample
		from string // source tag for priority resolution
	}

	var (
		// Legacy sources
		legacyMonitoring []monSample // msg 55
		legacyStress     []monSample // msg 227 stress
		legacyBB         []monSample // msg 227 body battery
		legacyResp       []monSample // msg 297
		legacySpo2       []monSample // msg 269

		// HSA sources
		hsaStress   []monSample // msg 306
		hsaBB       []monSample // msg 314
		hsaResp     []monSample // msg 307
		hsaHR       []monSample // msg 308
		hsaSpo2     []monSample // msg 305
		hsaWristT   []monSample // msg 409

		hasHsaStress bool
		hasHsaBB     bool
		hasHsaSpo2   bool

		restingHRByDate = make(map[string]int)
	)

	for i := range fit.Messages {
		msg := &fit.Messages[i]

		switch msg.Num {
		case typedef.MesgNumMonitoring: // 55 — non-cumulative fields only
			mon := mesgdef.NewMonitoring(msg)
			if mon.Timestamp.IsZero() || mon.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			s := model.MonitoringSample{
				UserID: userID,
				TS:     mon.Timestamp,
			}
			hasField := false

			// HeartRate (field 27, uint8)
			if mon.HeartRate != basetype.Uint8Invalid && mon.HeartRate > 0 {
				hr := int16(mon.HeartRate)
				s.HeartRate = &hr
				hasField = true
			}

			// ActivityType (field 5, byte→int16)
			if mon.ActivityType != typedef.ActivityTypeInvalid {
				at := int16(mon.ActivityType)
				s.ActivityType = &at
				hasField = true
			}

			// Intensity (field 28, uint8 RAW — NOT divided by scale 10)
			if mon.Intensity != basetype.Uint8Invalid {
				intensity := int16(mon.Intensity)
				s.Intensity = &intensity
				hasField = true
			}

			// Temperature (field 12, int16 /100)
			if mon.Temperature != basetype.Sint16Invalid {
				temp := float64(mon.Temperature) / 100.0
				s.TemperatureC = &temp
				hasField = true
			}

			// TemperatureMin (field 14, int16 /100)
			if mon.TemperatureMin != basetype.Sint16Invalid {
				temp := float64(mon.TemperatureMin) / 100.0
				s.TemperatureMinC = &temp
				hasField = true
			}

			// TemperatureMax (field 15, int16 /100)
			if mon.TemperatureMax != basetype.Sint16Invalid {
				temp := float64(mon.TemperatureMax) / 100.0
				s.TemperatureMaxC = &temp
				hasField = true
			}

			if hasField {
				legacyMonitoring = append(legacyMonitoring, monSample{ts: s.TS, s: s, from: "mon55"})
			}

		case typedef.MesgNumStressLevel: // 227 — stress + body battery (undocumented)
			sl := mesgdef.NewStressLevel(msg)
			ts := sl.StressLevelTime // NOTE: use StressLevelTime, NOT Timestamp
			if ts.IsZero() || ts.Before(minSaneTimestamp) {
				continue
			}

			// Stress: field 0 (int16), only store 0-100; negative → NULL
			if sl.StressLevelValue != basetype.Sint16Invalid && sl.StressLevelValue >= 0 && sl.StressLevelValue <= 100 {
				stress := sl.StressLevelValue
				legacyStress = append(legacyStress, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID: userID,
						TS:     ts,
						Stress: &stress,
					},
					from: "stress227",
				})
			}

			// Body Battery: undocumented field 3 (int8, 0-100)
			bbVal := msg.FieldValueByNum(3)
			if bbVal.Valid(basetype.Sint8) {
				bb := bbVal.Int8()
				if bb >= 0 && bb <= 100 {
					bbInt16 := int16(bb)
					sample := monSample{
						ts: ts,
						s: model.MonitoringSample{
							UserID:      userID,
							TS:          ts,
							BodyBattery: &bbInt16,
						},
						from: "bb227",
					}
					// Body Battery Delta: undocumented field 2 (int8)
					bbDeltaVal := msg.FieldValueByNum(2)
					if bbDeltaVal.Valid(basetype.Sint8) {
						delta := int16(bbDeltaVal.Int8())
						sample.s.BodyBatteryDelta = &delta
					}
					legacyBB = append(legacyBB, sample)
				}
			}

		case typedef.MesgNumHsaStressData: // 306 — HSA stress array
			hsa := mesgdef.NewHsaStressData(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			hasHsaStress = true
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, v := range hsa.StressLevel {
				// Negative values are blanks/errors: -1 off wrist, -2 motion, -3 not enough data,
				// -4 recovering, -5 unidentified, -16 blank → all NULL
				if v < 0 || v > 100 {
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				stress := int16(v)
				hsaStress = append(hsaStress, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID: userID,
						TS:     ts,
						Stress: &stress,
					},
					from: "hsa_stress",
				})
			}

		case typedef.MesgNumRespirationRate: // 297 — legacy respiration
			rr := mesgdef.NewRespirationRate(msg)
			if rr.Timestamp.IsZero() || rr.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			raw := rr.RespirationRate
			// Special blank markers: -300, -200, -100, and basetype invalid
			if raw == basetype.Sint16Invalid || raw == -300 || raw == -200 || raw == -100 {
				continue
			}
			scaled := float64(raw) / 100.0
			if scaled <= 0 || scaled > 100 {
				continue
			}
			legacyResp = append(legacyResp, monSample{
				ts: rr.Timestamp,
				s: model.MonitoringSample{
					UserID:          userID,
					TS:              rr.Timestamp,
					RespirationRate: &scaled,
				},
				from: "resp297",
			})

		case typedef.MesgNumHsaRespirationData: // 307 — HSA respiration array
			hsa := mesgdef.NewHsaRespirationData(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, raw := range hsa.RespirationRate {
				// Blanks: raw 240, 25200, 25300, 25400, 25500, and basetype.Sint16Invalid(32767)
				if raw == basetype.Sint16Invalid || raw == 240 || raw == 25200 || raw == 25300 || raw == 25400 || raw == 25500 {
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				scaled := float64(raw) / 100.0
				if scaled <= 0 || scaled > 100 {
					continue
				}
				hsaResp = append(hsaResp, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID:          userID,
						TS:              ts,
						RespirationRate: &scaled,
					},
					from: "hsa_resp",
				})
			}

		case typedef.MesgNumHsaHeartRateData: // 308 — HSA HR, status-gated
			hsa := mesgdef.NewHsaHeartRateData(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			// Status: 0=SEARCHING, 1=LOCKED. Only import if locked.
			if hsa.Status != 1 {
				continue
			}
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, hr := range hsa.HeartRate {
				if hr == 0 { // HR=0 → NULL
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				hrVal := int16(hr)
				hsaHR = append(hsaHR, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID:    userID,
						TS:        ts,
						HeartRate: &hrVal,
					},
					from: "hsa_hr",
				})
			}

		case typedef.MesgNumHsaBodyBatteryData: // 314 — HSA body battery array
			hsa := mesgdef.NewHsaBodyBatteryData(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			hasHsaBB = true
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, v := range hsa.Level {
				// Blank: -16 → NULL. Valid range: 0-100.
				if v == -16 || v < 0 || v > 100 {
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				bb := int16(v)
				hsaBB = append(hsaBB, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID:      userID,
						TS:          ts,
						BodyBattery: &bb,
					},
					from: "hsa_bb",
				})
			}

		case typedef.MesgNumHsaWristTemperatureData: // 409 — HSA wrist temperature array
			hsa := mesgdef.NewHsaWristTemperatureData(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, raw := range hsa.Value {
				if raw == basetype.Uint16Invalid {
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				tempC := float64(raw) / 1000.0
				hsaWristT = append(hsaWristT, monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID:            userID,
						TS:                ts,
						WristTemperatureC: &tempC,
					},
					from: "hsa_wrist_t",
				})
			}

		case typedef.MesgNumSpo2Data: // 269 — legacy SpO2
			spo2 := mesgdef.NewSpo2Data(msg)
			if spo2.Timestamp.IsZero() || spo2.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			if spo2.ReadingSpo2 == 0 || spo2.ReadingSpo2 == basetype.Uint8Invalid {
				continue
			}
			spo2Val := int16(spo2.ReadingSpo2)
			sample := monSample{
				ts: spo2.Timestamp,
				s: model.MonitoringSample{
					UserID:  userID,
					TS:      spo2.Timestamp,
					SpO2Pct: &spo2Val,
				},
				from: "spo2_269",
			}
			if spo2.ReadingConfidence != basetype.Uint8Invalid {
				conf := int16(spo2.ReadingConfidence)
				sample.s.SpO2Confidence = &conf
			}
			legacySpo2 = append(legacySpo2, sample)

		case typedef.MesgNumHsaSpo2Data: // 305 — HSA SpO2 array
			hsa := mesgdef.NewHsaSpo2Data(msg)
			if hsa.Timestamp.IsZero() || hsa.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			hasHsaSpo2 = true
			interval := hsaInterval(hsa.ProcessingInterval)
			for i, reading := range hsa.ReadingSpo2 {
				// Blank: 240 → NULL, also skip 0 and invalid
				if reading == 240 || reading == 0 || reading == basetype.Uint8Invalid {
					continue
				}
				ts := hsa.Timestamp.Add(time.Duration(i) * interval)
				if ts.Before(minSaneTimestamp) {
					continue
				}
				spo2Val := int16(reading)
				sample := monSample{
					ts: ts,
					s: model.MonitoringSample{
						UserID:  userID,
						TS:      ts,
						SpO2Pct: &spo2Val,
					},
					from: "hsa_spo2",
				}
				if i < len(hsa.Confidence) && hsa.Confidence[i] != basetype.Uint8Invalid {
					conf := int16(hsa.Confidence[i])
					sample.s.SpO2Confidence = &conf
				}
				hsaSpo2 = append(hsaSpo2, sample)
			}

		case 211: // MonitoringHrData — resting HR
			mhr := mesgdef.NewMonitoringHrData(msg)
			if mhr.Timestamp.IsZero() || mhr.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			rhr := 0
			if mhr.CurrentDayRestingHeartRate != 0xFF {
				rhr = int(mhr.CurrentDayRestingHeartRate)
			} else if mhr.RestingHeartRate != 0xFF {
				rhr = int(mhr.RestingHeartRate)
			}
			if rhr > 0 && rhr < 250 {
				calDate := mhr.Timestamp.UTC().Format("2006-01-02")
				restingHRByDate[calDate] = rhr
			}
		}
	}

	// --- Pass 2: Merge with source priority ---
	// Map keyed by timestamp for merging multiple fields into one sample.
	merged := make(map[time.Time]*model.MonitoringSample)

	getOrCreate := func(ts time.Time) *model.MonitoringSample {
		if s, ok := merged[ts]; ok {
			return s
		}
		s := &model.MonitoringSample{UserID: userID, TS: ts}
		merged[ts] = s
		return s
	}

	// Always include: monitoring (msg 55), HSA HR, HSA respiration, HSA wrist temp
	for _, m := range legacyMonitoring {
		s := getOrCreate(m.ts)
		mergeMonitoringSample(s, &m.s)
	}
	for _, m := range hsaHR {
		s := getOrCreate(m.ts)
		mergeMonitoringSample(s, &m.s)
	}
	for _, m := range hsaWristT {
		s := getOrCreate(m.ts)
		mergeMonitoringSample(s, &m.s)
	}

	// Stress: prefer HSA if present, else legacy
	if hasHsaStress {
		for _, m := range hsaStress {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	} else {
		for _, m := range legacyStress {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	}

	// Body Battery: prefer HSA if present, else legacy (from msg 227)
	if hasHsaBB {
		for _, m := range hsaBB {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	} else {
		for _, m := range legacyBB {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	}

	// Respiration: HSA replaces legacy (no flag needed — HSA is always preferred)
	if len(hsaResp) > 0 {
		for _, m := range hsaResp {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	} else {
		for _, m := range legacyResp {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	}

	// SpO2: prefer HSA if present, else legacy
	if hasHsaSpo2 {
		for _, m := range hsaSpo2 {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	} else {
		for _, m := range legacySpo2 {
			s := getOrCreate(m.ts)
			mergeMonitoringSample(s, &m.s)
		}
	}

	// NOTE: When HSA stress is present, we still want Body Battery from msg 227
	// if there is no HSA BB. The hasHsaStress flag only suppresses stress from 227,
	// not BB. This is already handled above since legacyStress and legacyBB are
	// separate slices with independent priority flags.

	// Build final slice
	samples := make([]model.MonitoringSample, 0, len(merged))
	for _, s := range merged {
		samples = append(samples, *s)
	}

	if len(samples) > 0 {
		if err := db.InsertMonitoringSamples(ctx, samples); err != nil {
			return fmt.Errorf("insert monitoring samples: %w", err)
		}
		slog.Info("processMonitoringBFIT: inserted samples",
			"user_id", userID,
			"count", len(samples),
			"hsa_stress", hasHsaStress,
			"hsa_bb", hasHsaBB,
			"hsa_spo2", hasHsaSpo2,
		)
	}

	// Upsert resting HR
	for calDate, rhr := range restingHRByDate {
		rhrVal := rhr
		summary := &model.HealthDailySummary{
			UserID:           userID,
			CalendarDate:     calDate,
			RestingHeartRate: &rhrVal,
			SourceType:       "garmin_fit",
		}
		if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
			slog.Warn("processMonitoringBFIT: upsert resting HR failed", "date", calDate, "error", err)
		}
	}

	return nil
}

// hsaInterval returns the processing interval as a time.Duration.
// Falls back to 60s if the interval is zero.
func hsaInterval(processingInterval uint16) time.Duration {
	if processingInterval == 0 || processingInterval == basetype.Uint16Invalid {
		return 60 * time.Second
	}
	return time.Duration(processingInterval) * time.Second
}

// mergeMonitoringSample merges non-nil fields from src into dst.
// Existing non-nil fields in dst are NOT overwritten (first writer wins).
func mergeMonitoringSample(dst, src *model.MonitoringSample) {
	if dst.HeartRate == nil && src.HeartRate != nil {
		dst.HeartRate = src.HeartRate
	}
	if dst.ActivityType == nil && src.ActivityType != nil {
		dst.ActivityType = src.ActivityType
	}
	if dst.Intensity == nil && src.Intensity != nil {
		dst.Intensity = src.Intensity
	}
	if dst.TemperatureC == nil && src.TemperatureC != nil {
		dst.TemperatureC = src.TemperatureC
	}
	if dst.TemperatureMinC == nil && src.TemperatureMinC != nil {
		dst.TemperatureMinC = src.TemperatureMinC
	}
	if dst.TemperatureMaxC == nil && src.TemperatureMaxC != nil {
		dst.TemperatureMaxC = src.TemperatureMaxC
	}
	if dst.Stress == nil && src.Stress != nil {
		dst.Stress = src.Stress
	}
	if dst.BodyBattery == nil && src.BodyBattery != nil {
		dst.BodyBattery = src.BodyBattery
	}
	if dst.BodyBatteryDelta == nil && src.BodyBatteryDelta != nil {
		dst.BodyBatteryDelta = src.BodyBatteryDelta
	}
	if dst.RespirationRate == nil && src.RespirationRate != nil {
		dst.RespirationRate = src.RespirationRate
	}
	if dst.SpO2Pct == nil && src.SpO2Pct != nil {
		dst.SpO2Pct = src.SpO2Pct
	}
	if dst.SpO2Confidence == nil && src.SpO2Confidence != nil {
		dst.SpO2Confidence = src.SpO2Confidence
	}
	if dst.WristTemperatureC == nil && src.WristTemperatureC != nil {
		dst.WristTemperatureC = src.WristTemperatureC
	}
}
