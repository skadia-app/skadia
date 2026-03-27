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

var minValidTS = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

// loadUserTimezone loads the timezone string for a user from the database.
// Returns "UTC" as fallback if the query fails or the value is empty.
func loadUserTimezone(ctx context.Context, db *database.DB, userID string) string {
	var tz string
	err := db.Pool.QueryRow(ctx, `SELECT timezone FROM users WHERE id = $1`, userID).Scan(&tz)
	if err != nil || tz == "" {
		return "UTC"
	}
	return tz
}

// processSleepFIT decodes a Garmin sleep FIT file (type 49) and persists:
//   - Sleep levels (msg 275) as MonitoringSamples
//   - Sleep assessment scores (msg 346) as HealthSleep
//   - Sleep window timestamps derived from Event msg 21 (event=74) or fallback to first/last SleepLevel
func processSleepFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	// Decode FIT first to find the first timestamp for filename/ts derivation.
	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return fmt.Errorf("processSleepFIT: decode: %w", err)
	}

	// --- Pass 1: determine first timestamp for storeHealthFIT ---
	var firstTS time.Time
	for i := range fit.Messages {
		msg := &fit.Messages[i]
		if msg.Num == typedef.MesgNumSleepLevel {
			sl := mesgdef.NewSleepLevel(msg)
			if !sl.Timestamp.IsZero() && sl.Timestamp.After(minValidTS) {
				firstTS = sl.Timestamp
				break
			}
		}
	}
	if firstTS.IsZero() {
		firstTS = time.Now().UTC()
	}

	filename := fmt.Sprintf("sleep_%s.fit", firstTS.UTC().Format("20060102T150405Z"))
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "sleep", filename, firstTS); err != nil {
		slog.Warn("processSleepFIT: storeHealthFIT failed", "error", err)
		// non-fatal, continue
	}

	// --- Pass 2: collect data ---
	var sleepLevels []model.MonitoringSample
	var assessment *mesgdef.SleepAssessment
	var sleepStartUTC, sleepEndUTC time.Time

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumSleepLevel: // 275
			sl := mesgdef.NewSleepLevel(msg)
			if sl.SleepLevel == typedef.SleepLevelInvalid {
				continue
			}
			if sl.Timestamp.IsZero() || !sl.Timestamp.After(minValidTS) {
				continue
			}
			level := int16(sl.SleepLevel)
			sleepLevels = append(sleepLevels, model.MonitoringSample{
				UserID:     userID,
				TS:         sl.Timestamp,
				SleepLevel: &level,
			})

		case typedef.MesgNumSleepAssessment: // 346
			assessment = mesgdef.NewSleepAssessment(msg)

		case 21: // Event
			// Proprietary sleep start/stop event: event field 0 = 74 (SleepEvent)
			// event_type field 1: 0 = start, 1 = stop
			// Timestamp is field 253.
			rawEvent := msg.FieldValueByNum(0).Uint8()
			if rawEvent != 74 {
				continue
			}
			rawTS := msg.FieldValueByNum(253).Uint32()
			if rawTS == basetype.Uint32Invalid {
				continue
			}
			ts := datetime.ToTime(rawTS)
			if ts.IsZero() || !ts.After(minValidTS) {
				continue
			}
			eventType := msg.FieldValueByNum(1).Uint8()
			switch eventType {
			case 0: // start
				sleepStartUTC = ts
			case 1: // stop
				sleepEndUTC = ts
			}
		}
	}

	// --- Fallback: use first/last SleepLevel timestamps if no Event msg with event=74 ---
	if sleepStartUTC.IsZero() && len(sleepLevels) > 0 {
		sleepStartUTC = sleepLevels[0].TS
	}
	if sleepEndUTC.IsZero() && len(sleepLevels) > 0 {
		sleepEndUTC = sleepLevels[len(sleepLevels)-1].TS
	}

	// --- Persist sleep levels ---
	if len(sleepLevels) > 0 {
		if err := db.InsertMonitoringSamples(ctx, sleepLevels); err != nil {
			slog.Warn("processSleepFIT: InsertMonitoringSamples failed", "error", err)
		}
	}

	// --- Build and persist HealthSleep if assessment present ---
	if assessment == nil {
		return nil
	}

	tzName := loadUserTimezone(ctx, db, userID)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		slog.Warn("processSleepFIT: unknown timezone, falling back to UTC", "tz", tzName, "error", err)
		loc = time.UTC
	}

	// calendar_date = sleep start in local time
	calDate := sleepStartUTC.In(loc).Format("2006-01-02")
	if sleepStartUTC.IsZero() {
		calDate = firstTS.In(loc).Format("2006-01-02")
	}

	sleep := &model.HealthSleep{
		UserID:       userID,
		CalendarDate: calDate,
		SourceType:   "garmin_fit",
	}

	// Set timestamps only if non-zero (COALESCE preserves existing values on conflict)
	if !sleepStartUTC.IsZero() {
		sleep.SleepStartUTC = &sleepStartUTC
	}
	if !sleepEndUTC.IsZero() {
		sleep.SleepEndUTC = &sleepEndUTC
	}

	// Map SleepAssessment fields — all uint8, 255 = invalid → nil
	sleep.ScoreOverall = uint8PtrToIntPtr(assessment.OverallSleepScore)
	sleep.ScoreQuality = uint8PtrToIntPtr(assessment.SleepQualityScore)
	sleep.ScoreDuration = uint8PtrToIntPtr(assessment.SleepDurationScore)
	sleep.ScoreDeep = uint8PtrToIntPtr(assessment.DeepSleepScore)
	sleep.ScoreRem = uint8PtrToIntPtr(assessment.RemSleepScore)
	sleep.AwakeCount = uint8PtrToIntPtr(assessment.AwakeningsCount)

	sleep.ScoreLight = uint8PtrToInt16Ptr(assessment.LightSleepScore)
	sleep.ScoreAwakeTime = uint8PtrToInt16Ptr(assessment.AwakeTimeScore)
	sleep.ScoreAwakenings = uint8PtrToInt16Ptr(assessment.AwakeningsCountScore)
	sleep.ScoreCombinedAwake = uint8PtrToInt16Ptr(assessment.CombinedAwakeScore)
	sleep.ScoreRestlessness = uint8PtrToInt16Ptr(assessment.SleepRestlessnessScore)
	sleep.ScoreInterruptions = uint8PtrToInt16Ptr(assessment.InterruptionsScore)
	sleep.ScoreRecovery = uint8PtrToInt16Ptr(assessment.SleepRecoveryScore)

	// AverageStressDuringSleep: uint16, scale /100 → float64
	if assessment.AverageStressDuringSleep != basetype.Uint16Invalid {
		v := float64(assessment.AverageStressDuringSleep) / 100.0
		sleep.AvgSleepStress = &v
	}

	if err := db.UpsertHealthSleep(ctx, sleep); err != nil {
		return fmt.Errorf("processSleepFIT: UpsertHealthSleep: %w", err)
	}

	return nil
}

// uint8PtrToIntPtr converts a uint8 value to *int, returning nil if invalid (255).
func uint8PtrToIntPtr(v uint8) *int {
	if v == basetype.Uint8Invalid {
		return nil
	}
	i := int(v)
	return &i
}

// uint8PtrToInt16Ptr converts a uint8 value to *int16, returning nil if invalid (255).
func uint8PtrToInt16Ptr(v uint8) *int16 {
	if v == basetype.Uint8Invalid {
		return nil
	}
	i := int16(v)
	return &i
}

