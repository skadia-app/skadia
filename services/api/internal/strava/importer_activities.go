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

	hdr := csvHeader(rows[0])
	total := len(rows) - 1

	// L53: Warn when key columns are missing — likely a non-German locale export.
	for _, col := range []string{"Dateiname", "Aktivitäts-ID", "Name der Aktivität"} {
		if _, ok := hdr[col]; !ok {
			slog.Warn("strava activities: expected CSV column not found — export may use a different locale",
				"column", col)
		}
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
		isEbike := false
		if sportType == "e_biking" || sportType == "e_bike" {
			sportType = "cycling"
			isEbike = true
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

		// Power: intentionally not read from CSV — Strava often estimates power
		// even without a power meter. Values are taken from the activity file only.

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
			IsVirtual: isVirtual,
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
			detHash := sha256.Sum256([]byte(fmt.Sprintf("strava_csv|%d|%s|%s", stravaID, data.StartedAt.Format(time.RFC3339), data.SportType)))
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

	// IsEbike: device data is authoritative (FIT has sub-sport ID for e-bike detection)
	if parsed.IsEbike {
		base.IsEbike = true
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
	case "Virtuelles Radfahren", "VirtualRide":
		return "virtual_ride"
	case "Virtuelles Laufen", "VirtualRun":
		return "virtual_run"
	case "Virtuelles Rudern", "VirtualRow":
		return "virtual_row"
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
		return "stand_up_paddleboarding"
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
