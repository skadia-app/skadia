package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// StepsImporter reads daily wellness / step data from the Garmin UDS (User Daily Summary) files.
//
// Real Garmin export structure:
//
//	DI_CONNECT/DI-Connect-Aggregator/UDSFile_<from>_<to>.json
//	  Each file is a JSON array of UDS records.
//
// Also imports: sleep, HRV snapshots, weight, blood pressure, training readiness, training load.
type StepsImporter struct{}

func (s *StepsImporter) Name() string        { return "health" }
func (s *StepsImporter) Description() string { return "Gesundheitsdaten (Schritte, Schlaf, Ruhepuls)" }
func (s *StepsImporter) Priority() int       { return 40 }

func (s *StepsImporter) Detect(src ExportSource) (bool, int, error) {
	files, _ := src.FindFiles("DI_CONNECT/DI-Connect-Aggregator/UDSFile_*.json")
	if len(files) > 0 {
		return true, len(files) * 100, nil
	}
	return false, 0, nil
}

// parseGarminTime parses Garmin's timestamp strings (with optional fractional seconds).
func parseGarminTime(s string) (time.Time, error) {
	for _, f := range []string{"2006-01-02T15:04:05.0", "2006-01-02T15:04:05", "2006-01-02T15:04:05.999"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q", s)
}

// parseBodyBattery extracts min/max values from Garmin's bodyBattery JSON field.
func parseBodyBattery(raw json.RawMessage) (min, max *int) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var arr []struct {
		BodyBatteryValuesArray [][]int `json:"bodyBatteryValuesArray"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, nil
	}
	var minVal, maxVal *int
	for _, entry := range arr {
		for _, pair := range entry.BodyBatteryValuesArray {
			if len(pair) < 2 {
				continue
			}
			v := pair[1]
			if minVal == nil || v < *minVal {
				cp := v
				minVal = &cp
			}
			if maxVal == nil || v > *maxVal {
				cp := v
				maxVal = &cp
			}
		}
	}
	return minVal, maxVal
}

// --- JSON record types ---

// garminUDSRecord matches UDSFile_*.json items.
type garminUDSRecord struct {
	CalendarDate        string          `json:"calendarDate"`
	TotalSteps          *int            `json:"totalSteps"`
	DailyStepGoal       *int            `json:"dailyStepGoal"`
	RestingHeartRate    *int            `json:"restingHeartRate"`
	MinHeartRate        *int            `json:"minHeartRate"`
	MaxHeartRate        *int            `json:"maxHeartRate"`
	TotalKilocalories   *float64        `json:"totalKilocalories"`
	ActiveKilocalories  *float64        `json:"activeKilocalories"`
	BmrKilocalories     *float64        `json:"bmrKilocalories"`
	TotalDistanceMeters *float64        `json:"totalDistanceMeters"`
	HighlyActiveSeconds *int            `json:"highlyActiveSeconds"`
	ActiveSeconds       *int            `json:"activeSeconds"`
	AllDayStress        json.RawMessage `json:"allDayStress"`
	BodyBattery         json.RawMessage `json:"bodyBattery"`
}

// garminSleepRecord matches *_sleepData.json items.
type garminSleepRecord struct {
	CalendarDate           string          `json:"calendarDate"`
	SleepStartTimestampGMT string          `json:"sleepStartTimestampGMT"`
	SleepEndTimestampGMT   string          `json:"sleepEndTimestampGMT"`
	DeepSleepSeconds       *int            `json:"deepSleepSeconds"`
	LightSleepSeconds      *int            `json:"lightSleepSeconds"`
	RemSleepSeconds        *int            `json:"remSleepSeconds"`
	AwakeSleepSeconds      *int            `json:"awakeSleepSeconds"`
	AverageRespiration     *float64        `json:"averageRespiration"`
	AwakeCount             *int            `json:"awakeCount"`
	AvgSleepStress         *float64        `json:"avgSleepStress"`
	RestlessMomentCount    *int            `json:"restlessMomentCount"`
	SleepScores            json.RawMessage `json:"sleepScores"`
}

// garminWellnessActivity matches *_wellnessActivities.json items.
type garminWellnessActivity struct {
	WellnessActivityType    string `json:"wellnessActivityType"`
	StartTimestampGMT       string `json:"startTimestampGMT"`
	SnapshotTimeOfDayType   string `json:"snapshotTimeOfDayType"`
	SummaryTypeDataList     []struct {
		SummaryType string  `json:"summaryType"`
		AvgValue    float64 `json:"avgValue"`
	} `json:"summaryTypeDataList"`
}

// garminBioMetrics matches *_userBioMetrics.json items.
type garminBioMetrics struct {
	Weight *struct {
		Weight       float64 `json:"weight"`
		BMI          float64 `json:"bmi"`
		BodyFat      float64 `json:"bodyFat"`
		TimestampGMT string  `json:"timestampGMT"`
	} `json:"weight"`
}

// garminBloodPressure matches BloodPressureFile_*.json items.
type garminBloodPressure struct {
	BloodPressure *struct {
		Systolic  int `json:"systolic"`
		Diastolic int `json:"diastolic"`
	} `json:"bloodPressure"`
	MetaData *struct {
		CalendarDate []int `json:"calendarDate"` // [year, month, day, hour, min, sec]
	} `json:"metaData"`
}

// garminTrainingReadiness matches TrainingReadinessDTO_*.json items.
type garminTrainingReadiness struct {
	CalendarDate                string   `json:"calendarDate"`
	Score                       *int     `json:"score"`
	Level                       *string  `json:"level"`
	SleepScore                  *int     `json:"sleepScore"`
	RecoveryTime                *int     `json:"recoveryTime"`
	AcwrFactorPercent           *float64 `json:"acwrFactorPercent"`
	StressHistoryFactorPercent  *float64 `json:"stressHistoryFactorPercent"`
	HRVFactorPercent            *float64 `json:"hrvFactorPercent"`
	SleepHistoryFactorPercent   *float64 `json:"sleepHistoryFactorPercent"`
	RecoveryTimeFactorPercent   *float64 `json:"recoveryTimeFactorPercent"`
	HRVWeeklyAverage            *float64 `json:"hrvWeeklyAverage"`
}

// garminTrainingLoad matches MetricsAcuteTrainingLoad_*.json items.
type garminTrainingLoad struct {
	CalendarDate              int64    `json:"calendarDate"` // Unix milliseconds
	AcwrPercent               *float64 `json:"acwrPercent"`
	AcwrStatus                *string  `json:"acwrStatus"`
	DailyTrainingLoadAcute    *float64 `json:"dailyTrainingLoadAcute"`
	DailyTrainingLoadChronic  *float64 `json:"dailyTrainingLoadChronic"`
}

func (s *StepsImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	// -----------------------------------------------------------------------
	// Abschnitt 1: UDS (User Daily Summary)
	// -----------------------------------------------------------------------
	udsFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Aggregator/UDSFile_*.json")
	totalUDS := 0
	for _, f := range udsFiles {
		var records []garminUDSRecord
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read UDS file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.CalendarDate == "" {
				continue
			}

			// Parse stress average from allDayStress.aggregatorList
			var stressAvg *float64
			if len(rec.AllDayStress) > 0 && string(rec.AllDayStress) != "null" {
				var allDayStress struct {
					AggregatorList []struct {
						Type              string  `json:"type"`
						AverageStressLevel float64 `json:"averageStressLevel"`
					} `json:"aggregatorList"`
				}
				if err := json.Unmarshal(rec.AllDayStress, &allDayStress); err == nil {
					for _, agg := range allDayStress.AggregatorList {
						if agg.Type == "TOTAL" {
							v := agg.AverageStressLevel
							stressAvg = &v
							break
						}
					}
				}
			}

			bbMin, bbMax := parseBodyBattery(rec.BodyBattery)

			summary := &model.HealthDailySummary{
				UserID:              params.UserID,
				CalendarDate:        rec.CalendarDate,
				TotalSteps:          rec.TotalSteps,
				StepGoal:            rec.DailyStepGoal,
				RestingHeartRate:    rec.RestingHeartRate,
				MinHeartRate:        rec.MinHeartRate,
				MaxHeartRate:        rec.MaxHeartRate,
				TotalCalories:       rec.TotalKilocalories,
				ActiveCalories:      rec.ActiveKilocalories,
				BmrCalories:         rec.BmrKilocalories,
				TotalDistanceM:      rec.TotalDistanceMeters,
				HighlyActiveSeconds: rec.HighlyActiveSeconds,
				ActiveSeconds:       rec.ActiveSeconds,
				StressAvg:           stressAvg,
				BodyBatteryMin:      bbMin,
				BodyBatteryMax:      bbMax,
				SourceType:          "garmin",
			}

			if err := params.DB.UpsertHealthDailySummary(ctx, summary); err != nil {
				slog.Warn("garmin health: upsert daily summary failed", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "upsert UDS "+rec.CalendarDate+": "+err.Error())
				continue
			}
			totalUDS++
		}
	}
	slog.Info("garmin health: imported daily summaries", "count", totalUDS)
	result.Imported += totalUDS

	// -----------------------------------------------------------------------
	// Abschnitt 2: Sleep
	// -----------------------------------------------------------------------
	sleepFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/*_sleepData.json")
	totalSleep := 0
	for _, f := range sleepFiles {
		var records []garminSleepRecord
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read sleep file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.SleepStartTimestampGMT == "" {
				continue
			}

			startUTC, err := parseGarminTime(rec.SleepStartTimestampGMT)
			if err != nil {
				slog.Warn("garmin health: cannot parse sleep start", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "parse sleep start "+rec.CalendarDate+": "+err.Error())
				continue
			}
			endUTC, err := parseGarminTime(rec.SleepEndTimestampGMT)
			if err != nil {
				slog.Warn("garmin health: cannot parse sleep end", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "parse sleep end "+rec.CalendarDate+": "+err.Error())
				continue
			}

			// Parse sleep scores
			var scoreOverall, scoreQuality, scoreDuration, scoreDeep, scoreRem *int
			if len(rec.SleepScores) > 0 && string(rec.SleepScores) != "null" {
				var scores struct {
					OverallScore   *int `json:"overallScore"`
					QualityScore   *int `json:"qualityScore"`
					DurationScore  *int `json:"durationScore"`
					DeepScore      *int `json:"deepScore"`
					RemScore       *int `json:"remScore"`
				}
				if err := json.Unmarshal(rec.SleepScores, &scores); err == nil {
					scoreOverall = scores.OverallScore
					scoreQuality = scores.QualityScore
					scoreDuration = scores.DurationScore
					scoreDeep = scores.DeepScore
					scoreRem = scores.RemScore
				}
			}

			sleep := &model.HealthSleep{
				UserID:          params.UserID,
				CalendarDate:    rec.CalendarDate,
				SleepStartUTC:   startUTC,
				SleepEndUTC:     endUTC,
				DeepSeconds:     rec.DeepSleepSeconds,
				LightSeconds:    rec.LightSleepSeconds,
				RemSeconds:      rec.RemSleepSeconds,
				AwakeSeconds:    rec.AwakeSleepSeconds,
				RespirationAvg:  rec.AverageRespiration,
				AwakeCount:      rec.AwakeCount,
				AvgSleepStress:  rec.AvgSleepStress,
				RestlessMoments: rec.RestlessMomentCount,
				ScoreOverall:    scoreOverall,
				ScoreQuality:    scoreQuality,
				ScoreDuration:   scoreDuration,
				ScoreDeep:       scoreDeep,
				ScoreRem:        scoreRem,
				SourceType:      "garmin",
			}

			if err := params.DB.UpsertHealthSleep(ctx, sleep); err != nil {
				slog.Warn("garmin health: upsert sleep failed", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "upsert sleep "+rec.CalendarDate+": "+err.Error())
				continue
			}
			totalSleep++
		}
	}
	slog.Info("garmin health: imported sleep records", "count", totalSleep)
	result.Imported += totalSleep

	// -----------------------------------------------------------------------
	// Abschnitt 3: HRV-Snapshots
	// -----------------------------------------------------------------------
	hrvFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/*_wellnessActivities.json")
	totalHRV := 0
	for _, f := range hrvFiles {
		var records []garminWellnessActivity
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read wellness activities file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.WellnessActivityType != "HEALTH_MONITORING" {
				continue
			}

			measuredAt, err := parseGarminTime(rec.StartTimestampGMT)
			if err != nil {
				slog.Warn("garmin health: cannot parse HRV timestamp", "ts", rec.StartTimestampGMT, "error", err)
				result.Errors = append(result.Errors, "parse HRV ts "+rec.StartTimestampGMT+": "+err.Error())
				continue
			}

			var rmssd, sdrr *float64
			for _, entry := range rec.SummaryTypeDataList {
				v := entry.AvgValue
				switch entry.SummaryType {
				case "RMSSD_HRV":
					rmssd = &v
				case "SDRR_HRV":
					sdrr = &v
				}
			}

			if rmssd == nil {
				continue // no RMSSD → skip
			}

			var timeOfDay *string
			if rec.SnapshotTimeOfDayType != "" {
				tod := strings.ToLower(rec.SnapshotTimeOfDayType)
				timeOfDay = &tod
			}

			snap := &model.HealthHRVSnapshot{
				UserID:        params.UserID,
				MeasuredAtUTC: measuredAt,
				RmssdMS:       rmssd,
				SdrrMS:        sdrr,
				TimeOfDay:     timeOfDay,
				SourceType:    "garmin",
			}

			if err := params.DB.UpsertHealthHRVSnapshot(ctx, snap); err != nil {
				slog.Warn("garmin health: upsert HRV snapshot failed", "ts", rec.StartTimestampGMT, "error", err)
				result.Errors = append(result.Errors, "upsert HRV "+rec.StartTimestampGMT+": "+err.Error())
				continue
			}
			totalHRV++
		}
	}
	slog.Info("garmin health: imported HRV snapshots", "count", totalHRV)
	result.Imported += totalHRV

	// -----------------------------------------------------------------------
	// Abschnitt 4: Gewicht
	// -----------------------------------------------------------------------
	weightFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/*_userBioMetrics.json")
	totalWeight := 0
	for _, f := range weightFiles {
		var records []garminBioMetrics
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read bioMetrics file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.Weight == nil || rec.Weight.Weight <= 0 {
				continue
			}

			measuredAt, err := parseGarminTime(rec.Weight.TimestampGMT)
			if err != nil {
				slog.Warn("garmin health: cannot parse weight timestamp", "ts", rec.Weight.TimestampGMT, "error", err)
				result.Errors = append(result.Errors, "parse weight ts "+rec.Weight.TimestampGMT+": "+err.Error())
				continue
			}

			entry := &model.HealthWeightEntry{
				UserID:        params.UserID,
				MeasuredAtUTC: measuredAt,
				WeightG:       int(rec.Weight.Weight), // Garmin liefert bereits Gramm
				SourceType:    "garmin",
			}
			if rec.Weight.BMI > 0 {
				v := rec.Weight.BMI
				entry.BMI = &v
			}
			if rec.Weight.BodyFat > 0 {
				v := rec.Weight.BodyFat
				entry.BodyFatPct = &v
			}

			if err := params.DB.UpsertHealthWeightEntry(ctx, entry); err != nil {
				slog.Warn("garmin health: upsert weight entry failed", "ts", rec.Weight.TimestampGMT, "error", err)
				result.Errors = append(result.Errors, "upsert weight "+rec.Weight.TimestampGMT+": "+err.Error())
				continue
			}
			totalWeight++
		}
	}
	slog.Info("garmin health: imported weight entries", "count", totalWeight)
	result.Imported += totalWeight

	// -----------------------------------------------------------------------
	// Abschnitt 5: Blutdruck
	// -----------------------------------------------------------------------
	bpFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/BloodPressureFile_*.json")
	totalBP := 0
	for _, f := range bpFiles {
		var records []garminBloodPressure
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read blood pressure file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.BloodPressure == nil || rec.MetaData == nil {
				continue
			}
			cd := rec.MetaData.CalendarDate
			if len(cd) < 6 {
				continue
			}

			measuredAt := time.Date(cd[0], time.Month(cd[1]), cd[2], cd[3], cd[4], cd[5], 0, time.UTC)

			sys := rec.BloodPressure.Systolic
			dia := rec.BloodPressure.Diastolic
			bp := &model.HealthBloodPressure{
				UserID:        params.UserID,
				MeasuredAtUTC: measuredAt,
				Systolic:      &sys,
				Diastolic:     &dia,
				SourceType:    "garmin",
			}

			if err := params.DB.UpsertHealthBloodPressure(ctx, bp); err != nil {
				slog.Warn("garmin health: upsert blood pressure failed", "ts", measuredAt, "error", err)
				result.Errors = append(result.Errors, "upsert bp "+measuredAt.String()+": "+err.Error())
				continue
			}
			totalBP++
		}
	}
	slog.Info("garmin health: imported blood pressure records", "count", totalBP)
	result.Imported += totalBP

	// -----------------------------------------------------------------------
	// Abschnitt 6: Training Readiness
	// -----------------------------------------------------------------------
	readinessFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Metrics/TrainingReadinessDTO_*.json")
	totalReadiness := 0
	for _, f := range readinessFiles {
		var records []garminTrainingReadiness
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read training readiness file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.CalendarDate == "" {
				continue
			}

			r := &model.HealthReadinessScore{
				UserID:                 params.UserID,
				CalendarDate:           rec.CalendarDate,
				Score:                  rec.Score,
				Level:                  rec.Level,
				SleepScore:             rec.SleepScore,
				RecoveryTimeMin:        rec.RecoveryTime,
				AcwrPct:                rec.AcwrFactorPercent,
				HRVFactorPct:           rec.HRVFactorPercent,
				SleepHistoryFactorPct:  rec.SleepHistoryFactorPercent,
				StressHistoryFactorPct: rec.StressHistoryFactorPercent,
				RecoveryTimeFactorPct:  rec.RecoveryTimeFactorPercent,
				HRVWeeklyAvgMS:         rec.HRVWeeklyAverage,
				SourceType:             "garmin",
			}

			if err := params.DB.UpsertHealthReadinessScore(ctx, r); err != nil {
				slog.Warn("garmin health: upsert readiness score failed", "date", rec.CalendarDate, "error", err)
				result.Errors = append(result.Errors, "upsert readiness "+rec.CalendarDate+": "+err.Error())
				continue
			}
			totalReadiness++
		}
	}
	slog.Info("garmin health: imported training readiness scores", "count", totalReadiness)
	result.Imported += totalReadiness

	// -----------------------------------------------------------------------
	// Abschnitt 7: Training Load (ATL/CTL)
	// -----------------------------------------------------------------------
	loadFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Metrics/MetricsAcuteTrainingLoad_*.json")
	totalLoad := 0
	for _, f := range loadFiles {
		var records []garminTrainingLoad
		if err := params.Source.ReadJSON(f, &records); err != nil {
			slog.Warn("garmin health: cannot read training load file", "file", f, "error", err)
			result.Errors = append(result.Errors, "read "+f+": "+err.Error())
			continue
		}
		for _, rec := range records {
			if rec.CalendarDate == 0 {
				continue
			}

			calDate := time.UnixMilli(rec.CalendarDate).UTC().Format("2006-01-02")

			load := &model.HealthTrainingLoad{
				UserID:       params.UserID,
				CalendarDate: calDate,
				ATLAcute:     rec.DailyTrainingLoadAcute,
				CTLChronic:   rec.DailyTrainingLoadChronic,
				AcwrPct:      rec.AcwrPercent,
				AcwrStatus:   rec.AcwrStatus,
				SourceType:   "garmin",
			}

			if err := params.DB.UpsertHealthTrainingLoad(ctx, load); err != nil {
				slog.Warn("garmin health: upsert training load failed", "date", calDate, "error", err)
				result.Errors = append(result.Errors, "upsert training load "+calDate+": "+err.Error())
				continue
			}
			totalLoad++
		}
	}
	slog.Info("garmin health: imported training load records", "count", totalLoad)
	result.Imported += totalLoad

	return result, nil
}
