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
	"maps"
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
		sportType, subType := mapStravaSportType(sportStr), ""

		// E-Bike normalization
		isElectric := false
		switch sportType {
		case "e_biking", "e_bike":
			sportType = "cycling"
			isElectric = true
		case "e_mountain_biking":
			sportType = "mountain_biking"
			isElectric = true
		}

		isVirtual := false
		switch sportType {
		case "virtual_ride":
			sportType = "cycling"
			isVirtual = true
		case "virtual_run":
			sportType = "running"
			isVirtual = true
		case "virtual_row":
			sportType = "rowing"
			isVirtual = true
		}

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
		isIndoor := sport.IsIndoor(sportType, subType) || filename == ""

		// Build base ActivityData (will be enriched if GPS file exists)
		data := &model.ActivityData{
			Source:     "Strava",
			SportType:  sportType,
			SubType:    subType,
			IsElectric: isElectric,
			IsVirtual:  isVirtual,
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
							Tags: map[string]string{
								"format": "fit",
								"source": "strava",
							},
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
					}
					// csvGroup == "generic" && fileGroup != "generic" → file already wins (from merge)
					// csvGroup != "generic" && fileGroup == "generic" → CSV already wins (from merge)
					// Same group → merge result stands

					if conflict {
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

		// CSV-only activity (no file in Strava export, e.g. manually created in Strava).
		// Store the CSV row as a JSON source file so every activity has a stored original.
		if len(fileBytes) == 0 {
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
			// Deterministic hash (not from JSON bytes which contain CreatedAt)
			detHash := sha256.Sum256(fmt.Appendf(nil, "strava_csv|%d|%s|%s", stravaID, data.StartedAt.Format(time.RFC3339), data.SportType))
			hashStr = hex.EncodeToString(detHash[:])
			jsonBytes, jsonErr := csvJSON.Marshal()
			if jsonErr == nil {
				data.FileHash = hashStr
				data.FileFormat = "strava_csv"
				format = "strava_csv"
				if params.ShadowFS != nil {
					rec, putErr := params.ShadowFS.Put(ctx, shadowfs.FileEntry{
						UserID:      params.UserID,
						FileType:    "activity",
						ContentType: "application/json",
						FileName:    fmt.Sprintf("strava_%d.json", stravaID),
						SizeBytes:   int64(len(jsonBytes)),
						FileHash:    hashStr,
						Tags: map[string]string{
							"format": "strava_csv",
							"source": "strava",
						},
					}, bytes.NewReader(jsonBytes))
					if putErr != nil {
						slog.Warn("strava activities: ShadowFS.Put for CSV-only failed", "error", putErr)
					} else {
						fileID = rec.ID
					}
				}
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
					Tags: map[string]string{
						"format": format,
						"source": "strava",
					},
				}, bytes.NewReader(fileBytes))
				if putErr != nil {
					slog.Warn("strava activities: ShadowFS.Put failed", "file", filename, "error", putErr)
				} else {
					fileID = rec.ID
				}
			}
		}

		// New activity inserted: run PostProcess (streams, track, heatmap, Stage 1, NATS…)
		if err := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, fileID, format, filename); err != nil {
			slog.Warn("strava activities: PostProcess failed", "id", actResult.ActivityID, "error", err)
		}
		tracker.Completed(ctx, actResult.ActivityID)

		// Apply Strava-specific enrichment: activity name + weather from CSV
		enrichFields := buildStravaEnrichFields(activityName, row, hdr)
		if len(enrichFields) > 0 {
			if err := params.DB.UpdateEnrichmentFields(ctx, actResult.ActivityID, enrichFields); err != nil {
				slog.Warn("strava activities: update enrichment fields failed", "error", err)
			}
		}

		// Create sport conflict queue entry if detected
		if data.SportConflictPending {
			if qErr := createSportConflictQueueEntry(ctx, params.DB, params.UserID,
				actResult.ActivityID, sportType, fileSportType, format); qErr != nil {
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

// buildStravaEnrichFields builds the enrichment field map for Strava-specific metadata:
// the activity title and weather data from the CSV row.
func buildStravaEnrichFields(activityName string, row []string, hdr map[string]int) map[string]any {
	fields := make(map[string]any)
	if activityName != "" {
		fields["title"] = activityName
	}
	maps.Copy(fields, extractWeatherFields(row, hdr))
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
	//   types into broad categories (e.g., mountain_biking → "Radfahren", bouldering → "Klettern")
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

// mapStravaSportType maps Strava sport type strings to Skadia canonical sport types.
//
// English names come from the Strava API SportType enum (verified, PascalCase in API,
// space-separated in CSV export). German names are ONLY included when verified from
// actual German Strava GDPR exports — no guesses.
//
// Sources:
//   - API: https://developers.strava.com/docs/reference/ (50 SportTypes as of 2025)
//   - New (Feb 2026): Padel, Basketball, Volleyball, Cricket, Dance
//   - German: verified from strava_export_deutsch/ and original codebase
func mapStravaSportType(stravaSport string) string {
	s := strings.TrimSpace(stravaSport)
	switch s {
	// ── Cycling ─────────────────────────────────────────────────────────
	// EN: Ride | DE verified: Radfahrt, Radfahren
	case "Ride", "Radfahrt", "Radfahren":
		return "cycling"
	// EN: E-Bike Ride | DE verified: E-Bike-Radfahrt, E-Bike
	case "E-Bike Ride", "E-Bike-Radfahrt", "E-Bike":
		return "e_biking" // normalized to cycling + is_electric downstream
	// EN: Mountain Bike Ride | DE verified: Mountainbiking, Mountainbike-Fahrt
	case "Mountain Bike Ride", "Mountainbiking", "Mountainbike-Fahrt":
		return "mountain_biking"
	// EN: E-Mountain Bike Ride (API: EMountainBikeRide)
	case "E-Mountain Bike Ride":
		return "e_mountain_biking" // normalized to mountain_biking + is_electric downstream
	// EN: Gravel Ride | DE verified: Gravelbike, Gravel-Radfahrt
	case "Gravel Ride", "Gravelbike", "Gravel-Radfahrt":
		return "gravel_cycling"
	// EN: Virtual Ride | DE verified: Virtuelles Radfahren
	case "Virtual Ride", "VirtualRide", "Virtuelles Radfahren":
		return "virtual_ride"
	// DE verified: Indoor-Radfahren, Spinning
	case "Indoor Cycling", "Indoor-Radfahren", "Spinning":
		return "indoor_cycling"
	// EN: Handcycle
	case "Handcycle":
		return "cycling"
	// EN: Velomobile
	case "Velomobile":
		return "cycling"

	// ── Running ─────────────────────────────────────────────────────────
	// EN: Run | DE verified: Lauf, Laufen
	case "Run", "Lauf", "Laufen":
		return "running"
	// EN: Trail Run | DE verified: Trailrunning, Trailrun
	case "Trail Run", "Trailrunning", "Trailrun":
		return "trail_running"
	// DE verified: Treadmill-Lauf, Laufband
	case "Treadmill", "Treadmill-Lauf", "Laufband":
		return "treadmill_running"
	// EN: Virtual Run | DE verified: Virtuelles Laufen
	case "Virtual Run", "VirtualRun", "Virtuelles Laufen":
		return "virtual_run"

	// ── Hiking / Walking ────────────────────────────────────────────────
	// EN: Hike | DE verified: Wandern, Wanderung
	case "Hike", "Wandern", "Wanderung":
		return "hiking"
	// EN: Walk | DE verified: Spaziergang
	case "Walk", "Spaziergang":
		return "walking"

	// ── Swimming ────────────────────────────────────────────────────────
	// EN: Swim | DE verified: Schwimmen
	case "Swim", "Schwimmen":
		return "swimming"
	// DE verified: Freiwasserschwimmen
	case "Open Water Swim", "Freiwasserschwimmen":
		return "open_water_swimming"

	// ── Water sports ────────────────────────────────────────────────────
	// EN: Stand Up Paddling | DE verified: Stand-up-Paddeln, SUP
	case "Stand Up Paddling", "Stand-up-Paddeln", "SUP":
		return "stand_up_paddleboarding"
	// EN: Kayaking | DE verified: Kajak fahren, Kajak
	case "Kayaking", "Kajak fahren", "Kajak":
		return "kayaking"
	// EN: Rowing | DE verified: Rudern
	case "Rowing", "Rudern":
		return "rowing"
	// EN: Canoeing | DE verified: Kanufahren
	case "Canoeing", "Kanufahren":
		return "kayaking"
	// EN: Virtual Row | DE verified: Virtuelles Rudern
	case "Virtual Row", "VirtualRow", "Virtuelles Rudern":
		return "virtual_row"
	// EN: Sail
	case "Sail":
		return "sailing"
	// EN: Surfing
	case "Surfing":
		return "surfing"
	// EN: Kitesurf
	case "Kitesurf":
		return "kitesurfing"
	// EN: Windsurf
	case "Windsurf":
		return "windsurfing"

	// ── Winter sports ───────────────────────────────────────────────────
	// EN: Alpine Ski | DE verified: Skifahren, Ski alpin
	case "Alpine Ski", "Skifahren", "Ski alpin":
		return "alpine_skiing"
	// EN: Nordic Ski | DE verified: Langlauf, Skilanglauf
	case "Nordic Ski", "Langlauf", "Skilanglauf":
		return "cross_country_skiing"
	// EN: Backcountry Ski
	case "Backcountry Ski":
		return "backcountry_skiing"
	// EN: Snowboard | DE verified: Snowboarden
	case "Snowboard", "Snowboarden":
		return "snowboarding"
	// EN: Snowshoe | DE verified: Schneeschuhwandern
	case "Snowshoe", "Schneeschuhwandern":
		return "snowshoeing"
	// EN: Ice Skate
	case "Ice Skate":
		return "ice_skating"
	// EN: Roller Ski
	case "Roller Ski":
		return "cross_country_skiing" // closest taxonomy match

	// ── Fitness / Gym ───────────────────────────────────────────────────
	// EN: Weight Training | DE verified: Krafttraining, Gewichtheben
	case "Weight Training", "Krafttraining", "Gewichtheben":
		return "strength_training"
	// EN: Yoga | identical in DE
	case "Yoga":
		return "yoga"
	// EN: Pilates | identical in DE
	case "Pilates":
		return "pilates"
	// EN: Crossfit | identical in DE
	case "Crossfit":
		return "training"
	// EN: HIIT (HighIntensityIntervalTraining) | identical in DE
	case "HIIT":
		return "hiit"
	// EN: Workout
	case "Workout":
		return "training"
	// EN: Elliptical
	case "Elliptical":
		return "fitness_equipment"
	// EN: Stair-Stepper (API: StairStepper)
	case "Stair-Stepper":
		return "fitness_equipment"
	// EN: Dance (added Feb 2026)
	case "Dance":
		return "dance"

	// ── Climbing ────────────────────────────────────────────────────────
	// EN: Rock Climbing | DE verified: Klettern, Felsenklettern, Bouldern
	case "Rock Climbing", "Klettern", "Felsenklettern", "Bouldern":
		return "rock_climbing"

	// ── Racquet / Ball / Team sports ────────────────────────────────────
	// EN: Tennis | identical in DE
	case "Tennis":
		return "tennis"
	// EN: Badminton | identical in DE
	case "Badminton":
		return "badminton"
	// EN: Table Tennis
	case "Table Tennis":
		return "table_tennis"
	// EN: Squash | identical in DE
	case "Squash":
		return "squash"
	// EN: Pickleball | identical in DE
	case "Pickleball":
		return "pickleball"
	// EN: Racquetball
	case "Racquetball":
		return "racket"
	// EN: Padel (added Feb 2026) | identical in DE
	case "Padel":
		return "racket"
	// EN: Football (Soccer) — Strava's CSV name for Soccer
	case "Football (Soccer)", "Soccer":
		return "soccer"
	// EN: Golf | identical in DE
	case "Golf":
		return "golf"
	// EN: Basketball (added Feb 2026) | identical in DE
	case "Basketball":
		return "basketball"
	// EN: Volleyball (added Feb 2026) | identical in DE
	case "Volleyball":
		return "volleyball"
	// EN: Cricket (added Feb 2026) | identical in DE
	case "Cricket":
		return "cricket"

	// ── Other ───────────────────────────────────────────────────────────
	// EN: Inline Skate
	case "Inline Skate":
		return "inline_skating"
	// EN: Skateboard
	case "Skateboard":
		return "inline_skating" // closest taxonomy match
	// EN: Wheelchair
	case "Wheelchair":
		return "generic"
	case "Triathlon":
		return "triathlon"
	case "Multisport":
		return "multi_sport"
	// DE verified: Sonstiges, Andere
	case "Sonstiges", "Andere":
		return "generic"

	default:
		slog.Debug("strava: unmapped sport type", "strava_type", s)
		return "generic"
	}
}


// extractWeatherFields previously populated flat weather_* columns from Strava CSV.
// TODO(weather-improvements Task 6): migrate to InsertWeatherReadings via activity_weather table.
// Flat columns have been removed from activities table (migration 000167).
func extractWeatherFields(row []string, hdr map[string]int) map[string]any {
	// Strava weather data import is temporarily disabled pending migration to
	// the new activity_weather table. Return empty map so no dead columns are written.
	_, _ = row, hdr
	return map[string]any{}
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

// sportRefinements maps broad Strava CSV sport types to the set of more specific
// device-recorded types that should take precedence.
var sportRefinements = map[string]map[string]bool{
	"cycling": {
		"mountain_biking": true,
		"gravel_cycling":  true,
		"indoor_cycling":  true,
	},
	"running": {
		"trail_running":     true,
		"treadmill_running": true,
	},
	"swimming": {
		"open_water_swimming": true,
	},
}

// isSportRefinement returns true when 'specific' is a known more-specific variant
// of 'broad' within the same sport family.
func isSportRefinement(broad, specific string) bool {
	if broad == specific {
		return false
	}
	refinements, ok := sportRefinements[broad]
	if !ok {
		return false
	}
	return refinements[specific]
}

// createSportConflictQueueEntry creates a sport_conflict import queue entry.
// Deduplicates: skips if a pending entry already exists for this activity.
func createSportConflictQueueEntry(ctx context.Context, db *database.DB, userID, activityID, csvSport, fileSport, fileFormat string) error {
	exists, err := db.SportConflictQueueExists(ctx, userID, activityID)
	if err != nil || exists {
		return err
	}

	_, err = db.CreateImportQueueItem(ctx, database.ImportQueueItem{
		UserID:    userID,
		QueueType: "sport_conflict",
		Metadata: map[string]any{
			"activity_id":     activityID,
			"file_sport":      fileSport,
			"file_source":     fileFormat,
			"platform_sport":  csvSport,
			"platform_source": "strava",
		},
	})
	return err
}
