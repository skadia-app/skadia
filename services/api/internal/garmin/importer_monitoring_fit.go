package garmin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
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

// Garmin monitoring FIT file types found in UploadedFiles ZIPs.
const (
	fitFileTypeActivity        = 4  // Standard activity (Laufen, Radfahren etc.)
	fitFileTypeSport           = 3  // Sport profile
	fitFileTypeWorkout         = 5  // Workout definition
	fitFileTypeCourse          = 6  // Course/route
	fitFileTypeWeight          = 9  // Weight scale data
	fitFileTypeMonitoringA     = 15 // Monitoring A (hourly device monitoring)
	fitFileTypeMonitoringDaily = 28 // Monitoring daily aggregates
	fitFileTypeMonitoringB     = 32 // Monitoring B
	fitFileTypeSegment         = 34 // Segment definition
	fitFileTypeHRVNight        = 44 // Nächtliche HRV-5-Minuten-Werte (msg 281, proprietary)
	fitFileTypeSleep           = 49 // Sleep data
	fitFileTypeSpO2            = 58 // Blood oxygen (SpO2)
	fitFileTypeMonitorMain     = 68 // Haupt-Monitoring: HRV-Status, Schlaf, Stress, Ruhepuls
)

// monitoringFITResult holds all health data extracted from a single monitoring FIT file.
type monitoringFITResult struct {
	// From hrv_status_summary (msg 370): daily HRV status and baseline.
	HRVSnapshots []model.HealthHRVSnapshot

	// From hrv_value (msg 371, SDK standard) or proprietary msg 281: 5-min night RMSSD.
	HRVNightValues []model.HealthHRVNightValue

	// From MonitoringHrData (msg 211): daily resting heart rate.
	RestingHRByDate map[string]int // calendarDate -> resting_hr_bpm (7-day rolling avg)

	// From proprietary msg 369 (Training Readiness): daily readiness score and factors.
	TrainingReadiness []*model.HealthReadinessScore
}

// importMonitoringFITs reads all UploadedFiles ZIPs and extracts health data from
// monitoring FIT files (types 68, 44). Skips activity FIT files (type 4) and
// all other unknown types silently.
// Returns import counts and any non-fatal errors.
func importMonitoringFITs(ctx context.Context, params ImportParams) (imported int, errs []string) {
	zips, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if len(zips) == 0 {
		return 0, nil
	}

	fitErrors := 0

	_, _ = params.DB.Pool.Exec(ctx, `SELECT remove_compression_policy('health_monitoring_samples', if_exists => true)`)
	defer func() {
		_, _ = params.DB.Pool.Exec(ctx, `SELECT add_compression_policy('health_monitoring_samples', INTERVAL '7 days')`)
	}()

	for _, z := range zips {
		zipPath := filepath.Join(params.Source.BaseDir(), z)
		zr, err := zip.OpenReader(zipPath)
		if err != nil {
			slog.Warn("garmin monitoring fit: cannot open zip", "zip", z, "error", err)
			continue
		}

		for _, f := range zr.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".fit") {
				continue
			}

			data, err := readZipEntry(f)
			if err != nil {
				fitErrors++
				continue
			}

			ftype := readFITFileType(data)

			handled, procErr := RouteFITByType(ctx, params.DB, params.ShadowFS, data, params.UserID, ftype)
			if !handled {
				continue // activity or unknown → skip
			}

			fileSize := int64(len(data))
			var batchIDPtr *string
			if params.JobID != "" {
				batchIDPtr = &params.JobID
			}
			tracker := params.DB.NewImportTracker(ctx, params.UserID, "garmin", f.Name, &fileSize, batchIDPtr)
			tracker.Processing(ctx)
			hashArr := sha256.Sum256(data)
			tracker.SetFileHash(ctx, hex.EncodeToString(hashArr[:]))

			if procErr != nil {
				slog.Warn("garmin monitoring fit: process failed", "file", f.Name, "type", ftype, "error", procErr)
				tracker.Error(ctx, procErr)
				fitErrors++
				continue
			}

			imported++
			tracker.Completed(ctx, "")
		}
		zr.Close()
	}

	// Compress all uncompressed chunks
	_, _ = params.DB.Pool.Exec(ctx, `SELECT compress_chunk(c) FROM show_chunks('health_monitoring_samples') c WHERE NOT is_compressed`)
	// Refresh Continuous Aggregate for all historical data
	_, _ = params.DB.Pool.Exec(ctx, `CALL refresh_continuous_aggregate('health_monitoring_daily', NULL, NULL)`)

	slog.Info("garmin monitoring fit: import done",
		"imported", imported,
		"fit_errors", fitErrors,
	)

	return imported, errs
}

// readFITFileTypeFromZipEntry liest nur die ersten 256 Bytes eines ZIP-Eintrags und
// bestimmt daraus den FIT-Dateityp. Das reicht für readFITFileType, da die file_id-
// Nachricht immer als erste Nachricht direkt nach dem FIT-Header steht.
// Deutlich schneller als readZipEntry für bloße Typ-Erkennung (Detect-Phase).
func readFITFileTypeFromZipEntry(f *zip.File) uint8 {
	rc, err := f.Open()
	if err != nil {
		return 0
	}
	defer rc.Close()
	buf := make([]byte, 256)
	n, _ := rc.Read(buf)
	return readFITFileType(buf[:n])
}

// maxMonitoringFileBytes limits monitoring FIT entries to 500 MB to prevent OOM.
const maxMonitoringFileBytes = 500 << 20

// readZipEntry reads the full contents of a zip.File into a byte slice.
// L46: Size-limited to prevent OOM on unexpectedly large zip entries.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxMonitoringFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxMonitoringFileBytes {
		return nil, fmt.Errorf("zip entry %s exceeds %d byte limit", f.Name, maxMonitoringFileBytes)
	}
	return data, nil
}

// readFITFileType reads the file_id.type field from the FIT binary header.
// Returns the numeric file type, or 0 if unreadable.
// FIT file format: 14-byte header, then definition+data messages.
// file_id (global msg 0) contains field_num=0 → file type enum.
func readFITFileType(data []byte) uint8 {
	if len(data) < 20 || string(data[8:12]) != ".FIT" {
		return 0
	}

	// Fast path: scan first few messages to find file_id type.
	hSize := int(data[0])
	pos := hSize
	type defn struct {
		global uint16
		fields []struct{ num, size uint8 }
		total  int
	}
	defs := map[uint8]*defn{}

	for i := 0; i < 20 && pos < len(data)-1; i++ {
		rh := data[pos]
		pos++

		if rh&0x80 != 0 { // Compressed timestamp
			ln := (rh >> 5) & 0x3
			if d, ok := defs[ln]; ok {
				pos += d.total
			}
			continue
		}

		msgType := (rh >> 6) & 0x3
		ln := rh & 0x0F

		if msgType == 1 || msgType == 3 { // Definition
			if pos+5 > len(data) {
				break
			}
			arch := data[pos+1]
			var g uint16
			if arch == 0 {
				g = uint16(data[pos+2]) | uint16(data[pos+3])<<8
			} else {
				g = uint16(data[pos+2])<<8 | uint16(data[pos+3])
			}
			nf := data[pos+4]
			d := &defn{global: g}
			for j := 0; j < int(nf); j++ {
				off := pos + 5 + j*3
				if off+2 >= len(data) {
					break
				}
				d.fields = append(d.fields, struct{ num, size uint8 }{data[off], data[off+1]})
				d.total += int(data[off+1])
			}
			defs[ln] = d
			pos += 5 + int(nf)*3
			if msgType == 3 && pos < len(data) {
				nd := data[pos]
				pos += 1 + int(nd)*3
			}
		} else if msgType == 0 { // Data
			d, ok := defs[ln]
			if !ok {
				break
			}
			if d.global == 0 { // file_id
				off := pos
				for _, f := range d.fields {
					if f.num == 0 && f.size == 1 && off < len(data) {
						return data[off]
					}
					off += int(f.size)
				}
			}
			pos += d.total
		}
	}
	return 0
}

// ReadFITFileType is the exported version of readFITFileType for use outside
// the garmin package (hotfolder, HTTP upload handler).
func ReadFITFileType(data []byte) uint8 {
	return readFITFileType(data)
}

// RouteFITByType routes a non-activity FIT file to the appropriate parser.
// Returns (true, nil) if handled, (false, nil) if the caller should handle it
// (e.g. as activity), or (true, err) on processing error.
func RouteFITByType(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string, ftype uint8) (handled bool, err error) {
	switch ftype {
	case fitFileTypeMonitorMain:
		return true, processMonitorMainFIT(ctx, db, fs, data, userID)
	case fitFileTypeHRVNight:
		return true, processHRVNightFIT(ctx, db, fs, data, userID)
	case fitFileTypeMonitoringA, fitFileTypeMonitoringDaily, fitFileTypeMonitoringB:
		return true, processMonitoringBFIT(ctx, db, fs, data, userID)
	case fitFileTypeSleep:
		return true, processSleepFIT(ctx, db, fs, data, userID)
	case fitFileTypeSpO2:
		return true, processSpO2FIT(ctx, db, fs, data, userID)
	case fitFileTypeSport:
		return true, processSportFIT(ctx, db, fs, data, userID)
	case fitFileTypeWorkout:
		return true, processWorkoutFIT(ctx, db, fs, data, userID)
	case fitFileTypeCourse:
		return true, processCourseFIT(ctx, db, fs, data, userID)
	case fitFileTypeWeight:
		return true, processWeightFIT(ctx, db, fs, data, userID)
	case fitFileTypeSegment:
		return true, processSegmentFIT(ctx, db, fs, data, userID)
	case fitFileTypeActivity:
		return false, nil
	default:
		return false, nil
	}
}

// ProcessMonitoringFIT parses a monitoring FIT file and upserts the health data
// into the database. Returns nil if the file was processed successfully, or an
// error if the file type is not a supported type.
func ProcessMonitoringFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	ftype := readFITFileType(data)
	handled, err := RouteFITByType(ctx, db, fs, data, userID, ftype)
	if err != nil {
		return err
	}
	if !handled {
		return fmt.Errorf("unsupported monitoring FIT file type: %d", ftype)
	}
	return nil
}

func processMonitorMainFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	result := parseMonitorMainFIT(data, userID)
	if result == nil {
		return nil
	}
	for i := range result.HRVSnapshots {
		snap := &result.HRVSnapshots[i]
		if err := db.UpsertHealthHRVSnapshot(ctx, snap); err != nil {
			slog.Warn("processMonitorMainFIT: upsert HRV snapshot failed", "error", err)
		}
	}
	if len(result.HRVNightValues) > 0 {
		if err := db.UpsertHealthHRVNightValuesBatch(ctx, userID, result.HRVNightValues); err != nil {
			slog.Warn("processMonitorMainFIT: upsert HRV night values failed", "error", err)
		}
	}
	for calDate, rhr := range result.RestingHRByDate {
		rhrVal := rhr
		summary := &model.HealthDailySummary{
			UserID:           userID,
			CalendarDate:     calDate,
			RestingHeartRate: &rhrVal,
			SourceType:       "garmin_fit",
		}
		if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
			slog.Warn("processMonitorMainFIT: upsert resting HR failed", "date", calDate, "error", err)
		}
	}
	return nil
}

func processHRVNightFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	// Store original FIT file in ShadowFS for GDPR export and storage migration.
	if len(data) > 14 { // minimal FIT header check
		ts := time.Now()
		storeHealthFIT(ctx, db, fs, data, userID, "hrv_night", fmt.Sprintf("%s_hrv_night.fit", ts.Format("2006-01-02")), ts)
	}

	result := parseHRVNightFIT(data, userID)
	if result == nil {
		return nil
	}
	if len(result.HRVNightValues) > 0 {
		if err := db.UpsertHealthHRVNightValuesBatch(ctx, userID, result.HRVNightValues); err != nil {
			return fmt.Errorf("upsert HRV night values: %w", err)
		}
	}
	for _, readiness := range result.TrainingReadiness {
		if err := db.UpsertHealthReadinessScore(ctx, readiness); err != nil {
			slog.Warn("processHRVNightFIT: upsert training readiness failed", "error", err)
		}
	}
	return nil
}

// hrvsStatusExtra holds the extra HRV-status fields stored in health_snapshots.extra_data.
type hrvStatusExtra struct {
	WeeklyAvgMS            float64 `json:"weekly_avg_ms"`
	LastNightAvgMS         float64 `json:"last_night_avg_ms"`
	LastNight5MinHighMS    float64 `json:"last_night_5min_high_ms"`
	BaselineLowUpperMS     float64 `json:"baseline_low_upper_ms"`
	BaselineBalancedLowMS  float64 `json:"baseline_balanced_lower_ms"`
	BaselineBalancedHighMS float64 `json:"baseline_balanced_upper_ms"`
	Status                 string  `json:"hrv_status"`
}

// parseMonitorMainFIT decodes a Garmin monitoring FIT file (type 68) and extracts:
//   - hrv_status_summary (msg 370) → HRVSnapshots
//   - hrv_value (msg 371, SDK standard) → HRVNightValues (5-min RMSSD during sleep)
//   - MonitoringHrData (msg 211) → RestingHRByDate
func parseMonitorMainFIT(data []byte, userID string) *monitoringFITResult {
	result := &monitoringFITResult{
		RestingHRByDate: make(map[string]int),
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return result
	}

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case 370: // hrv_status_summary
			hrv := mesgdef.NewHrvStatusSummary(msg)
			if hrv.Timestamp.IsZero() {
				continue
			}
			if hrv.LastNightAverage == basetype.Uint16Invalid ||
				hrv.WeeklyAverage == basetype.Uint16Invalid {
				continue
			}
			lastNightAvgMS := float64(hrv.LastNightAverage) / 128.0
			weeklyAvgMS := float64(hrv.WeeklyAverage) / 128.0

			extra := hrvStatusExtra{
				WeeklyAvgMS:    weeklyAvgMS,
				LastNightAvgMS: lastNightAvgMS,
				Status:         hrvStatusString(hrv.Status),
			}
			if hrv.LastNight5MinHigh != basetype.Uint16Invalid {
				extra.LastNight5MinHighMS = float64(hrv.LastNight5MinHigh) / 128.0
			}
			if hrv.BaselineLowUpper != basetype.Uint16Invalid {
				extra.BaselineLowUpperMS = float64(hrv.BaselineLowUpper) / 128.0
			}
			if hrv.BaselineBalancedLower != basetype.Uint16Invalid {
				extra.BaselineBalancedLowMS = float64(hrv.BaselineBalancedLower) / 128.0
			}
			if hrv.BaselineBalancedUpper != basetype.Uint16Invalid {
				extra.BaselineBalancedHighMS = float64(hrv.BaselineBalancedUpper) / 128.0
			}

			extraJSON, _ := json.Marshal(extra)
			extraStr := string(extraJSON)

			tod := "morning"
			snap := model.HealthHRVSnapshot{
				UserID:        userID,
				MeasuredAtUTC: hrv.Timestamp,
				RmssdMS:       &lastNightAvgMS,
				TimeOfDay:     &tod,
				SourceType:    "garmin_fit",
				ExtraData:     &extraStr,
			}
			result.HRVSnapshots = append(result.HRVSnapshots, snap)

		case typedef.MesgNumHrvValue: // 371 — SDK-standard 5-min HRV during sleep
			hrv := mesgdef.NewHrvValue(msg)
			if hrv.Timestamp.IsZero() || hrv.Value == basetype.Uint16Invalid {
				continue
			}
			result.HRVNightValues = append(result.HRVNightValues, model.HealthHRVNightValue{
				UserID:     userID,
				TS:         hrv.Timestamp,
				RmssdMS:    hrv.ValueScaled(),
				SourceType: "garmin_fit",
			})

		case 211: // MonitoringHrData: RestingHeartRate (7-day avg) + CurrentDayRestingHeartRate
			mhr := mesgdef.NewMonitoringHrData(msg)
			if mhr.Timestamp.IsZero() {
				continue
			}
			// Prefer CurrentDayRestingHeartRate (today's RHR) if valid.
			rhr := 0
			if mhr.CurrentDayRestingHeartRate != 0xFF {
				rhr = int(mhr.CurrentDayRestingHeartRate)
			} else if mhr.RestingHeartRate != 0xFF {
				rhr = int(mhr.RestingHeartRate)
			}
			if rhr > 0 && rhr < 250 {
				calDate := mhr.Timestamp.UTC().Format("2006-01-02")
				result.RestingHRByDate[calDate] = rhr
			}
		}
	}

	return result
}

// parseHRVNightFIT decodes a Garmin HRV-night FIT file (type 44) and extracts
// proprietary hrv night values (msg 281) — one 5-minute RMSSD window per message.
// Msg 281 is not in the FIT SDK standard; fields are read raw:
//
//	field 253 = timestamp (uint32, FIT epoch)
//	field 0   = RMSSD value (uint16, scale 128, ms)
func parseHRVNightFIT(data []byte, userID string) *monitoringFITResult {
	result := &monitoringFITResult{
		RestingHRByDate: make(map[string]int),
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return result
	}

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case 281: // Proprietary HRV night 5-min windows.
			// Raw field access: field 253 = timestamp, field 0 = RMSSD (uint16, scale 128).
			rawTS := msg.FieldValueByNum(253).Uint32()
			rawVal := msg.FieldValueByNum(0).Uint16()
			if rawTS == basetype.Uint32Invalid || rawVal == basetype.Uint16Invalid {
				continue
			}
			ts := datetime.ToTime(rawTS)
			if ts.IsZero() {
				continue
			}
			result.HRVNightValues = append(result.HRVNightValues, model.HealthHRVNightValue{
				UserID:     userID,
				TS:         ts,
				RmssdMS:    float64(rawVal) / 128.0,
				SourceType: "garmin_fit",
			})

		case 369: // Training Readiness (proprietary)
			rawTS := msg.FieldValueByNum(253).Uint32()
			if rawTS == basetype.Uint32Invalid {
				continue
			}
			ts := datetime.ToTime(rawTS)
			if ts.IsZero() || ts.Before(time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)) {
				continue
			}
			calDate := ts.UTC().Format("2006-01-02")
			readiness := &model.HealthReadinessScore{
				UserID:       userID,
				CalendarDate: calDate,
				SourceType:   "garmin_fit",
			}
			// Field 0: score (sint16 → int)
			if v := msg.FieldValueByNum(0); v.Valid(basetype.Sint16) {
				s := int(v.Int16())
				readiness.Score = &s
			}
			// Field 5: sleep_score
			if v := msg.FieldValueByNum(5); v.Valid(basetype.Sint16) {
				s := int(v.Int16())
				readiness.SleepScore = &s
			}
			// Field 8: recovery_time_factor_pct (sint16 → float64)
			if v := msg.FieldValueByNum(8); v.Valid(basetype.Sint16) {
				f := float64(v.Int16())
				readiness.RecoveryTimeFactorPct = &f
			}
			// Field 10: acwr_pct
			if v := msg.FieldValueByNum(10); v.Valid(basetype.Sint16) {
				f := float64(v.Int16())
				readiness.AcwrPct = &f
			}
			// Field 12: stress_history_factor_pct
			if v := msg.FieldValueByNum(12); v.Valid(basetype.Sint16) {
				f := float64(v.Int16())
				readiness.StressHistoryFactorPct = &f
			}
			// Field 14: hrv_factor_pct
			if v := msg.FieldValueByNum(14); v.Valid(basetype.Sint16) {
				f := float64(v.Int16())
				readiness.HRVFactorPct = &f
			}
			// Field 16: sleep_history_factor_pct
			if v := msg.FieldValueByNum(16); v.Valid(basetype.Sint16) {
				f := float64(v.Int16())
				readiness.SleepHistoryFactorPct = &f
			}
			result.TrainingReadiness = append(result.TrainingReadiness, readiness)
		}
	}

	return result
}

// hrvStatusString converts typedef.HrvStatus to a human-readable string.
func hrvStatusString(s typedef.HrvStatus) string {
	switch s {
	case typedef.HrvStatusPoor:
		return "poor"
	case typedef.HrvStatusLow:
		return "low"
	case typedef.HrvStatusUnbalanced:
		return "unbalanced"
	case typedef.HrvStatusBalanced:
		return "balanced"
	default:
		return "none"
	}
}

// MonitoringFITImporter is a standalone DataImporter that reads Garmin monitoring FIT files
// (HRV-Status, nächtliche HRV-Werte, Ruhepuls) from the UploadedFiles ZIPs.
// It was previously embedded in StepsImporter; splitting it out allows users to configure
// whether to import the large set of monitoring FITs independently from daily aggregates.
type MonitoringFITImporter struct{}

func (m *MonitoringFITImporter) Name() string { return "monitoring" }
func (m *MonitoringFITImporter) Description() string {
	return "Monitoring-Daten (HRV, Ruhepuls, SpO2) aus Garmin-Sensor-FITs"
}
func (m *MonitoringFITImporter) Priority() int { return 45 }

func (m *MonitoringFITImporter) Detect(src ExportSource) (bool, int, error) {
	zips, _ := src.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if len(zips) == 0 {
		return false, 0, nil
	}
	count := 0
	for _, z := range zips {
		zr, err := zip.OpenReader(filepath.Join(src.BaseDir(), z))
		if err != nil {
			continue
		}
		for _, f := range zr.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".fit") {
				continue
			}
			ft := readFITFileTypeFromZipEntry(f)
			// Count all FIT files that are NOT activities (type 4).
			// This includes type 20 (hourly device monitoring), type 44 (HRV night),
			// type 68 (HRV/resting HR status), and any other non-activity types.
			if ft != 0 && ft != fitFileTypeActivity {
				count++
			}
		}
		zr.Close()
	}
	return count > 0, count, nil
}

func (m *MonitoringFITImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	imported, errs := importMonitoringFITs(ctx, params)
	return &ImportResult{Imported: imported, Errors: errs}, nil
}
