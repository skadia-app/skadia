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

// parsedSleepData holds the pure decode output of a sleep FIT file.
type parsedSleepData struct {
	SleepLevels   []model.MonitoringSample
	Assessment    *mesgdef.SleepAssessment // nil when absent
	SleepStartUTC time.Time
	SleepEndUTC   time.Time
	FirstTS       time.Time // earliest valid SleepLevel timestamp, used for ShadowFS naming
}

// decodeSleep decodes a raw sleep FIT payload into typed data without any I/O.
func decodeSleep(raw []byte, userID string) (parsedSleepData, error) {
	dec := decoder.New(bytes.NewReader(raw), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return parsedSleepData{}, fmt.Errorf("decodeSleep: %w", err)
	}

	// Pass 1: find first valid SleepLevel timestamp for ShadowFS filename derivation.
	var firstTS time.Time
	for i := range fit.Messages {
		msg := &fit.Messages[i]
		if msg.Num == typedef.MesgNumSleepLevel {
			sl := mesgdef.NewSleepLevel(msg)
			if !sl.Timestamp.IsZero() && sl.Timestamp.After(minSaneTimestamp) {
				firstTS = sl.Timestamp
				break
			}
		}
	}
	if firstTS.IsZero() {
		firstTS = time.Now().UTC()
	}

	// Pass 2: collect sleep levels, assessment, and window timestamps.
	var sleepLevels []model.MonitoringSample
	var assessment *mesgdef.SleepAssessment
	var sleepStartUTC, sleepEndUTC time.Time

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumSleepLevel: // 275
			sl := mesgdef.NewSleepLevel(msg)
			if sl.SleepLevel == typedef.SleepLevelInvalid || sl.SleepLevel > 4 {
				continue
			}
			if sl.Timestamp.IsZero() || !sl.Timestamp.After(minSaneTimestamp) {
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
			if ts.IsZero() || !ts.After(minSaneTimestamp) {
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

	// Fallback: use first/last SleepLevel timestamps when no Event msg with event=74 is present.
	if sleepStartUTC.IsZero() && len(sleepLevels) > 0 {
		sleepStartUTC = sleepLevels[0].TS
	}
	if sleepEndUTC.IsZero() && len(sleepLevels) > 0 {
		sleepEndUTC = sleepLevels[len(sleepLevels)-1].TS
	}

	return parsedSleepData{
		SleepLevels:   sleepLevels,
		Assessment:    assessment,
		SleepStartUTC: sleepStartUTC,
		SleepEndUTC:   sleepEndUTC,
		FirstTS:       firstTS,
	}, nil
}

// processSleepFIT decodes a Garmin sleep FIT file (type 49) and persists:
//   - Sleep levels (msg 275) as MonitoringSamples
//   - Sleep assessment scores (msg 346) as HealthSleep
//   - Sleep window timestamps derived from Event msg 21 (event=74) or fallback to first/last SleepLevel
func processSleepFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	parsed, err := decodeSleep(data, userID)
	if err != nil {
		return fmt.Errorf("processSleepFIT: %w", err)
	}

	filename := fmt.Sprintf("sleep_%s.fit", parsed.FirstTS.UTC().Format("20060102T150405Z"))
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "sleep", filename, parsed.FirstTS); err != nil {
		slog.Warn("processSleepFIT: storeHealthFIT failed", "error", err)
		// non-fatal, continue
	}

	if len(parsed.SleepLevels) > 0 {
		if err := db.InsertMonitoringSamples(ctx, parsed.SleepLevels); err != nil {
			slog.Warn("processSleepFIT: InsertMonitoringSamples failed", "error", err)
		}
	}

	if parsed.Assessment == nil {
		return nil
	}

	tzName := loadUserTimezone(ctx, db, userID)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		slog.Warn("processSleepFIT: unknown timezone, falling back to UTC", "tz", tzName, "error", err)
		loc = time.UTC
	}

	// calendar_date = sleep start in local time
	calDate := parsed.SleepStartUTC.In(loc).Format("2006-01-02")
	if parsed.SleepStartUTC.IsZero() {
		calDate = parsed.FirstTS.In(loc).Format("2006-01-02")
	}

	sleep := &model.HealthSleep{
		UserID:       userID,
		CalendarDate: calDate,
		SourceType:   "garmin_fit",
	}

	// Set timestamps only if non-zero (COALESCE preserves existing values on conflict).
	if !parsed.SleepStartUTC.IsZero() {
		sleep.SleepStartUTC = &parsed.SleepStartUTC
	}
	if !parsed.SleepEndUTC.IsZero() {
		sleep.SleepEndUTC = &parsed.SleepEndUTC
	}

	// Map SleepAssessment fields — all uint8, 255 = invalid → nil.
	sleep.ScoreOverall = uint8PtrToIntPtr(parsed.Assessment.OverallSleepScore)
	sleep.ScoreQuality = uint8PtrToIntPtr(parsed.Assessment.SleepQualityScore)
	sleep.ScoreDuration = uint8PtrToIntPtr(parsed.Assessment.SleepDurationScore)
	sleep.ScoreDeep = uint8PtrToIntPtr(parsed.Assessment.DeepSleepScore)
	sleep.ScoreRem = uint8PtrToIntPtr(parsed.Assessment.RemSleepScore)
	sleep.AwakeCount = uint8PtrToIntPtr(parsed.Assessment.AwakeningsCount)

	sleep.ScoreLight = uint8PtrToInt16Ptr(parsed.Assessment.LightSleepScore)
	sleep.ScoreAwakeTime = uint8PtrToInt16Ptr(parsed.Assessment.AwakeTimeScore)
	sleep.ScoreAwakenings = uint8PtrToInt16Ptr(parsed.Assessment.AwakeningsCountScore)
	sleep.ScoreCombinedAwake = uint8PtrToInt16Ptr(parsed.Assessment.CombinedAwakeScore)
	sleep.ScoreRestlessness = uint8PtrToInt16Ptr(parsed.Assessment.SleepRestlessnessScore)
	sleep.ScoreInterruptions = uint8PtrToInt16Ptr(parsed.Assessment.InterruptionsScore)
	sleep.ScoreRecovery = uint8PtrToInt16Ptr(parsed.Assessment.SleepRecoveryScore)

	// AverageStressDuringSleep: uint16, scale /100 → float64.
	if parsed.Assessment.AverageStressDuringSleep != basetype.Uint16Invalid {
		v := float64(parsed.Assessment.AverageStressDuringSleep) / 100.0
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

