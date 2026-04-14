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
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
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

func (a *ActivitiesImporter) Name() string { return "activities" }
func (a *ActivitiesImporter) Description() string {
	return "Aktivitäten (FIT/GPX/TCX aus UploadedFiles)"
}
func (a *ActivitiesImporter) Priority() int { return 30 }

func (a *ActivitiesImporter) Detect(src ExportSource) (bool, int, error) {
	zips, _ := src.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if len(zips) == 0 {
		return false, 0, nil
	}
	// Count only genuine activity files: GPX/TCX always qualify; for FIT files
	// we read the first 256 bytes to check the file_id type (activity = 4).
	count := 0
	for _, z := range zips {
		zr, err := zip.OpenReader(filepath.Join(src.BaseDir(), z))
		if err != nil {
			continue
		}
		for _, f := range zr.File {
			if !isActivityFile(f.Name) {
				continue
			}
			lower := strings.ToLower(f.Name)
			if strings.HasSuffix(lower, ".fit") {
				if readFITFileTypeFromZipEntry(f) != fitFileTypeActivity {
					continue // skip monitoring / HRV / other non-activity FITs
				}
			}
			count++
		}
		zr.Close()
	}
	return count > 0, count, nil
}

// TODO(visibility): Extend loadActivityIDMap to also extract the "rule" field ("private"/"public")
// per activity and map it to data.Visibility during import.

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

	// Build a map from start time → real Garmin Connect activity ID.
	// UploadedFiles filenames contain upload file IDs, not the activity IDs shown
	// in Garmin Connect URLs — the summarizedActivities JSON has the real IDs.
	activityIDMap := loadActivityIDMap(params.Source)

	// Cache open ZIP readers to avoid re-opening per file.
	zipReaders := make(map[string]*zip.ReadCloser)
	defer func() {
		for _, r := range zipReaders {
			r.Close()
		}
	}()

	current := 0
	for _, ref := range allFiles {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		current++
		if params.OnProgress != nil {
			params.OnProgress(current, total, fmt.Sprintf("activities: %d/%d %s", current, total, ref.fileName))
		}

		var batchIDPtr *string
		if params.JobID != "" {
			batchIDPtr = &params.JobID
		}
		tracker := params.DB.NewImportTracker(ctx, params.UserID, "garmin", ref.fileName, nil, batchIDPtr)
		tracker.Processing(ctx)

		fileBytes, err := readFileFromZipCached(zipReaders, filepath.Join(params.Source.BaseDir(), ref.zipPath), ref.fileName)
		if err != nil {
			slog.Warn("garmin activities: cannot read file from zip", "file", ref.fileName, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %s", ref.fileName, err))
			tracker.Error(ctx, err)
			continue
		}

		// Detect format
		format := importutil.DetectFormat(fileBytes, ref.fileName)
		if format == "" {
			result.Skipped++
			tracker.Skipped(ctx, "format_unsupported", nil)
			continue
		}

		// For FIT files: skip non-activity types (monitoring, HRV, steps etc.) silently.
		// These are processed by importMonitoringFITs; trying to parse them as activities
		// would always fail with "no session message".
		if format == "fit" && readFITFileType(fileBytes) != fitFileTypeActivity {
			result.Skipped++
			tracker.Skipped(ctx, "not_activity_type", nil)
			continue
		}

		// For FIT files: try multi-session parsing first (e.g. triathlons)
		if format == "fit" && params.ImportService != nil {
			sessions, parseErr := fitimport.ParseMultiSessionFIT(fileBytes, ref.fileName)
			if parseErr == nil && len(sessions) > 1 {
				// Multisport detected — route through ProcessMultisport
				hashArr := sha256.Sum256(fileBytes)
				hashStr := hex.EncodeToString(hashArr[:])
				tracker.SetFileHash(ctx, hashStr)
				// Upload file via ShadowFS.
				var msFileID string
				if params.ShadowFS != nil {
					rec, putErr := params.ShadowFS.Put(ctx, shadowfs.FileEntry{
						UserID:      params.UserID,
						FileType:    "activity",
						ContentType: "application/octet-stream",
						FileName:    filepath.Base(ref.fileName), // L47: sanitize
						SizeBytes:   int64(len(fileBytes)),
						FileHash:    hashStr,
						Tags: map[string]string{
							"format": "fit",
							"source": "garmin",
						},
					}, bytes.NewReader(fileBytes))
					if putErr != nil {
						slog.Error("garmin activities: ShadowFS.Put multisport failed — skipping activity", "file", ref.fileName, "error", putErr)
						result.Errors = append(result.Errors, fmt.Sprintf("multisport upload %s: %s", ref.fileName, putErr))
						tracker.Error(ctx, putErr)
						continue
					}
					msFileID = rec.ID
				}

				meta := importer.ImportMetadata{
					GarminActivityID: resolveGarminActivityID(activityIDMap, sessions[0].StartedAt),
					SourcePlatform:   "garmin",
					ConflictStrategy: params.Options.ConflictStrategy,
				}
				msResult, msErr := params.ImportService.ProcessMultisport(ctx, params.UserID, sessions, fileBytes, hashStr, msFileID, "fit", ref.fileName, meta)
				if msErr != nil {
					slog.Warn("garmin activities: multisport import failed", "file", ref.fileName, "sessions", len(sessions), "error", msErr)
					result.Errors = append(result.Errors, fmt.Sprintf("multisport %s: %s", ref.fileName, msErr))
					tracker.Error(ctx, msErr)
				} else {
					for _, child := range msResult.Children {
						if child.Status == "created" {
							result.Imported++
						} else {
							result.Skipped++
						}
					}
					slog.Info("garmin activities: multisport imported", "file", ref.fileName, "sessions", len(sessions), "parent", msResult.ParentID)
					tracker.Completed(ctx, "")
				}
				continue
			}
			// Single-session or parse error — fall through to normal parsing below
		}

		// Parse activity file (Garmin-specific: FIT/GPX/TCX)
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
			tracker.Error(ctx, err)
			continue
		}

		// Date filter
		if params.Options.DateFrom != nil && data.StartedAt.Before(*params.Options.DateFrom) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}
		if params.Options.DateTo != nil && data.StartedAt.After(*params.Options.DateTo) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}

		if data.Source == "" {
			data.Source = "Garmin"
		}

		// Compute file hash and set it on data before ProcessActivity (mirrors upload handler)
		hashArr := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(hashArr[:])
		data.FileHash = hashStr
		tracker.SetFileHash(ctx, hashStr)

		meta := importer.ImportMetadata{
			GarminActivityID: resolveGarminActivityID(activityIDMap, data.StartedAt),
			SourcePlatform:   "garmin",
			ConflictStrategy: params.Options.ConflictStrategy,
		}
		actResult, processErr := params.ImportService.ProcessActivity(ctx, params.UserID, data, fileBytes, meta)
		if processErr != nil {
			slog.Error("garmin activities: ProcessActivity failed", "file", ref.fileName, "error", processErr)
			result.Errors = append(result.Errors, fmt.Sprintf("process %s: %s", ref.fileName, processErr))
			tracker.Error(ctx, processErr)
			continue
		}

		if actResult.Status == "skipped" {
			result.Skipped++
			tracker.Skipped(ctx, "duplicate", &actResult.ActivityID)
			continue
		}

		// Upload file only for newly created activities (mirrors Strava importer behavior).
		// For enriched/replaced the file already exists in storage — uploading again creates
		// duplicate ShadowFS records (M20).
		if actResult.Status != "created" {
			if postErr := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, "", format, ref.fileName); postErr != nil {
				slog.Warn("garmin activities: PostProcess failed (enriched/replaced)", "file", ref.fileName, "error", postErr)
			}
			result.Skipped++
			tracker.Completed(ctx, actResult.ActivityID)
			continue
		}

		contentType := "application/octet-stream"
		if format == "gpx" || format == "tcx" {
			contentType = "application/xml"
		}
		// L34: Apply filepath.Base to prevent directory components in stored file names.
		safeFileName := filepath.Base(ref.fileName)
		var fileID string
		if params.ShadowFS != nil {
			rec, putErr := params.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      params.UserID,
				FileType:    "activity",
				ContentType: contentType,
				FileName:    safeFileName,
				SizeBytes:   int64(len(fileBytes)),
				FileHash:    hashStr,
				Tags: map[string]string{
					"format": format,
					"source": "garmin",
				},
			}, bytes.NewReader(fileBytes))
			if putErr != nil {
				slog.Warn("garmin activities: ShadowFS.Put failed", "file", ref.fileName, "error", putErr)
			} else {
				fileID = rec.ID
			}
		}

		// New activity inserted — run PostProcess (streams, track, heatmap, Stage 1, NATS…)
		if postErr := params.ImportService.PostProcess(ctx, params.UserID, actResult.ActivityID, data, fileID, format, ref.fileName); postErr != nil {
			slog.Warn("garmin activities: PostProcess failed", "file", ref.fileName, "error", postErr)
			// Non-fatal: activity is already stored
		}
		tracker.Completed(ctx, actResult.ActivityID)
		result.Imported++
	}

	slog.Info("garmin activities import done", "imported", result.Imported, "skipped", result.Skipped, "errors", len(result.Errors))
	return result, nil
}

// loadActivityIDMap reads all *_summarizedActivities.json files from the export and builds
// a map from activity start time (Unix seconds) to the real Garmin Connect activity ID.
//
// Background: UploadedFiles ZIP entries are named "<email>_<uploadFileID>.fit" where the
// upload file ID is NOT the Garmin Connect activity ID shown in connect.garmin.com URLs.
// The real activity IDs are in summarizedActivities.json, keyed by start time.
//
// Timestamps that appear more than once map to 0 (ambiguous; caller falls back to filename ID).
func loadActivityIDMap(src ExportSource) map[int64]int64 {
	files, err := src.FindFiles("DI_CONNECT/DI-Connect-Fitness/*_summarizedActivities.json")
	if err != nil || len(files) == 0 {
		return nil
	}
	type entry struct {
		ActivityID   int64   `json:"activityId"`
		StartTimeGmt float64 `json:"startTimeGmt"` // Unix milliseconds
	}
	type fileRoot []struct {
		Activities []entry `json:"summarizedActivitiesExport"`
	}
	m := make(map[int64]int64)
	seen := make(map[int64]bool)
	for _, f := range files {
		var root fileRoot
		if err := src.ReadJSON(f, &root); err != nil {
			slog.Warn("garmin activities: cannot read summarizedActivities", "file", f, "err", err)
			continue
		}
		for _, section := range root {
			for _, a := range section.Activities {
				if a.ActivityID <= 0 || a.StartTimeGmt <= 0 {
					continue
				}
				ts := int64(a.StartTimeGmt / 1000)
				if seen[ts] {
					m[ts] = 0 // ambiguous — two activities with same start second
					continue
				}
				seen[ts] = true
				m[ts] = a.ActivityID
			}
		}
	}
	slog.Info("garmin activities: loaded summarizedActivities ID map", "entries", len(m))
	return m
}

// resolveGarminActivityID returns the real Garmin Connect activity ID for an activity
// with the given start time. Returns 0 if the map is nil, has no entry for this
// timestamp, or has an ambiguous entry — in all those cases backfillPlatformIDs
// will skip the write (meta.GarminActivityID <= 0), leaving any existing correct
// value untouched. The upload file ID from the filename is never stored directly:
// it is not a Garmin Connect activity ID and would produce broken deep-links.
func resolveGarminActivityID(activityIDMap map[int64]int64, startTime time.Time) int64 {
	if activityIDMap == nil {
		return 0
	}
	id := activityIDMap[startTime.Unix()]
	// id == 0 means either not found or ambiguous — both cases: skip write.
	return id
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
	_, after, ok := strings.Cut(afterAt, "_")
	if !ok {
		return 0
	}
	rest := after
	// rest is now "<activityId>_<originalFilename>"
	parts := strings.SplitN(rest, "_", 2)
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	return id
}

// readFileFromZipCached extracts a single named file using a cached ZIP reader.
// The reader is opened on first access and reused for subsequent files from the same ZIP.
func readFileFromZipCached(cache map[string]*zip.ReadCloser, zipPath, fileName string) ([]byte, error) {
	zr, ok := cache[zipPath]
	if !ok {
		var err error
		zr, err = zip.OpenReader(zipPath)
		if err != nil {
			return nil, fmt.Errorf("open zip %s: %w", zipPath, err)
		}
		cache[zipPath] = zr
	}

	// M27: Cap per-entry reads to 500 MB to prevent zip-bomb OOM.
	const maxZipEntryBytes = 500 << 20

	for _, f := range zr.File {
		if f.Name == fileName {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open file %s in zip: %w", fileName, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(io.LimitReader(rc, maxZipEntryBytes+1))
			if err != nil {
				return nil, fmt.Errorf("read file %s in zip: %w", fileName, err)
			}
			if len(data) > maxZipEntryBytes {
				return nil, fmt.Errorf("file %s in zip exceeds %d byte limit", fileName, maxZipEntryBytes)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in zip %s", fileName, zipPath)
}

