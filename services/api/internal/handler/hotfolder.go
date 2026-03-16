package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// UploadFromFile imports a single activity file from disk on behalf of userID.
// This is used by the hotfolder watcher.
func (h *Handler) UploadFromFile(ctx context.Context, userID, filePath string) error {
	start := time.Now()
	filename := filepath.Base(filePath)

	// 1. File size check via os.Stat before reading
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	fileSize := info.Size()
	if fileSize == 0 {
		return fmt.Errorf("empty file: %s", filename)
	}
	var maxSize int64
	if h.Config != nil {
		maxSize = h.Config.MaxUploadSize
	}
	if maxSize <= 0 {
		maxSize = 50 << 20 // default 50 MB
	}
	if fileSize > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", fileSize, maxSize)
	}

	// 2. Insert import_log entry (status=pending)
	logEntry := model.ImportLogEntry{
		UserID:         userID,
		SourcePlatform: "hotfolder",
		Filename:       filename,
		FileSizeBytes:  &fileSize,
		Status:         "pending",
	}
	logID, logErr := h.DB.InsertImportLog(ctx, logEntry)
	if logErr != nil {
		slog.Warn("hotfolder: failed to record import start", "file", filename, "error", logErr)
	}

	// 3. Update to processing
	if logID != "" {
		if err := h.DB.UpdateImportLogStatus(ctx, logID, "processing", nil, nil, nil); err != nil {
			slog.Warn("hotfolder: failed to update import status to processing", "id", logID, "error", err)
		}
	}

	// updateLog is a helper to finalize the import_log entry
	updateLog := func(status string, errMsg *string, activityID *string) {
		if logID == "" {
			return
		}
		elapsed := int(time.Since(start).Milliseconds())
		if err := h.DB.UpdateImportLogStatus(ctx, logID, status, errMsg, activityID, &elapsed); err != nil {
			slog.Warn("hotfolder: failed to finalize import log", "id", logID, "error", err)
		}
	}

	// 4. Read file
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		msg := fmt.Sprintf("read file: %s", err)
		updateLog("error", &msg, nil)
		return fmt.Errorf("read file: %w", err)
	}

	// 5. Detect format + decompress .gz if needed
	decompressed, format, err := detectFormatAndDecompress(fileBytes, filename, maxSize*10)
	if err != nil {
		msg := err.Error()
		updateLog("error", &msg, nil)
		return err
	}

	// 6. SHA256 hash (of decompressed bytes)
	hashArr := sha256.Sum256(decompressed)
	hashStr := hex.EncodeToString(hashArr[:])

	// 7. FIT file type routing — monitoring FITs go to health tables, not activity import
	if format == "fit" {
		ftype := garmin.ReadFITFileType(decompressed)
		if ftype == 68 || ftype == 44 {
			// Monitoring FIT — no S3 upload, parse directly to health tables
			monErr := garmin.ProcessMonitoringFIT(ctx, h.DB, decompressed, userID)
			if monErr != nil {
				msg := fmt.Sprintf("process monitoring FIT (type %d): %s", ftype, monErr)
				updateLog("error", &msg, nil)
				return monErr
			}
			// Update log with monitoring metadata
			if logID != "" {
				elapsed := int(time.Since(start).Milliseconds())
				if err := h.DB.UpdateImportLogStatus(ctx, logID, "completed", nil, nil, &elapsed); err != nil {
					slog.Warn("hotfolder: failed to finalize monitoring FIT log", "id", logID, "error", err)
				}
				meta, _ := json.Marshal(map[string]any{
					"fit_file_type": ftype,
					"data_type":     "monitoring",
				})
				if err := h.DB.UpdateImportLogMetadata(ctx, logID, meta); err != nil {
					slog.Warn("hotfolder: failed to set monitoring FIT metadata", "id", logID, "error", err)
				}
			}
			// Delete source file
			if removeErr := os.Remove(filePath); removeErr != nil {
				slog.Warn("hotfolder: failed to delete monitoring FIT after import", "path", filePath, "error", removeErr)
			} else {
				slog.Info("hotfolder: monitoring FIT deleted after import", "path", filePath)
			}
			slog.Info("hotfolder: monitoring FIT processed", "type", ftype, "file", filename)
			return nil
		}
		// ftype == 4 (activity) or unknown → continue to activity import
	}

	// 8. S3 upload (decompressed bytes, without .gz extension)
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}
	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, format)
	if err := h.Storage.Upload(ctx, s3Key, decompressed, contentType); err != nil {
		importErr := fmt.Errorf("S3 upload: %w", err)
		msg := importErr.Error()
		updateLog("error", &msg, nil)
		return importErr
	}

	// 9. Get user's conflict strategy
	var conflictStrategy importer.ConflictStrategy
	if importSettings, settErr := h.DB.GetImportSettings(ctx, userID); settErr == nil && importSettings != nil {
		conflictStrategy = importer.ConflictStrategy(importSettings.ConflictStrategy)
	}

	// 10. For FIT files: try multi-session parsing
	if format == "fit" {
		sessions, multiErr := fitimport.ParseMultiSessionFIT(decompressed, filename)
		if multiErr == nil && len(sessions) >= 2 {
			msResult, msErr := h.ImportService.ProcessMultisport(
				ctx, userID, sessions, decompressed, hashStr,
				s3Key, format, filename,
				importer.ImportMetadata{
					SourcePlatform:   "hotfolder",
					ConflictStrategy: conflictStrategy,
				},
			)
			if msErr != nil {
				importErr := fmt.Errorf("process multisport: %w", msErr)
				msg := importErr.Error()
				updateLog("error", &msg, nil)
				return importErr
			}
			slog.Info("hotfolder: multisport activity processed",
				"parent_id", msResult.ParentID,
				"children", len(msResult.Children),
				"file", filename,
			)
			updateLog("completed", nil, &msResult.ParentID)
			// Delete source file
			if removeErr := os.Remove(filePath); removeErr != nil {
				slog.Warn("hotfolder: failed to delete source file after multisport import", "path", filePath, "error", removeErr)
			} else {
				slog.Info("hotfolder: source file deleted after multisport import", "path", filePath)
			}
			return nil
		}
	}

	// 11. Single-session parsing
	var data *model.ActivityData
	switch format {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(decompressed), filename)
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(decompressed), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(decompressed), filename)
	}
	if err != nil {
		importErr := fmt.Errorf("parse %s: %w", format, err)
		msg := importErr.Error()
		updateLog("error", &msg, nil)
		return importErr
	}

	data.FileHash = hashStr

	// 12. Run duplicate detection + DB insert via ImportService
	result, err := h.ImportService.ProcessActivity(ctx, userID, data, decompressed, importer.ImportMetadata{
		SourcePlatform:   "hotfolder",
		ConflictStrategy: conflictStrategy,
	})
	if err != nil {
		importErr := fmt.Errorf("process activity: %w", err)
		msg := importErr.Error()
		updateLog("error", &msg, nil)
		slog.Error("hotfolder: ProcessActivity failed", "file", filename, "error", err)
		return importErr
	}

	slog.Info("hotfolder: activity processed",
		"id", result.ActivityID,
		"status", result.Status,
		"sport", data.SportType,
		"format", format,
		"file", filename,
	)

	// 13. PostProcess for created/enriched/replaced (NOT skipped)
	if result.Status == "created" || result.Status == "enriched" || result.Status == "replaced" {
		if err := h.ImportService.PostProcess(ctx, userID, result.ActivityID, data, decompressed, s3Key, format, filename); err != nil {
			slog.Warn("hotfolder: PostProcess failed", "id", result.ActivityID, "file", filename, "error", err)
		}
		// Delete source file after successful import
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("hotfolder: failed to delete source file after import", "path", filePath, "error", removeErr)
		} else {
			slog.Info("hotfolder: source file deleted after import", "path", filePath)
		}
		updateLog("completed", nil, &result.ActivityID)
	} else if result.Status == "skipped" {
		// Duplicate — keep file in place so user notices
		updateLog("skipped", nil, nil)
	}

	return nil
}
