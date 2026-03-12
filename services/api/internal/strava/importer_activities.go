package strava

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// ActivitiesImporter reads activity CSV and imports FIT/GPX files from a Strava export.
//
// activities.csv structure (German headers, German number format):
//
//	Column 0:  Aktivitäts-ID
//	Column 1:  Aktivitätsdatum  — "DD.MM.YYYY, HH:MM:SS"
//	Column 2:  Name der Aktivität
//	Column 3:  Aktivitätsart
//	Column 4:  Aktivitätsbeschreibung
//	Column 12: Dateiname        — e.g. "activities/12345678.gpx.gz"
//	Column 16: Bewegungszeit    — seconds (may be float "2497.0")
//	Column 17: Distanz          — metres (may be float "4045.7")
//	Column 20: Höhenzunahme     — elevation gain metres
//	Column 31: Durchschnittliche Herzfrequenz
//	Column 30: Max. Herzfrequenz (second occurrence)
//	Column 33: Durchschnittliche Watt
//	Column 32: Max. Watt
//	Column 34: Kalorien
//	Column 18: Höchstgeschw.    — m/s
//	Column 19: Durchschn. Geschwindigkeit — m/s
//	Column 36: Durchschnittliche Temperatur — °C
//	Column 55: Wetterlage
//	Column 56: Wetter: Temperatur — °C
//	Column 59: Luftfeuchtigkeit  — 0-1 fraction
//	Column 61: Windgeschwindigkeit — m/s
type ActivitiesImporter struct{}

func (a *ActivitiesImporter) Name() string        { return "activities" }
func (a *ActivitiesImporter) Description() string { return "Aktivitäten (FIT/GPX aus activities.csv)" }
func (a *ActivitiesImporter) Priority() int       { return 30 }

func (a *ActivitiesImporter) Detect(src ExportSource) (bool, int, error) {
	rows, err := src.ReadCSV("activities.csv")
	if err != nil || len(rows) < 2 {
		return false, 0, nil
	}
	return true, len(rows) - 1, nil
}

func (a *ActivitiesImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	rows, err := params.Source.ReadCSV("activities.csv")
	if err != nil {
		slog.Warn("strava activities: cannot read activities.csv", "error", err)
		return result, nil
	}
	if len(rows) < 2 {
		return result, nil
	}

	hdr := csvHeader(rows[0])
	total := len(rows) - 1

	slog.Info("strava activities: found rows", "count", total)

	for i, row := range rows[1:] {
		if params.OnProgress != nil {
			params.OnProgress(i+1, total, fmt.Sprintf("activities: %d/%d %s", i+1, total, colVal(row, hdr["Name der Aktivität"])))
		}

		// --- Parse Strava activity ID ---
		stravaIDStr := colVal(row, hdr["Aktivitäts-ID"])
		stravaID, _ := strconv.ParseInt(stravaIDStr, 10, 64)

		// --- Parse started_at: "DD.MM.YYYY, HH:MM:SS" ---
		dateStr := colVal(row, hdr["Aktivitätsdatum"])
		startedAt, err := parseStravaDateTime(dateStr)
		if err != nil {
			slog.Warn("strava activities: cannot parse date", "date", dateStr, "error", err)
			result.Skipped++
			continue
		}

		// --- Date filter ---
		if params.Options.DateFrom != nil && startedAt.Before(*params.Options.DateFrom) {
			result.Skipped++
			continue
		}
		if params.Options.DateTo != nil && startedAt.After(*params.Options.DateTo) {
			result.Skipped++
			continue
		}

		// --- Get file path ---
		filename := colVal(row, hdr["Dateiname"])

		// --- Build ActivityData ---
		activityName := colVal(row, hdr["Name der Aktivität"])
		description := colVal(row, hdr["Aktivitätsbeschreibung"])
		sportStr := colVal(row, hdr["Aktivitätsart"])
		sportType, subType := mapStravaSportType(sportStr), ""

		// E-Bike normalization
		isEbike := false
		if sportType == "e_biking" || sportType == "e_bike" {
			sportType = "cycling"
			isEbike = true
		}

		// Moving time (seconds)
		movingTimeS, _ := parseGermanInt(colVal(row, hdr["Bewegungszeit"]))
		// Elapsed time (seconds) — first column named "Verstrichene Zeit"
		elapsedTimeS, _ := parseGermanInt(colVal(row, hdr["Verstrichene Zeit"]))

		// Distances: column 17 is metres
		distanceM, _ := parseGermanFloat(colVal(row, hdr["Distanz"]))
		// But multiple "Distanz" columns exist — use index 17 explicitly for metres
		if idx := findNthCol(rows[0], "Distanz", 2); idx >= 0 {
			if v, ok := parseGermanFloat(colVal(row, idx)); ok {
				distanceM = v
			}
		}

		// Elevation gain
		elevGainM, _ := parseGermanFloat(colVal(row, hdr["Höhenzunahme"]))
		// Max / min elevation
		maxElevM, _ := parseGermanFloat(colVal(row, hdr["Max. Höhe"]))
		minElevM, _ := parseGermanFloat(colVal(row, hdr["Min. Höhe"]))

		// Speed
		maxSpeedMS, _ := parseGermanFloat(colVal(row, hdr["Höchstgeschw."]))
		avgSpeedMS, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Geschwindigkeit"]))

		// Heart rate — two "Max. Herzfrequenz" columns: col 7 and col 30
		// Use the second occurrence (col 30) which is numeric bpm
		avgHR, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Herzfrequenz"]))
		maxHR, _ := parseGermanInt(colVal(row, hdr["Max. Herzfrequenz"]))
		// Prefer the second Max. HR occurrence (col 30) if col 7 is the same
		if maxHR2Idx := findNthCol(rows[0], "Max. Herzfrequenz", 2); maxHR2Idx >= 0 {
			if v, ok := parseGermanInt(colVal(row, maxHR2Idx)); ok && v > 0 {
				maxHR = v
			}
		}

		// Power
		avgPower, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Watt"]))
		maxPower, _ := parseGermanInt(colVal(row, hdr["Max. Watt"]))
		normalizedPower, _ := parseGermanFloat(colVal(row, hdr["Gewichtete durchschnittliche Leistung"]))

		// Cadence
		avgCadence, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Trittfrequenz"]))

		// Calories
		calories, _ := parseGermanInt(colVal(row, hdr["Kalorien"]))

		// Temperature
		avgTempC, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Temperatur"]))

		// TSS (from training load if available)
		tss, _ := parseGermanFloat(colVal(row, hdr["Trainingsbelastung"]))

		// Computed end time
		var endedAt time.Time
		if elapsedTimeS > 0 {
			endedAt = startedAt.Add(time.Duration(elapsedTimeS) * time.Second)
		} else if movingTimeS > 0 {
			endedAt = startedAt.Add(time.Duration(movingTimeS) * time.Second)
		}

		// Is indoor: true when no GPS file and no GPS data
		isIndoor := filename == ""

		// Build base ActivityData (will be enriched if GPS file exists)
		data := &model.ActivityData{
			Source:    "Strava",
			SportType: sportType,
			SubType:   subType,
			IsEbike:   isEbike,
			StartedAt: startedAt,
			EndedAt:   endedAt,
		}
		if activityName != "" {
			data.OriginalName = activityName
		}
		if description != "" {
			slog.Debug("strava activities: activity description (not yet stored)", "description", description)
		}

		// Numeric fields from CSV
		if elapsedTimeS > 0 {
			data.ElapsedTimeS = elapsedTimeS
		}
		if movingTimeS > 0 {
			data.MovingTimeS = movingTimeS
		}
		if distanceM > 0 {
			data.DistanceM = distanceM
		}
		if elevGainM > 0 {
			data.ElevationGainM = elevGainM
		}
		if maxElevM > 0 {
			data.MaxElevationM = maxElevM
		}
		if minElevM > 0 {
			data.MinElevationM = minElevM
		}
		if maxSpeedMS > 0 {
			data.MaxSpeedMS = maxSpeedMS
		}
		if avgSpeedMS > 0 {
			data.AvgSpeedMS = avgSpeedMS
		}
		if avgHR > 0 {
			data.AvgHeartRate = avgHR
		}
		if maxHR > 0 {
			data.MaxHeartRate = maxHR
		}
		if avgPower > 0 {
			data.AvgPowerW = avgPower
		}
		if maxPower > 0 {
			data.MaxPowerW = maxPower
		}
		if normalizedPower > 0 {
			data.NormalizedPowerW = normalizedPower
		}
		if avgCadence > 0 {
			data.AvgCadence = avgCadence
		}
		if calories > 0 {
			data.TotalCalories = calories
		}
		if avgTempC != 0 {
			data.AvgTemperatureC = avgTempC
		}
		if tss > 0 {
			data.TSS = tss
		}
		data.IsIndoor = isIndoor

		var fileBytes []byte
		var format string

		// --- Try to load the track file ---
		if filename != "" {
			fileBytes, format, err = readStravaActivityFile(params.Source.BaseDir(), filename)
			if err != nil {
				slog.Warn("strava activities: cannot read activity file",
					"file", filename, "error", err)
				// Fall through: import with CSV metadata only (no track)
			}
		}

		var hashStr string
		if len(fileBytes) > 0 {
			// Parse the file to fill GPS/stream data
			var parseErr error
			var parsedData *model.ActivityData
			switch format {
			case "fit":
				parsedData, parseErr = fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
			case "gpx":
				parsedData, parseErr = gpximport.ParseGPX(bytes.NewReader(fileBytes), filename)
			case "tcx":
				parsedData, parseErr = tcximport.ParseTCX(bytes.NewReader(fileBytes), filename)
			}

			if parseErr != nil {
				slog.Warn("strava activities: parse failed — using CSV metadata only",
					"file", filename, "format", format, "error", parseErr)
				// Continue with CSV-only data
			} else if parsedData != nil {
				// Merge parsed data: prefer parsed GPS/stream data, keep CSV metrics where missing
				mergeActivityData(data, parsedData)
			}

			// Compute hash for storage key
			hash := sha256.Sum256(fileBytes)
			hashStr = hex.EncodeToString(hash[:])
			data.FileHash = hashStr
			data.FileFormat = format
		} else if filename != "" {
			// File reading or decompression failed: generate a synthetic hash based on filename + timestamp
			// so that InsertActivity() gets a FileHash that the download handler can detect
			h := sha256.New()
			h.Write([]byte(filename))
			h.Write([]byte(startedAt.String()))
			hashStr = hex.EncodeToString(h.Sum(nil))
			// Mark this as a fallback hash: "-missing" suffix indicates file was not successfully retrieved
			data.FileHash = hashStr + "-missing"
			// Do not set FileFormat since we don't know it
		}

		// Upload original file to Garage (must happen before PostProcess which writes the sidecar)
		var s3Key string
		if len(fileBytes) > 0 && params.Storage != nil {
			s3Key = fmt.Sprintf("%s/%s.%s", params.UserID, hashStr, format)
			contentType := "application/octet-stream"
			if format == "gpx" || format == "tcx" {
				contentType = "application/xml"
			}
			if err := params.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
				slog.Warn("strava activities: S3 upload failed", "file", filename, "error", err)
			}
		}

		// --- Use ImportService pipeline when available ---
		if params.ImportService != nil {
			importMeta := importer.ImportMetadata{
				StravaActivityID: stravaID,
				SourcePlatform:   "strava",
			}
			actResult, err := params.ImportService.ProcessActivity(ctx, params.UserID, data, fileBytes, importMeta)
			if err != nil {
				slog.Warn("strava activities: ProcessActivity failed", "strava_id", stravaID, "error", err)
				result.Errors = append(result.Errors, fmt.Sprintf("process strava_id=%d: %s", stravaID, err))
				continue
			}

			if actResult.Status == "skipped" || actResult.Status == "enriched" {
				result.Skipped++
				continue
			}

			// New activity inserted: run PostProcess (streams, track, heatmap, Stage 1, sidecar, NATS…)
			if err := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, fileBytes, s3Key, format, filename); err != nil {
				slog.Warn("strava activities: PostProcess failed", "id", actResult.ActivityID, "error", err)
			}

			// Apply Strava-specific enrichment: activity name + weather from CSV
			enrichFields := buildStravaEnrichFields(activityName, row, hdr)
			if len(enrichFields) > 0 {
				if err := params.DB.UpdateEnrichmentFields(ctx, actResult.ActivityID, enrichFields); err != nil {
					slog.Warn("strava activities: update enrichment fields failed", "error", err)
				}
			}

			result.Imported++
			continue
		}

		// --- Legacy path (ImportService not wired): inline duplicate check + insert ---
		match, dupErr := importer.CheckTier1(ctx, params.DB, params.UserID, fileBytes, data, 0, stravaID, 0)
		if dupErr != nil {
			slog.Warn("strava activities: duplicate check failed", "error", dupErr)
			result.Errors = append(result.Errors, fmt.Sprintf("dedup strava_id=%d: %s", stravaID, dupErr))
			continue
		}
		if match != nil {
			switch params.Options.ConflictStrategy {
			case importer.StrategyReplaceAll:
				_ = importer.ReplaceActivity(ctx, params.DB, match.ExistingActivityID, data)
			case importer.StrategySkip:
				// nothing
			default: // enrich_missing
				existing, getErr := params.DB.GetActivity(ctx, match.ExistingActivityID)
				if getErr != nil {
					slog.Warn("strava activities: cannot fetch existing activity for enrich", "id", match.ExistingActivityID, "error", getErr)
				}
				_, _ = importer.EnrichActivity(ctx, params.DB, match.ExistingActivityID, existing, data)
			}
			// Ensure strava_activity_id is set on the existing activity
			if stravaID > 0 {
				_ = params.DB.SetActivityStravaID(ctx, match.ExistingActivityID, stravaID)
			}
			result.Skipped++
			continue
		}

		// --- Insert activity into DB ---
		activity, err := params.DB.InsertActivity(ctx, params.UserID, data)
		if err != nil {
			slog.Warn("strava activities: insert failed", "strava_id", stravaID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("insert strava_id=%d: %s", stravaID, err))
			continue
		}

		// Set strava_activity_id
		if stravaID > 0 {
			if err := params.DB.SetActivityStravaID(ctx, activity.ID, stravaID); err != nil {
				slog.Warn("strava activities: set strava_activity_id failed", "error", err)
			}
		}

		// Apply Strava-specific enrichment: title + weather from CSV
		enrichFields := buildStravaEnrichFields(activityName, row, hdr)
		if len(enrichFields) > 0 {
			if err := params.DB.UpdateEnrichmentFields(ctx, activity.ID, enrichFields); err != nil {
				slog.Warn("strava activities: update enrichment fields failed", "error", err)
			}
		}

		// Update file info
		if len(fileBytes) > 0 && params.Storage != nil {
			metaKey := s3Key + ".meta"
			bucket := "skadia-originals"
			if err := params.DB.UpdateActivityFileInfo(ctx, activity.ID, bucket, s3Key, format, filename, metaKey); err != nil {
				slog.Warn("strava activities: update file info failed", "error", err)
			}
			// Write .meta sidecar
			metaObj := sidecar.GenerateMeta(activity.ID, data)
			if metaBytes, err := metaObj.Marshal(); err == nil {
				if err := params.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
					slog.Warn("strava activities: meta upload failed", "error", err)
				}
			}
		}

		// Insert streams, track, heatmap
		if len(data.Streams) > 0 {
			if err := params.DB.InsertStreams(ctx, activity.ID, data.Streams); err != nil {
				slog.Warn("strava activities: insert streams failed", "error", err)
			}
		}
		if len(data.TrackPoints) > 0 {
			if err := params.DB.InsertTrack(ctx, activity.ID, data.TrackPoints); err != nil {
				slog.Warn("strava activities: insert track failed", "error", err)
			}
			if len(data.TrackPoints) >= 2 {
				var subTypePtr *string
				if data.SubType != "" {
					subTypePtr = &data.SubType
				}
				if err := params.DB.InsertHeatmapLine(ctx, params.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt); err != nil {
					slog.Warn("strava activities: insert heatmap line failed", "error", err)
				}
				interval, _ := params.DB.GetUserHeatmapInterval(ctx, params.UserID)
				if err := params.DB.InsertHeatmapPoints(ctx, params.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
					slog.Warn("strava activities: insert heatmap points failed", "error", err)
				}
			}
		}

		// Insert sensors, laps, splits, lengths
		if len(data.Sensors) > 0 {
			if err := params.DB.InsertSensors(ctx, activity.ID, data.Sensors); err != nil {
				slog.Warn("strava activities: insert sensors failed", "error", err)
			}
		}
		if len(data.Laps) > 0 {
			if err := params.DB.InsertLaps(ctx, activity.ID, data.Laps); err != nil {
				slog.Warn("strava activities: insert laps failed", "error", err)
			}
		}
		if len(data.Splits) > 0 {
			if err := params.DB.InsertSplits(ctx, activity.ID, data.Splits); err != nil {
				slog.Warn("strava activities: insert splits failed", "error", err)
			}
		}
		if len(data.Lengths) > 0 {
			if err := params.DB.InsertLengths(ctx, activity.ID, data.Lengths); err != nil {
				slog.Warn("strava activities: insert lengths failed", "error", err)
			}
		}

		// Stage 1 enrichment (reverse geocode, auto-title, etc.)
		// Skip weather fetch because we already have it from CSV.
		// Events publisher is not available in the legacy path; pass nil (variadic).
		if data.HasGPS && data.StartLat != 0 {
			enrichment.RunStage1(ctx, params.DB, params.UserID, activity.ID, data)
		}

		result.Imported++
	}

	slog.Info("strava activities import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// buildStravaEnrichFields builds the enrichment field map for Strava-specific metadata:
// the activity title and weather data from the CSV row.
func buildStravaEnrichFields(activityName string, row []string, hdr map[string]int) map[string]any {
	fields := make(map[string]any)
	if activityName != "" {
		fields["title"] = activityName
	}
	for k, v := range extractWeatherFields(row, hdr) {
		fields[k] = v
	}
	return fields
}

// readStravaActivityFile reads and optionally decompresses an activity file.
// Supported: .fit, .gpx, .fit.gz, .gpx.gz, .tcx, .tcx.gz
func readStravaActivityFile(baseDir, relPath string) ([]byte, string, error) {
	fullPath := filepath.Join(baseDir, relPath)
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("read file %s: %w", fullPath, err)
	}

	// Decompress gzip if needed
	lower := strings.ToLower(relPath)
	if strings.HasSuffix(lower, ".gz") {
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, "", fmt.Errorf("gzip open %s: %w", relPath, err)
		}
		defer gr.Close()
		raw, err = io.ReadAll(gr)
		if err != nil {
			return nil, "", fmt.Errorf("gzip read %s: %w", relPath, err)
		}
		// Strip .gz suffix for format detection
		lower = strings.TrimSuffix(lower, ".gz")
	}

	// Detect format
	format := detectStravaFormat(raw, lower)
	if format == "" {
		return nil, "", fmt.Errorf("unknown format for %s", relPath)
	}
	return raw, format, nil
}

// detectStravaFormat detects file format from content and filename.
func detectStravaFormat(data []byte, lowerName string) string {
	// FIT: check binary header signature
	if len(data) >= 14 {
		headerSize := int(data[0])
		if (headerSize == 12 || headerSize == 14) && headerSize <= len(data) {
			sig := string(data[headerSize-4 : headerSize])
			if sig == ".FIT" {
				return "fit"
			}
		}
	}
	// XML-based
	content := strings.ToLower(string(data[:min(len(data), 1024)]))
	if strings.Contains(content, "<gpx") {
		return "gpx"
	}
	if strings.Contains(content, "<trainingcenterdatabase") {
		return "tcx"
	}
	// Extension fallback
	switch {
	case strings.HasSuffix(lowerName, ".fit"):
		return "fit"
	case strings.HasSuffix(lowerName, ".gpx"):
		return "gpx"
	case strings.HasSuffix(lowerName, ".tcx"):
		return "tcx"
	}
	return ""
}

// mergeActivityData merges parsedData (GPS/stream data) into base (CSV metadata).
// GPS and stream fields from parsed file take precedence; CSV metrics fill gaps.
func mergeActivityData(base, parsed *model.ActivityData) {
	// Always use GPS/stream/track data from parsed file
	base.HasGPS = parsed.HasGPS
	base.StartLat = parsed.StartLat
	base.StartLng = parsed.StartLng
	base.EndLat = parsed.EndLat
	base.EndLng = parsed.EndLng
	base.Streams = parsed.Streams
	base.TrackPoints = parsed.TrackPoints
	base.Sensors = parsed.Sensors
	base.Laps = parsed.Laps
	base.Splits = parsed.Splits
	base.SplitSummaries = parsed.SplitSummaries
	base.Lengths = parsed.Lengths
	base.ClimbProEvents = parsed.ClimbProEvents
	base.TimeInZoneData = parsed.TimeInZoneData

	// Use parsed file times if more accurate (FIT/GPX have precise timestamps)
	if !parsed.StartedAt.IsZero() {
		base.StartedAt = parsed.StartedAt
	}
	if !parsed.EndedAt.IsZero() {
		base.EndedAt = parsed.EndedAt
	}

	// Fill gaps in metrics from parsed data
	if base.ElapsedTimeS == 0 && parsed.ElapsedTimeS != 0 {
		base.ElapsedTimeS = parsed.ElapsedTimeS
	}
	if base.MovingTimeS == 0 && parsed.MovingTimeS != 0 {
		base.MovingTimeS = parsed.MovingTimeS
	}
	if base.DistanceM == 0 && parsed.DistanceM != 0 {
		base.DistanceM = parsed.DistanceM
	}
	if base.ElevationGainM == 0 && parsed.ElevationGainM != 0 {
		base.ElevationGainM = parsed.ElevationGainM
	}
	if base.AvgHeartRate == 0 && parsed.AvgHeartRate != 0 {
		base.AvgHeartRate = parsed.AvgHeartRate
	}
	if base.MaxHeartRate == 0 && parsed.MaxHeartRate != 0 {
		base.MaxHeartRate = parsed.MaxHeartRate
	}
	if base.AvgPowerW == 0 && parsed.AvgPowerW != 0 {
		base.AvgPowerW = parsed.AvgPowerW
	}
	if base.MaxPowerW == 0 && parsed.MaxPowerW != 0 {
		base.MaxPowerW = parsed.MaxPowerW
	}

	// Sport type from parsed file may be more specific
	if parsed.SportType != "" && parsed.SportType != "generic" {
		base.SportType = parsed.SportType
		base.SubType = parsed.SubType
		base.FitSportID = parsed.FitSportID
		base.FitSubSportID = parsed.FitSubSportID
	}

	// UserProfileImport from FIT
	if parsed.UserProfileImport != nil {
		base.UserProfileImport = parsed.UserProfileImport
	}
}

// parseStravaDateTime parses "DD.MM.YYYY, HH:MM:SS" into a time.Time (UTC).
func parseStravaDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	// Format: "13.08.2019, 09:54:21"
	t, err := time.Parse("02.01.2006, 15:04:05", s)
	if err != nil {
		// Try without comma
		t, err = time.Parse("02.01.2006 15:04:05", s)
	}
	return t, err
}

// mapStravaSportType maps German Strava sport type strings to Skadia canonical sport types.
func mapStravaSportType(stravaSport string) string {
	// Normalize
	s := strings.TrimSpace(stravaSport)
	switch s {
	// Cycling
	case "Radfahrt", "Radfahren":
		return "cycling"
	case "E-Bike-Radfahrt", "E-Bike":
		return "e_biking" // normalized to cycling + is_ebike downstream
	case "Mountainbiking":
		return "mountain_biking"
	case "Gravelbike", "Gravel-Radfahrt":
		return "gravel_cycling"
	case "Virtuelles Radfahren":
		return "virtual_ride"
	case "Indoor-Radfahren", "Spinning":
		return "indoor_cycling"
	// Running
	case "Lauf", "Laufen":
		return "running"
	case "Trailrunning", "Trailrun":
		return "trail_running"
	case "Treadmill-Lauf", "Laufband":
		return "treadmill_running"
	// Hiking / Walking
	case "Wandern", "Wanderung":
		return "hiking"
	case "Spaziergang":
		return "walking"
	// Swimming
	case "Schwimmen":
		return "swimming"
	case "Freiwasserschwimmen":
		return "open_water_swimming"
	// Water sports
	case "Stand-up-Paddeln", "SUP":
		return "stand_up_paddling"
	case "Kajak fahren", "Kajak":
		return "kayaking"
	case "Rudern":
		return "rowing"
	case "Kanufahren":
		return "kayaking"
	// Winter sports
	case "Skifahren", "Ski alpin":
		return "alpine_skiing"
	case "Langlauf", "Skilanglauf":
		return "cross_country_skiing"
	case "Snowboarden", "Snowboard":
		return "snowboarding"
	case "Schneeschuhwandern":
		return "snowshoeing"
	// Fitness
	case "Krafttraining", "Gewichtheben":
		return "strength_training"
	case "Yoga":
		return "yoga"
	case "Crossfit":
		return "training"
	case "HIIT":
		return "hiit"
	// Climbing
	case "Klettern", "Felsenklettern", "Bouldern":
		return "rock_climbing"
	// Multi-sport
	case "Triathlon":
		return "triathlon"
	// Other
	case "Multisport":
		return "multi_sport"
	case "Sonstiges", "Andere":
		return "generic"
	default:
		// Unknown type — map to generic
		slog.Debug("strava: unmapped sport type", "strava_type", s)
		return "generic"
	}
}

// findNthCol finds the index of the nth occurrence (1-based) of a column name in the header row.
func findNthCol(headerRow []string, colName string, n int) int {
	count := 0
	for i, h := range headerRow {
		if strings.TrimSpace(h) == colName {
			count++
			if count == n {
				return i
			}
		}
	}
	return -1
}

// extractWeatherFields builds a map of weather fields from a Strava CSV row.
// Strava stores weather as numeric codes and metric values.
func extractWeatherFields(row []string, hdr map[string]int) map[string]any {
	fields := make(map[string]any)

	// Temperature in Celsius
	if idx, ok := hdr["Wetter: Temperatur"]; ok {
		if temp, ok2 := parseGermanFloat(colVal(row, idx)); ok2 && temp != 0 {
			fields["weather_temp_c"] = temp
		}
	}

	// Humidity: 0-1 fraction → convert to integer 0-100
	if idx, ok := hdr["Luftfeuchtigkeit"]; ok {
		if hum, ok2 := parseGermanFloat(colVal(row, idx)); ok2 && hum > 0 {
			humidity := int(hum * 100)
			if hum > 1 { // already 0-100
				humidity = int(hum)
			}
			fields["weather_humidity"] = humidity
		}
	}

	// Wind speed m/s
	if idx, ok := hdr["Windgeschwindigkeit"]; ok {
		if wind, ok2 := parseGermanFloat(colVal(row, idx)); ok2 && wind > 0 {
			fields["weather_wind_speed_ms"] = wind
		}
	}

	// Weather condition (Wetterlage) — Strava uses numeric codes
	// We map them to descriptive strings
	if idx, ok := hdr["Wetterlage"]; ok {
		if condStr := colVal(row, idx); condStr != "" && condStr != "0" {
			if cond, ok2 := parseGermanFloat(condStr); ok2 && cond != 0 {
				if condLabel := mapStravaWeatherCondition(int(cond)); condLabel != "" {
					fields["weather_condition"] = condLabel
				}
			}
		}
	}

	return fields
}

// mapStravaWeatherCondition maps Strava weather condition codes to descriptive strings.
// Strava uses DarkSky-compatible condition codes.
func mapStravaWeatherCondition(code int) string {
	// DarkSky / Strava precipitation type codes:
	// The "Wetterlage" field appears to be a general condition code
	// Common mappings based on DarkSky icon names
	switch code {
	case 1:
		return "clear-day"
	case 2:
		return "clear-night"
	case 3:
		return "rain"
	case 4:
		return "snow"
	case 5:
		return "sleet"
	case 6:
		return "wind"
	case 7:
		return "fog"
	case 8:
		return "cloudy"
	case 9:
		return "partly-cloudy-day"
	case 10:
		return "partly-cloudy-night"
	default:
		return ""
	}
}
