package garmin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// ActivitiesImporter imports FIT/GPX/TCX files from Garmin UploadedFiles ZIPs.
//
// Real Garmin export structure:
//
//	DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_0-_Part1.zip
//	  → files named: <email>_<activityId>_<originalFilename>.<ext>
//	    e.g. user@example.com_182805268653_7699291570.gpx
//
// The middle segment (second underscore-separated token after the email) is the Garmin activity ID.
// CSV files (weight logs etc.) are skipped.
type ActivitiesImporter struct{}

func (a *ActivitiesImporter) Name() string        { return "activities" }
func (a *ActivitiesImporter) Description() string { return "Aktivitäten (FIT/GPX/TCX aus UploadedFiles)" }
func (a *ActivitiesImporter) Priority() int       { return 30 }

func (a *ActivitiesImporter) Detect(src ExportSource) (bool, int, error) {
	zips, _ := src.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if len(zips) == 0 {
		return false, 0, nil
	}
	// Count activity files across all ZIPs
	count := 0
	for _, z := range zips {
		zr, err := zip.OpenReader(filepath.Join(src.BaseDir(), z))
		if err != nil {
			continue
		}
		for _, f := range zr.File {
			if isActivityFile(f.Name) {
				count++
			}
		}
		zr.Close()
	}
	return count > 0, count, nil
}

func (a *ActivitiesImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	zips, err := params.Source.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if err != nil || len(zips) == 0 {
		slog.Warn("garmin activities: no uploaded files ZIPs found")
		return result, nil
	}

	// Count total files for progress reporting
	total := 0
	type fileRef struct {
		zipPath  string
		fileName string
	}
	var allFiles []fileRef

	for _, z := range zips {
		zr, err := zip.OpenReader(filepath.Join(params.Source.BaseDir(), z))
		if err != nil {
			slog.Warn("garmin activities: cannot open zip", "zip", z, "error", err)
			continue
		}
		for _, f := range zr.File {
			if isActivityFile(f.Name) {
				allFiles = append(allFiles, fileRef{zipPath: z, fileName: f.Name})
				total++
			}
		}
		zr.Close()
	}

	slog.Info("garmin activities: found activity files", "count", total)

	current := 0
	for _, ref := range allFiles {
		current++
		if params.OnProgress != nil {
			params.OnProgress(current, total, "activities: "+ref.fileName)
		}

		fileBytes, err := readFileFromZip(filepath.Join(params.Source.BaseDir(), ref.zipPath), ref.fileName)
		if err != nil {
			slog.Warn("garmin activities: cannot read file from zip", "file", ref.fileName, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %s", ref.fileName, err))
			continue
		}

		// Extract Garmin activity ID from filename: email_activityId_originalName.ext
		garminActivityID := extractActivityID(ref.fileName)

		// Detect format
		format := detectActivityFormat(fileBytes, ref.fileName)
		if format == "" {
			result.Skipped++
			continue
		}

		// Parse activity file first (needed for temporal duplicate check)
		var data *model.ActivityData
		switch format {
		case "fit":
			data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), ref.fileName)
		case "gpx":
			data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), ref.fileName)
		case "tcx":
			data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), ref.fileName)
		}
		if err != nil {
			slog.Warn("garmin activities: parse failed", "file", ref.fileName, "format", format, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("parse %s: %s", ref.fileName, err))
			continue
		}

		// Date filter
		if params.Options.DateFrom != nil && data.StartedAt.Before(*params.Options.DateFrom) {
			result.Skipped++
			continue
		}
		if params.Options.DateTo != nil && data.StartedAt.After(*params.Options.DateTo) {
			result.Skipped++
			continue
		}

		// Duplicate check (hash + garmin_activity_id + temporal)
		match, dupErr := importer.CheckTier1(ctx, params.DB, params.UserID, fileBytes, data, garminActivityID, 0)
		if dupErr != nil {
			slog.Warn("garmin activities: duplicate check failed", "error", dupErr)
			result.Errors = append(result.Errors, fmt.Sprintf("dedup %s: %s", ref.fileName, dupErr))
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
					slog.Warn("garmin activities: cannot fetch existing activity for enrich", "id", match.ExistingActivityID, "error", getErr)
				}
				_, _ = importer.EnrichActivity(ctx, params.DB, match.ExistingActivityID, existing, data)
			}
			// Ensure garmin_activity_id is set on the existing activity
			if garminActivityID > 0 {
				_ = params.DB.SetActivityGarminID(ctx, match.ExistingActivityID, garminActivityID)
			}
			result.Skipped++
			continue
		}

		// Compute file hash for storage key
		hash := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(hash[:])
		data.FileHash = hashStr
		if data.Source == "" {
			data.Source = "Garmin"
		}

		// Upload original to Garage
		s3Key := fmt.Sprintf("%s/%s.%s", params.UserID, hashStr, format)
		contentType := "application/octet-stream"
		if format == "gpx" || format == "tcx" {
			contentType = "application/xml"
		}
		if params.Storage != nil {
			if err := params.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
				slog.Warn("garmin activities: S3 upload failed", "file", ref.fileName, "error", err)
				// Continue anyway – we still insert into DB
			}
		}

		// Insert activity into DB
		activity, err := params.DB.InsertActivity(ctx, params.UserID, data)
		if err != nil {
			slog.Warn("garmin activities: insert failed", "file", ref.fileName, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("insert %s: %s", ref.fileName, err))
			continue
		}

		// Set garmin_activity_id
		if garminActivityID > 0 {
			if err := params.DB.SetActivityGarminID(ctx, activity.ID, garminActivityID); err != nil {
				slog.Warn("garmin activities: set garmin_activity_id failed", "error", err)
			}
		}

		// Update file info
		metaKey := s3Key + ".meta"
		bucket := "skadia-originals"
		if params.Storage != nil {
			if err := params.DB.UpdateActivityFileInfo(ctx, activity.ID, bucket, s3Key, format, ref.fileName, metaKey); err != nil {
				slog.Warn("garmin activities: update file info failed", "error", err)
			}
			// Write .meta sidecar
			meta := sidecar.GenerateMeta(activity.ID, data)
			if metaBytes, err := meta.Marshal(); err == nil {
				if err := params.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
					slog.Warn("garmin activities: meta upload failed", "error", err)
				}
			}
		}

		// Insert streams, track, and heatmap
		if err := params.DB.InsertStreams(ctx, activity.ID, data.Streams); err != nil {
			slog.Warn("garmin activities: insert streams failed", "error", err)
		}
		if len(data.RRIntervals) > 0 {
			if err := params.DB.InsertRRIntervals(ctx, activity.ID, data.RRIntervals); err != nil {
				slog.Warn("garmin activities: insert rr intervals failed", "error", err)
			}
		}
		if err := params.DB.InsertTrack(ctx, activity.ID, data.TrackPoints); err != nil {
			slog.Warn("garmin activities: insert track failed", "error", err)
		}
		if len(data.TrackPoints) >= 2 {
			var subTypePtr *string
			if data.SubType != "" {
				subTypePtr = &data.SubType
			}
			if err := params.DB.InsertHeatmapLine(ctx, params.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt); err != nil {
				slog.Warn("garmin activities: insert heatmap line failed", "error", err)
			}
			interval, _ := params.DB.GetUserHeatmapInterval(ctx, params.UserID)
			if err := params.DB.InsertHeatmapPoints(ctx, params.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
				slog.Warn("garmin activities: insert heatmap points failed", "error", err)
			}
		}

		// Insert sensors, laps, splits, lengths, climb pro
		if err := params.DB.InsertSensors(ctx, activity.ID, data.Sensors); err != nil {
			slog.Warn("garmin activities: insert sensors failed", "error", err)
		}
		if err := params.DB.InsertLaps(ctx, activity.ID, data.Laps); err != nil {
			slog.Warn("garmin activities: insert laps failed", "error", err)
		}
		if err := params.DB.InsertSplits(ctx, activity.ID, data.Splits); err != nil {
			slog.Warn("garmin activities: insert splits failed", "error", err)
		}
		if err := params.DB.InsertSplitSummaries(ctx, activity.ID, data.SplitSummaries); err != nil {
			slog.Warn("garmin activities: insert split summaries failed", "error", err)
		}
		if err := params.DB.InsertLengths(ctx, activity.ID, data.Lengths); err != nil {
			slog.Warn("garmin activities: insert lengths failed", "error", err)
		}
		if err := params.DB.InsertClimbProEvents(ctx, activity.ID, data.ClimbProEvents); err != nil {
			slog.Warn("garmin activities: insert climb pro events failed", "error", err)
		}
		if len(data.TimeInZoneData) > 0 {
			if err := params.DB.SaveTimeInZones(ctx, activity.ID, data.TimeInZoneData); err != nil {
				slog.Warn("garmin activities: save time in zones failed", "error", err)
			}
		}

		// Stage 1 enrichment
		if data.HasGPS && data.StartLat != 0 {
			enrichment.RunStage1(ctx, params.DB, params.UserID, activity.ID, data)
		}

		result.Imported++
	}

	slog.Info("garmin activities import done", "imported", result.Imported, "skipped", result.Skipped, "errors", len(result.Errors))
	return result, nil
}

// isActivityFile returns true for FIT, GPX, TCX files (not CSV etc.)
func isActivityFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".fit") ||
		strings.HasSuffix(lower, ".gpx") ||
		strings.HasSuffix(lower, ".tcx")
}

// ExtractActivityID parses the Garmin activity ID from the filename.
// Format: <email>_<activityId>_<rest>.<ext>
// The email may contain underscore-like segments (it uses @ and .) so we split smartly.
// Exported so that scan handlers in the handler package can reuse the same logic.
func ExtractActivityID(filename string) int64 {
	return extractActivityID(filename)
}

// extractActivityID is the internal implementation.
func extractActivityID(filename string) int64 {
	// Strip directory component
	base := filepath.Base(filename)
	// Strip extension
	noExt := strings.TrimSuffix(base, filepath.Ext(base))
	// Split by underscore; the activity ID is the numeric token after the email
	// The email ends at the "@" sign – find position of first "_" after "@"
	atIdx := strings.Index(noExt, "@")
	if atIdx < 0 {
		// No "@" – try simple split: first segment is email/prefix, second is ID
		parts := strings.SplitN(noExt, "_", 3)
		if len(parts) >= 2 {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			return id
		}
		return 0
	}
	// Find the first "_" after the "@"
	afterAt := noExt[atIdx:]
	underIdx := strings.Index(afterAt, "_")
	if underIdx < 0 {
		return 0
	}
	rest := afterAt[underIdx+1:]
	// rest is now "<activityId>_<originalFilename>"
	parts := strings.SplitN(rest, "_", 2)
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	return id
}

// readFileFromZip extracts a single named file from a ZIP archive into memory.
func readFileFromZip(zipPath, fileName string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == fileName {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open file %s in zip: %w", fileName, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("read file %s in zip: %w", fileName, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in zip %s", fileName, zipPath)
}

// detectActivityFormat detects the file format from content + extension.
func detectActivityFormat(data []byte, filename string) string {
	// FIT: check header
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
	// Fallback: extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".fit":
		return "fit"
	case ".gpx":
		return "gpx"
	case ".tcx":
		return "tcx"
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
