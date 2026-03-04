package garmin

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// StepsImporter reads daily wellness / step data from the Garmin UDS (User Daily Summary) files.
//
// Real Garmin export structure:
//
//	DI_CONNECT/DI-Connect-Aggregator/UDSFile_<from>_<to>.json
//	  Each file is a JSON array of UDS records with fields like:
//	    calendarDate, totalSteps, dailyStepGoal, restingHeartRate,
//	    totalKilocalories, activeKilocalories, bodyBattery, allDayStress, …
//
// Sleep data:
//
//	DI_CONNECT/DI-Connect-Wellness/*_sleepData.json
//	  Each file is a JSON array of sleep records with:
//	    calendarDate, deepSleepSeconds, lightSleepSeconds, remSleepSeconds,
//	    awakeSleepSeconds, sleepStartTimestampGMT, sleepEndTimestampGMT, …
type StepsImporter struct{}

func (s *StepsImporter) Name() string        { return "health" }
func (s *StepsImporter) Description() string { return "Gesundheitsdaten (Schritte, Schlaf, Ruhepuls)" }
func (s *StepsImporter) Priority() int       { return 40 }

// garminUDSRecord matches UDSFile_*.json items.
type garminUDSRecord struct {
	CalendarDate         string      `json:"calendarDate"`
	TotalSteps           *int        `json:"totalSteps"`
	DailyStepGoal        *int        `json:"dailyStepGoal"`
	RestingHeartRate     *int        `json:"restingHeartRate"`
	TotalKilocalories    *float64    `json:"totalKilocalories"`
	ActiveKilocalories   *float64    `json:"activeKilocalories"`
	BodyBattery          interface{} `json:"bodyBattery"` // can be array or null
	AllDayStress         interface{} `json:"allDayStress"` // can be object or null
}

// garminSleepRecord matches *_sleepData.json items.
type garminSleepRecord struct {
	CalendarDate         string `json:"calendarDate"`
	DeepSleepSeconds     *int   `json:"deepSleepSeconds"`
	LightSleepSeconds    *int   `json:"lightSleepSeconds"`
	RemSleepSeconds      *int   `json:"remSleepSeconds"`
	AwakeSleepSeconds    *int   `json:"awakeSleepSeconds"`
	SleepStartTimestampGMT *int64 `json:"sleepStartTimestampGMT"`
	SleepEndTimestampGMT   *int64 `json:"sleepEndTimestampGMT"`
}

func (s *StepsImporter) Detect(src ExportSource) (bool, int, error) {
	files, _ := src.FindFiles("DI_CONNECT/DI-Connect-Aggregator/UDSFile_*.json")
	if len(files) > 0 {
		return true, len(files) * 100, nil // rough estimate
	}
	return false, 0, nil
}

func (s *StepsImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	// Import daily summaries (UDS files)
	udsFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Aggregator/UDSFile_*.json")
	totalUDS := 0
	for _, f := range udsFiles {
		var records []garminUDSRecord
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read UDS file", "file", f, "error", err)
			continue
		}
		for _, rec := range records {
			if rec.CalendarDate == "" {
				continue
			}
			summary := database.GarminDailySummary{
				CalendarDate:       rec.CalendarDate,
				TotalSteps:         rec.TotalSteps,
				StepGoal:           rec.DailyStepGoal,
				RestingHeartRate:   rec.RestingHeartRate,
				TotalCalories:      rec.TotalKilocalories,
				ActiveCalories:     rec.ActiveKilocalories,
			}
			if err := params.DB.UpsertGarminDailySummary(ctx, params.UserID, summary); err != nil {
				slog.Warn("garmin health: upsert daily summary failed", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "upsert "+rec.CalendarDate+": "+err.Error())
				continue
			}
			totalUDS++
		}
	}
	result.Imported += totalUDS
	slog.Info("garmin health: imported daily summaries", "count", totalUDS)

	// Import sleep data
	sleepFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/*_sleepData.json")
	totalSleep := 0
	for _, f := range sleepFiles {
		var records []garminSleepRecord
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read sleep file", "file", f, "error", err)
			continue
		}
		for _, rec := range records {
			if rec.CalendarDate == "" {
				continue
			}
			sleep := database.GarminSleepSummary{
				CalendarDate:      rec.CalendarDate,
				DeepSleepSeconds:  rec.DeepSleepSeconds,
				LightSleepSeconds: rec.LightSleepSeconds,
				RemSleepSeconds:   rec.RemSleepSeconds,
				AwakeSleepSeconds: rec.AwakeSleepSeconds,
			}
			if err := params.DB.UpsertGarminSleepSummary(ctx, params.UserID, sleep); err != nil {
				slog.Warn("garmin health: upsert sleep failed", "date", rec.CalendarDate, "error", err)
				continue
			}
			totalSleep++
		}
	}
	result.Imported += totalSleep
	slog.Info("garmin health: imported sleep records", "count", totalSleep)

	return result, nil
}
