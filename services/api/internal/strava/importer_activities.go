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

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// maxStravaFileSize is the maximum decompressed size accepted from a Strava gzip file (500 MB).
const maxStravaFileSize = 500 << 20

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

	hdr := CSVHeader(rows[0])
	total := len(rows) - 1

	// Detect locale for logging (aliases make both languages work transparently).
	if _, ok := hdr["Activity ID"]; ok {
		slog.Info("strava activities: detected English-language export")
	}

	slog.Info("strava activities: found rows", "count", total)

	for i, row := range rows[1:] {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if params.OnProgress != nil {
			params.OnProgress(i+1, total, fmt.Sprintf("activities: %d/%d %s", i+1, total, colVal(row, hdr["Name der Aktivität"])))
		}

		// --- Import log tracking ---
		logFilename := colVal(row, hdr["Dateiname"])
		if logFilename == "" {
			logFilename = colVal(row, hdr["Aktivitäts-ID"])
		}
		var batchIDPtr *string
		if params.JobID != "" {
			batchIDPtr = &params.JobID
		}
		tracker := params.DB.NewImportTracker(ctx, params.UserID, "strava", logFilename, nil, batchIDPtr)
		tracker.Processing(ctx)

		// --- Parse Strava activity ID ---
		stravaIDStr := colVal(row, hdr["Aktivitäts-ID"])
		stravaID, _ := strconv.ParseInt(stravaIDStr, 10, 64)

		// --- Parse started_at: "DD.MM.YYYY, HH:MM:SS" ---
		dateStr := colVal(row, hdr["Aktivitätsdatum"])
		startedAt, err := parseStravaDateTime(dateStr)
		if err != nil {
			slog.Warn("strava activities: cannot parse date", "date", dateStr, "error", err)
			result.Skipped++
			tracker.Skipped(ctx, "date_parse_error", nil)
			continue
		}

		// --- Date filter ---
		if params.Options.DateFrom != nil && startedAt.Before(*params.Options.DateFrom) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}
		if params.Options.DateTo != nil && startedAt.After(*params.Options.DateTo) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}

		// --- Get file path ---
		filename := colVal(row, hdr["Dateiname"])

		// --- Build ActivityData ---
		activityName := colVal(row, hdr["Name der Aktivität"])
		description := colVal(row, hdr["Aktivitätsbeschreibung"])
		sportStr := colVal(row, hdr["Aktivitätsart"])
		sportFields, err := normalizeStravaSportFields(sportStr)
		if err != nil {
			slog.Warn("strava activities: cannot normalize sport type", "sport", sportStr, "error", err)
			result.Skipped++
			tracker.Skipped(ctx, "sport_type_error", nil)
			continue
		}
		sportType, subType := sportFields.SportType, sportFields.SubType
		isElectric := sportFields.IsElectric
		isVirtual := sportFields.IsVirtual
		isCommute, _ := parseBool(colVal(row, hdr["Pendeln"]))

		// Moving time (seconds)
		movingTimeS, _ := parseGermanInt(colVal(row, hdr["Bewegungszeit"]))
		// Elapsed time (seconds) — first column named "Verstrichene Zeit"
		elapsedTimeS, _ := parseGermanInt(colVal(row, hdr["Verstrichene Zeit"]))

		// Distances: column 17 is metres
		distanceM, _ := parseGermanFloat(colVal(row, hdr["Distanz"]))
		// But multiple "Distanz"/"Distance" columns exist — use the 2nd explicitly for metres.
		if idx := findNthColMulti(rows[0], 2, "Distanz", "Distance"); idx >= 0 {
			if v, ok := parseNumber(colVal(row, idx)); ok {
				distanceM = v
			}
		}

		// Elevation gain
		elevGainM, _ := parseGermanFloat(colVal(row, hdr["Höhenzunahme"]))
		// Max / min elevation
		maxElevM, hasMaxElev := parseGermanFloat(colVal(row, hdr["Max. Höhe"]))
		minElevM, hasMinElev := parseGermanFloat(colVal(row, hdr["Min. Höhe"]))

		// Speed
		maxSpeedMS, _ := parseGermanFloat(colVal(row, hdr["Höchstgeschw."]))
		avgSpeedMS, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Geschwindigkeit"]))

		// Heart rate — two "Max. Herzfrequenz"/"Max Heart Rate" columns: col 7 and col 30
		// Use the second occurrence (col 30) which is numeric bpm.
		avgHR, _ := parseNumber(colVal(row, hdr["Durchschnittliche Herzfrequenz"]))
		maxHR, _ := parseInt(colVal(row, hdr["Max. Herzfrequenz"]))
		// Prefer the second Max. HR occurrence (col 30) if col 7 is the same
		if maxHR2Idx := findNthColMulti(rows[0], 2, "Max. Herzfrequenz", "Max Heart Rate"); maxHR2Idx >= 0 {
			if v, ok := parseInt(colVal(row, maxHR2Idx)); ok && v > 0 {
				maxHR = v
			}
		}

		// Power: intentionally not read from CSV — Strava often estimates power
		// even without a power meter. Values are taken from the activity file only.

		// Cadence
		avgCadence, _ := parseGermanFloat(colVal(row, hdr["Durchschnittliche Trittfrequenz"]))

		// Calories
		calories, _ := parseGermanInt(colVal(row, hdr["Kalorien"]))

		// Temperature
		avgTempC, hasTempC := parseGermanFloat(colVal(row, hdr["Durchschnittliche Temperatur"]))

		// TSS (from training load if available)
		tss, _ := parseGermanFloat(colVal(row, hdr["Trainingsbelastung"]))

		// Computed end time
		var endedAt time.Time
		if elapsedTimeS > 0 {
			endedAt = startedAt.Add(time.Duration(elapsedTimeS) * time.Second)
		} else if movingTimeS > 0 {
			endedAt = startedAt.Add(time.Duration(movingTimeS) * time.Second)
		}

		// Strava: filename=="" is a valuable signal (no GPS file = often indoor).
		// Combine with taxonomy-based classification.
		isIndoor := sportFields.IsIndoor || sport.IsIndoor(sportType, subType) || filename == ""

		// Build base ActivityData (will be enriched if GPS file exists)
		data := &model.ActivityData{
			Source:     "Strava",
			SportType:  sportType,
			SubType:    subType,
			IsElectric: isElectric,
			IsVirtual:  isVirtual,
			IsCommute:  isCommute,
			StartedAt:  startedAt,
			EndedAt:    endedAt,
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
		if hasMaxElev {
			data.MaxElevationM = maxElevM
		}
		if hasMinElev {
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
		if avgCadence > 0 {
			data.AvgCadence = avgCadence
		}
		if calories > 0 {
			data.TotalCalories = calories
		}
		if hasTempC {
			data.AvgTemperatureC = avgTempC
		}
		if tss > 0 {
			data.TSS = tss
		}
		data.IsIndoor = isIndoor

		var fileBytes []byte
		var format string
		var fileSportType string // set during parse if file has a sport type
		var conflictPlatformSnapshot sportConflictSnapshot
		var conflictFileSnapshot sportConflictSnapshot

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
			// For FIT files, try multi-session parsing to handle multisport activities.
			if format == "fit" && params.ImportService != nil {
				sessions, msParseErr := fitimport.ParseMultiSessionFIT(fileBytes, filename)
				if msParseErr == nil && len(sessions) > 1 {
					// Multi-session FIT: upload via ShadowFS, then delegate.
					hash := sha256.Sum256(fileBytes)
					msHash := hex.EncodeToString(hash[:])
					var msFileID string
					if params.ShadowFS != nil {
						rec, putErr := params.ShadowFS.Put(ctx, shadowfs.FileEntry{
							UserID:      params.UserID,
							FileType:    "activity",
							ContentType: "application/octet-stream",
							FileName:    filepath.Base(filename), // L48: strip directory prefix
							SizeBytes:   int64(len(fileBytes)),
							FileHash:    msHash,
							Tags:        shadowfs.ActivitySchemaTags(importer.MultisportActivitySchemaTagsInput("fit", "strava", sessions)),
						}, bytes.NewReader(fileBytes))
						if putErr != nil {
							slog.Error("strava activities: ShadowFS.Put multisport failed — skipping activity", "file", filename, "error", putErr)
							result.Errors = append(result.Errors, fmt.Sprintf("multisport upload strava_id=%d: %s", stravaID, putErr))
							tracker.Error(ctx, putErr)
							continue
						}
						msFileID = rec.ID
					}

					importMeta := importer.ImportMetadata{
						StravaActivityID: stravaID,
						SourcePlatform:   "strava",
						ConflictStrategy: params.Options.ConflictStrategy,
					}
					_, msErr := params.ImportService.ProcessMultisport(
						ctx, params.UserID, sessions, fileBytes, msHash,
						msFileID, "fit", filename, importMeta,
					)
					if msErr != nil {
						slog.Warn("strava activities: ProcessMultisport failed", "strava_id", stravaID, "error", msErr)
						result.Errors = append(result.Errors, fmt.Sprintf("multisport strava_id=%d: %s", stravaID, msErr))
						tracker.Error(ctx, msErr)
					} else {
						result.Imported += len(sessions) // L51: count each child session
						tracker.Completed(ctx, "")
					}
					continue
				}
				// Single-session or parse error: fall through to normal parsing below
			}

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
				// Save file sport type BEFORE merge (merge may change it)
				fileSportType = parsedData.SportType

				// Merge parsed data: prefer parsed GPS/stream data, keep CSV metrics where missing
				mergeActivityData(data, parsedData)

				// --- Sport conflict check ---
				// Compare CSV sport (post E-Bike normalization) vs file sport using SegmentGroup.
				// Only check if we have a real file sport (not empty/unknown).
				// Note: "generic" is allowed through — generic vs generic triggers a queue entry.
				if fileSportType != "" && fileSportType != "unknown" {
					csvGroup := sport.SegmentGroup(sportType) // sportType from CSV (post E-Bike normalization)
					fileGroup := sport.SegmentGroup(fileSportType)
					platformSnapshot := sportConflictSnapshot{
						Sport:      sportType,
						SubType:    subType,
						IsElectric: isElectric,
						IsIndoor:   isIndoor,
						IsVirtual:  isVirtual,
					}
					fileSnapshot := sportConflictSnapshot{
						Sport:      parsedData.SportType,
						SubType:    parsedData.SubType,
						IsElectric: parsedData.IsElectric,
						IsIndoor:   parsedData.IsIndoor,
						IsVirtual:  parsedData.IsVirtual,
					}

					conflict := false
					if csvGroup == "generic" && fileGroup == "generic" {
						// Both unknown → queue
						conflict = true
					} else if csvGroup != "generic" && fileGroup != "generic" && csvGroup != fileGroup {
						// Different specific groups → queue, file wins
						conflict = true
						data.SportType = fileSportType
						// Restore SubType from parsed file too — SportType + SubType are a pair,
						// mixing sources creates inconsistent combinations (e.g. cycling type + hiking subtype)
						if parsedData != nil {
							data.SubType = parsedData.SubType
						}
					} else if sportConflictSnapshotsConflict(platformSnapshot, fileSnapshot) {
						conflict = true
					}
					// csvGroup == "generic" && fileGroup != "generic" → file already wins (from merge)
					// csvGroup != "generic" && fileGroup == "generic" → CSV already wins (from merge)
					// Same group → merge result stands

					if conflict {
						conflictPlatformSnapshot = platformSnapshot
						conflictFileSnapshot = fileSnapshot
						data.SportConflictPending = true
					}
				}
			}

			// Compute hash for storage key
			hash := sha256.Sum256(fileBytes)
			hashStr = hex.EncodeToString(hash[:])
			tracker.SetFileHash(ctx, hashStr)
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

		// File upload is deferred until after ProcessActivity confirms a "created" status.
		// This prevents orphaned files in ShadowFS for duplicates/skipped activities.
		var fileID string

		var csvSourceFile *stravaCSVSourceFile
		if len(fileBytes) == 0 {
			var csvErr error
			csvSourceFile, csvErr = buildStravaCSVSourceFile(params.UserID, stravaID, data, activityName, row, hdr)
			if csvErr != nil {
				slog.Warn("strava activities: build CSV-only source file failed", "strava_id", stravaID, "error", csvErr)
			} else {
				hashStr = csvSourceFile.hash
				format = "strava_csv"
			}
		}

		// --- ImportService pipeline ---
		importMeta := importer.ImportMetadata{
			StravaActivityID: stravaID,
			SourcePlatform:   "strava",
			ConflictStrategy: params.Options.ConflictStrategy,
		}
		actResult, err := params.ImportService.ProcessActivity(ctx, params.UserID, data, fileBytes, importMeta)
		if err != nil {
			slog.Warn("strava activities: ProcessActivity failed", "strava_id", stravaID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("process strava_id=%d: %s", stravaID, err))
			tracker.Error(ctx, err)
			continue
		}

		if actResult.Status == "skipped" {
			result.Skipped++
			tracker.Skipped(ctx, "duplicate", &actResult.ActivityID)
			continue
		}
		if actResult.Status == "enriched" || actResult.Status == "replaced" {
			// M21: Run PostProcess for enriched/replaced too so heatmap+streams stay up to date.
			if postErr := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, fileID, format, filename); postErr != nil {
				slog.Warn("strava activities: PostProcess failed (enriched/replaced)", "strava_id", stravaID, "error", postErr)
			}
			result.Skipped++ // counts as "not newly created" for import stats
			tracker.Completed(ctx, actResult.ActivityID)
			continue
		}

		// New activity inserted — upload original file via ShadowFS.
		// Only reached for "created" status, preventing orphaned files for skipped/enriched/replaced.
		if len(fileBytes) > 0 {
			contentType := "application/octet-stream"
			if format == "gpx" || format == "tcx" {
				contentType = "application/xml"
			}
			if params.ShadowFS != nil {
				rec, putErr := params.ShadowFS.Put(ctx, shadowfs.FileEntry{
					UserID:      params.UserID,
					FileType:    "activity",
					ContentType: contentType,
					FileName:    filepath.Base(filename), // L48: strip directory prefix
					SizeBytes:   int64(len(fileBytes)),
					FileHash:    hashStr,
					Tags: shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput{
						Format:         format,
						Source:         "strava",
						StartedAt:      data.StartedAt,
						SportType:      data.SportType,
						SubType:        data.SubType,
						Title:          data.Title,
						DistanceM:      data.DistanceM,
						ElapsedTimeS:   data.ElapsedTimeS,
						ElevationGainM: data.ElevationGainM,
						IsElectric:     data.IsElectric,
						IsIndoor:       data.IsIndoor,
						IsVirtual:      data.IsVirtual,
					}),
				}, bytes.NewReader(fileBytes))
				if putErr != nil {
					slog.Warn("strava activities: ShadowFS.Put failed", "file", filename, "error", putErr)
				} else {
					fileID = rec.ID
				}
			}
		} else if csvSourceFile != nil && params.ShadowFS != nil {
			rec, putErr := putStravaCSVSourceFile(ctx, params.ShadowFS, csvSourceFile)
			if putErr != nil {
				slog.Warn("strava activities: ShadowFS.Put for CSV-only failed", "error", putErr)
			} else {
				fileID = rec.ID
			}
		}

		// New activity inserted: run PostProcess (streams, track, heatmap, Stage 1, NATS…)
		if err := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, fileID, format, filename); err != nil {
			slog.Warn("strava activities: PostProcess failed", "id", actResult.ActivityID, "error", err)
		}
		tracker.Completed(ctx, actResult.ActivityID)

		// Apply Strava-specific enrichment: activity name + weather from CSV
		enrichFields := buildStravaEnrichFields(activityName)
		if len(enrichFields) > 0 {
			if err := params.DB.UpdateEnrichmentFields(ctx, actResult.ActivityID, enrichFields); err != nil {
				slog.Warn("strava activities: update enrichment fields failed", "error", err)
			}
		}

		// Create sport conflict queue entry if detected
		if data.SportConflictPending {
			if conflictPlatformSnapshot.Sport == "" {
				conflictPlatformSnapshot = sportConflictSnapshot{Sport: sportType, SubType: subType, IsElectric: isElectric, IsIndoor: isIndoor, IsVirtual: isVirtual}
			}
			if conflictFileSnapshot.Sport == "" {
				conflictFileSnapshot = sportConflictSnapshot{Sport: data.SportType, SubType: data.SubType, IsElectric: data.IsElectric, IsIndoor: data.IsIndoor, IsVirtual: data.IsVirtual}
			}
			if qErr := createSportConflictQueueEntry(ctx, params.DB, params.UserID,
				actResult.ActivityID,
				conflictPlatformSnapshot,
				conflictFileSnapshot,
				format); qErr != nil {
				slog.Warn("sport conflict: failed to create queue entry",
					"activity", actResult.ActivityID, "error", qErr)
			}
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

// buildStravaEnrichFields builds the enrichment field map for CSV activity metadata.
func buildStravaEnrichFields(activityName string) map[string]any {
	fields := make(map[string]any)
	if activityName != "" {
		fields["title"] = activityName
	}
	return fields
}

func csvRowToMap(row []string, hdr map[string]int) map[string]string {
	m := make(map[string]string, len(hdr))
	for key, idx := range hdr {
		if idx < len(row) && row[idx] != "" {
			m[key] = row[idx]
		}
	}
	return m
}

type stravaCSVSourceFile struct {
	entry shadowfs.FileEntry
	data  []byte
	hash  string
}

func buildStravaCSVSourceFile(userID string, stravaID int64, data *model.ActivityData, activityName string, row []string, hdr map[string]int) (*stravaCSVSourceFile, error) {
	csvJSON := &sidecar.ManualActivityFile{
		SchemaVersion:  1,
		FileFormat:     "strava_csv",
		SportType:      data.SportType,
		SubType:        data.SubType,
		Title:          activityName,
		StartedAt:      data.StartedAt,
		ElapsedTimeS:   float64(data.ElapsedTimeS),
		DistanceM:      data.DistanceM,
		ElevationGainM: data.ElevationGainM,
		Source:         "Strava",
		SourceFields:   csvRowToMap(row, hdr),
		CreatedAt:      time.Now().UTC(),
	}
	detHash := sha256.Sum256(fmt.Appendf(nil, "strava_csv|%d|%s", stravaID, data.StartedAt.Format(time.RFC3339)))
	hashStr := hex.EncodeToString(detHash[:])
	jsonBytes, err := csvJSON.Marshal()
	if err != nil {
		return nil, err
	}
	data.FileHash = hashStr
	data.FileFormat = "strava_csv"
	return &stravaCSVSourceFile{
		data: jsonBytes,
		hash: hashStr,
		entry: shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "activity",
			ContentType: "application/json",
			FileName:    fmt.Sprintf("strava_%d.json", stravaID),
			SizeBytes:   int64(len(jsonBytes)),
			FileHash:    hashStr,
			Tags: shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput{
				Format:         "strava_csv",
				Source:         "strava",
				StartedAt:      data.StartedAt,
				SportType:      data.SportType,
				SubType:        data.SubType,
				Title:          activityName,
				DistanceM:      data.DistanceM,
				ElapsedTimeS:   data.ElapsedTimeS,
				ElevationGainM: data.ElevationGainM,
				IsElectric:     data.IsElectric,
				IsIndoor:       data.IsIndoor,
				IsVirtual:      data.IsVirtual,
			}),
		},
	}, nil
}

func putStravaCSVSourceFile(ctx context.Context, fs shadowfs.ShadowFS, file *stravaCSVSourceFile) (shadowfs.FileRecord, error) {
	return fs.Put(ctx, file.entry, bytes.NewReader(file.data))
}

// readStravaActivityFile reads and optionally decompresses an activity file.
// Supported: .fit, .gpx, .fit.gz, .gpx.gz, .tcx, .tcx.gz
func readStravaActivityFile(baseDir, relPath string) ([]byte, string, error) {
	// M26: Path containment check — reject traversal attempts from CSV filenames.
	fullPath := filepath.Join(baseDir, relPath)
	cleanFull := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(baseDir) + string(filepath.Separator)
	if !strings.HasPrefix(cleanFull, cleanBase) {
		return nil, "", fmt.Errorf("path traversal blocked: %q escapes base %q", relPath, baseDir)
	}
	// L43: Size-limited read to prevent OOM on unexpectedly large files.
	f, err := os.Open(cleanFull)
	if err != nil {
		return nil, "", fmt.Errorf("read file %s: %w", fullPath, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxStravaFileSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read file %s: %w", fullPath, err)
	}
	if int64(len(raw)) > maxStravaFileSize {
		return nil, "", fmt.Errorf("file %s exceeds maximum size of %d bytes", relPath, maxStravaFileSize)
	}

	// Decompress gzip if needed
	lower := strings.ToLower(relPath)
	if strings.HasSuffix(lower, ".gz") {
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, "", fmt.Errorf("gzip open %s: %w", relPath, err)
		}
		defer gr.Close()
		// L44: Error on truncation by reading one byte beyond the limit.
		decompressed, readErr := io.ReadAll(io.LimitReader(gr, maxStravaFileSize+1))
		if readErr != nil {
			return nil, "", fmt.Errorf("gzip read %s: %w", relPath, readErr)
		}
		if int64(len(decompressed)) > maxStravaFileSize {
			return nil, "", fmt.Errorf("gzip file %s exceeds maximum decompressed size of %d bytes", relPath, maxStravaFileSize)
		}
		raw = decompressed
		// Strip .gz suffix for format detection
		lower = strings.TrimSuffix(lower, ".gz")
	}

	// Detect format
	format := importutil.DetectFormat(raw, lower)
	if format == "" {
		return nil, "", fmt.Errorf("unknown format for %s", relPath)
	}
	return raw, format, nil
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
	if base.NormalizedPowerW == 0 && parsed.NormalizedPowerW != 0 {
		base.NormalizedPowerW = parsed.NormalizedPowerW
	}

	// Sport type merge strategy:
	// - Strava CSV "Aktivitätsart" is a lossy localized string that collapses specific
	//   types into broad categories (e.g., cycling/mountain → "Radfahren", bouldering → "Klettern")
	// - FIT/GPX files contain precise sport/sub-sport IDs from the recording device
	// - When Strava gave a broad category and the device has a more specific variant
	//   in the same sport family, prefer the device data
	// - When they disagree on family, prefer Strava (user may have corrected it,
	//   e.g. Wahoo ELEMNT records hiking as cycling but Strava shows "Wandern")
	base.FitSportID = parsed.FitSportID
	base.FitSubSportID = parsed.FitSubSportID
	if parsed.SportType != "" && parsed.SportType != "generic" && parsed.SportType != "unknown" {
		if base.SportType == "" || base.SportType == "generic" || base.SportType == "unknown" {
			// Strava couldn't map the sport → use device data
			base.SportType = parsed.SportType
			base.SubType = parsed.SubType
		} else if isSportRefinement(base.SportType, parsed.SportType) {
			// Device has more specific sport in same family → prefer device
			base.SportType = parsed.SportType
			base.SubType = parsed.SubType
		} else if base.SportType == parsed.SportType && base.SubType == "" && parsed.SubType != "" && parsed.SubType != "generic" {
			// Same sport type but device has sub-type that Strava CSV lost (e.g. bouldering)
			base.SubType = parsed.SubType
		}
	}

	// IsElectric: device data is authoritative (FIT has sub-sport ID for e-bike detection)
	if parsed.IsElectric {
		base.IsElectric = true
	}
	if parsed.IsIndoor {
		base.IsIndoor = true
	}
	if parsed.IsVirtual {
		base.IsVirtual = true
	}
	if parsed.IsCommute {
		base.IsCommute = true
	}

	// UserProfileImport from FIT
	if parsed.UserProfileImport != nil {
		base.UserProfileImport = parsed.UserProfileImport
	}
}

// parseStravaDateTime parses Strava date strings in German ("DD.MM.YYYY, HH:MM:SS")
// or English ("Jun 29, 2025, 3:30:55 PM") format into a time.Time (UTC).
func parseStravaDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	// English format: "Jun 29, 2025, 3:30:55 PM" or "Jun 4, 2025, 2:00:00 PM"
	if t, err := time.Parse("Jan 2, 2006, 3:04:05 PM", s); err == nil {
		return t, nil
	}
	// English 24h variant (some locales)
	if t, err := time.Parse("Jan 2, 2006, 15:04:05", s); err == nil {
		return t, nil
	}

	// German format: "13.08.2019, 09:54:21"
	if t, err := time.Parse("02.01.2006, 15:04:05", s); err == nil {
		return t, nil
	}
	// German without comma
	if t, err := time.Parse("02.01.2006 15:04:05", s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse date %q: no known format matched", s)
}

type normalizedStravaSport struct {
	SportType  string
	SubType    string
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
}

func normalizeStravaSportFields(stravaSport string) (normalizedStravaSport, error) {
	resolved, err := sport.ResolveSport(sport.ResolveInput{Source: sport.SourceStrava, RawSport: stravaSport})
	if err != nil {
		return normalizedStravaSport{}, err
	}
	if !resolved.Resolved {
		slog.Debug("strava: unmapped sport type", "strava_type", strings.TrimSpace(stravaSport))
		return normalizedStravaSport{SportType: "unknown"}, nil
	}
	modifiers := sport.ApplyModifierPatch(sport.ModifierValues{}, resolved.Patch)
	out := normalizedStravaSport{
		SportType:  resolved.Tuple.SportType,
		SubType:    resolved.Tuple.SubType,
		IsElectric: modifiers.IsElectric,
		IsIndoor:   modifiers.IsIndoor,
		IsVirtual:  modifiers.IsVirtual,
	}
	return out, nil
}

// isSportRefinement returns true when 'specific' is a known more-specific variant
// of 'broad' within the same sport family.
func isSportRefinement(broad, specific string) bool {
	if broad == specific {
		return false
	}
	if broad == "" || specific == "" || broad == "generic" || broad == "unknown" || specific == "generic" || specific == "unknown" {
		return false
	}
	broadDef, broadOK := sport.GetSportDef(broad)
	specificDef, specificOK := sport.GetSportDef(specific)
	if !broadOK || !specificOK {
		return false
	}
	if broadDef.Traits.ActivityForm != "fallback" && broadDef.Traits.ActivityForm != "family" {
		return false
	}
	if specificDef.Traits.ActivityForm == "fallback" || specificDef.Traits.ActivityForm == "family" {
		return false
	}
	if broadDef.CategoryKey != specificDef.CategoryKey {
		return false
	}
	return traitsOverlap(broadDef.Traits.Environments, specificDef.Traits.Environments) ||
		traitsOverlap(broadDef.Traits.MovementFamilies, specificDef.Traits.MovementFamilies) ||
		traitsOverlap(broadDef.Traits.EquipmentFamilies, specificDef.Traits.EquipmentFamilies)
}

func traitsOverlap(a, b []string) bool {
	for _, left := range a {
		if left == "" || left == "generic" {
			continue
		}
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}

type sportConflictSnapshot struct {
	Sport      string
	SubType    string
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
}

func sportConflictSnapshotsConflict(platform, file sportConflictSnapshot) bool {
	if platform.Sport != file.Sport {
		return false
	}
	if platform.SubType != "" && platform.SubType != "generic" && file.SubType != "" && file.SubType != "generic" && platform.SubType != file.SubType {
		return true
	}
	return platform.IsElectric != file.IsElectric ||
		platform.IsIndoor != file.IsIndoor ||
		platform.IsVirtual != file.IsVirtual
}

func sportConflictMetadata(platform, file sportConflictSnapshot, fileFormat string) map[string]any {
	return map[string]any{
		"activity_id":          "",
		"file_sport":           file.Sport,
		"file_sub_type":        file.SubType,
		"file_is_electric":     file.IsElectric,
		"file_is_indoor":       file.IsIndoor,
		"file_is_virtual":      file.IsVirtual,
		"file_source":          fileFormat,
		"platform_sport":       platform.Sport,
		"platform_sub_type":    platform.SubType,
		"platform_is_electric": platform.IsElectric,
		"platform_is_indoor":   platform.IsIndoor,
		"platform_is_virtual":  platform.IsVirtual,
		"platform_source":      "strava",
	}
}

// createSportConflictQueueEntry creates a sport_conflict import queue entry.
// Deduplicates: skips if a pending entry already exists for this activity.
func createSportConflictQueueEntry(ctx context.Context, db *database.DB, userID, activityID string, platform, file sportConflictSnapshot, fileFormat string) error {
	exists, err := db.SportConflictQueueExists(ctx, userID, activityID)
	if err != nil || exists {
		return err
	}
	meta := sportConflictMetadata(platform, file, fileFormat)
	meta["activity_id"] = activityID

	_, err = db.CreateImportQueueItem(ctx, database.ImportQueueItem{
		UserID:    userID,
		QueueType: "sport_conflict",
		Metadata:  meta,
	})
	return err
}
