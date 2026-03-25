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

	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
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

// TODO(visibility): Parse DI_CONNECT/DI-Connect-Fitness/*_summarizedActivities.json to extract
// the "rule" field ("private"/"public") per activity and map it to data.Visibility.
// This requires matching summarizedActivities entries to FIT files by Garmin activity ID.

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

	// Cache open ZIP readers to avoid re-opening per file.
	zipReaders := make(map[string]*zip.ReadCloser)
	defer func() {
		for _, r := range zipReaders {
			r.Close()
		}
	}()

	current := 0
	for _, ref := range allFiles {
		current++
		if params.OnProgress != nil {
			params.OnProgress(current, total, "activities: "+ref.fileName)
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

		// Extract Garmin activity ID from filename: email_activityId_originalName.ext
		garminActivityID := extractActivityID(ref.fileName)

		// Detect format
		format := detectActivityFormat(fileBytes, ref.fileName)
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
				// Upload file — FileStore path preferred; legacy S3 as fallback.
				var msFileID string
				if params.FileStore != nil {
					rec, putErr := params.FileStore.Put(ctx, filestore.FileEntry{
						UserID:      params.UserID,
						FileType:    "activity",
						ContentType: "application/octet-stream",
						FileName:    ref.fileName,
						SizeBytes:   int64(len(fileBytes)),
						FileHash:    hashStr,
						Tags: map[string]string{
							"format": "fit",
							"source": "garmin",
						},
					}, bytes.NewReader(fileBytes))
					if putErr != nil {
						slog.Warn("garmin activities: FileStore.Put multisport failed", "file", ref.fileName, "error", putErr)
					} else {
						msFileID = rec.ID
					}
				} else if params.Storage != nil {
					s3Key := fmt.Sprintf("%s/%s.fit", params.UserID, hashStr)
					if uploadErr := params.Storage.Upload(ctx, s3Key, fileBytes, "application/octet-stream"); uploadErr != nil {
						slog.Warn("garmin activities: S3 upload multisport failed", "file", ref.fileName, "error", uploadErr)
					}
				}

				meta := importer.ImportMetadata{
					GarminActivityID: garminActivityID,
					SourcePlatform:   "garmin",
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
			GarminActivityID: garminActivityID,
			SourcePlatform:   "garmin",
			ConflictStrategy: params.Options.ConflictStrategy,
		}
		actResult, processErr := params.ImportService.ProcessActivity(ctx, params.UserID, data, fileBytes, meta)
		if processErr != nil {
			slog.Warn("garmin activities: ProcessActivity failed", "file", ref.fileName, "error", processErr)
			result.Errors = append(result.Errors, fmt.Sprintf("process %s: %s", ref.fileName, processErr))
			tracker.Error(ctx, processErr)
			continue
		}

		if actResult.Status == "enriched" || actResult.Status == "replaced" {
			result.Skipped++ // counts as "not newly created" for import stats
			tracker.Completed(ctx, actResult.ActivityID)
			continue
		}
		if actResult.Status == "skipped" {
			result.Skipped++
			tracker.Skipped(ctx, "duplicate", &actResult.ActivityID)
			continue
		}

		// New activity inserted — store file via FileStore (preferred) or legacy S3
		contentType := "application/octet-stream"
		if format == "gpx" || format == "tcx" {
			contentType = "application/xml"
		}
		var fileID string
		if params.FileStore != nil {
			rec, putErr := params.FileStore.Put(ctx, filestore.FileEntry{
				UserID:      params.UserID,
				FileType:    "activity",
				ContentType: contentType,
				FileName:    ref.fileName,
				SizeBytes:   int64(len(fileBytes)),
				FileHash:    hashStr,
				Tags: map[string]string{
					"format": format,
					"source": "garmin",
				},
			}, bytes.NewReader(fileBytes))
			if putErr != nil {
				slog.Warn("garmin activities: FileStore.Put failed", "file", ref.fileName, "error", putErr)
			} else {
				fileID = rec.ID
			}
		} else if params.Storage != nil {
			s3Key := fmt.Sprintf("%s/%s.%s", params.UserID, hashStr, format)
			if err := params.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
				slog.Warn("garmin activities: S3 upload failed", "file", ref.fileName, "error", err)
			}
		}
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
	// FIT: check header — ".FIT" signature is always at bytes 8–11,
	// regardless of whether the header is 12 or 14 bytes.
	if len(data) >= 12 && string(data[8:12]) == ".FIT" {
		return "fit"
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
