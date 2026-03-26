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

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
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

	// 2. Insert import_log entry (status=pending) + transition to processing
	tracker := h.DB.NewImportTracker(ctx, userID, "hotfolder", filename, &fileSize, nil)
	tracker.Processing(ctx)

	// 3. Read file
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		tracker.Error(ctx, fmt.Errorf("read file: %w", err))
		return fmt.Errorf("read file: %w", err)
	}

	// 4. Detect format + decompress .gz if needed
	decompressed, format, err := detectFormatAndDecompress(fileBytes, filename, maxSize*10)
	if err != nil {
		tracker.Error(ctx, err)
		return err
	}

	// 5. SHA256 hash (of decompressed bytes)
	hashArr := sha256.Sum256(decompressed)
	hashStr := hex.EncodeToString(hashArr[:])
	tracker.SetFileHash(ctx, hashStr)

	// 6. FIT file type routing — monitoring FITs go to health tables, not activity import
	if format == "fit" {
		ftype := garmin.ReadFITFileType(decompressed)
		if ftype == 68 || ftype == 44 {
			// Monitoring FIT — no S3 upload, parse directly to health tables
			monErr := garmin.ProcessMonitoringFIT(ctx, h.DB, decompressed, userID)
			if monErr != nil {
				tracker.Error(ctx, fmt.Errorf("process monitoring FIT (type %d): %s", ftype, monErr))
				return monErr
			}
			// Update log with monitoring metadata
			tracker.Completed(ctx, "")
			meta, _ := json.Marshal(map[string]any{
				"fit_file_type": ftype,
				"data_type":     "monitoring",
			})
			tracker.SetMetadata(ctx, meta)
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

	// 7. Store file via ShadowFS.
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}
	var hotfolderFileID string
	{
		rec, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "activity",
			ContentType: contentType,
			FileName:    filename,
			SizeBytes:   int64(len(decompressed)),
			FileHash:    hashStr,
			Tags: map[string]string{
				"format": format,
				"source": "hotfolder",
			},
		}, bytes.NewReader(decompressed))
		if putErr != nil {
			importErr := fmt.Errorf("ShadowFS upload: %w", putErr)
			tracker.Error(ctx, importErr)
			return importErr
		}
		hotfolderFileID = rec.ID
	}

	// 8. Get user's conflict strategy
	var conflictStrategy importer.ConflictStrategy
	if importSettings, settErr := h.DB.GetImportSettings(ctx, userID); settErr == nil && importSettings != nil {
		conflictStrategy = importer.ConflictStrategy(importSettings.ConflictStrategy)
	}

	// 9. For FIT files: try multi-session parsing
	if format == "fit" {
		sessions, multiErr := fitimport.ParseMultiSessionFIT(decompressed, filename)
		if multiErr == nil && len(sessions) >= 2 {
			msResult, msErr := h.ImportService.ProcessMultisport(
				ctx, userID, sessions, decompressed, hashStr,
				hotfolderFileID, format, filename,
				importer.ImportMetadata{
					SourcePlatform:   "hotfolder",
					ConflictStrategy: conflictStrategy,
				},
			)
			if msErr != nil {
				importErr := fmt.Errorf("process multisport: %w", msErr)
				tracker.Error(ctx, importErr)
				return importErr
			}
			slog.Info("hotfolder: multisport activity processed",
				"parent_id", msResult.ParentID,
				"children", len(msResult.Children),
				"file", filename,
			)
			tracker.Completed(ctx, msResult.ParentID)
			// Delete source file
			if removeErr := os.Remove(filePath); removeErr != nil {
				slog.Warn("hotfolder: failed to delete source file after multisport import", "path", filePath, "error", removeErr)
			} else {
				slog.Info("hotfolder: source file deleted after multisport import", "path", filePath)
			}
			return nil
		}
	}

	// 10. Single-session parsing
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
		tracker.Error(ctx, importErr)
		return importErr
	}

	data.FileHash = hashStr

	// 11. Run duplicate detection + DB insert via ImportService
	result, err := h.ImportService.ProcessActivity(ctx, userID, data, decompressed, importer.ImportMetadata{
		SourcePlatform:   "hotfolder",
		ConflictStrategy: conflictStrategy,
	})
	if err != nil {
		importErr := fmt.Errorf("process activity: %w", err)
		tracker.Error(ctx, importErr)
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

	// 12. PostProcess for created/enriched/replaced (NOT skipped)
	if result.Status == "created" || result.Status == "enriched" || result.Status == "replaced" {
		if err := h.ImportService.PostProcess(ctx, userID, result.ActivityID, data, hotfolderFileID, format, filename); err != nil {
			slog.Warn("hotfolder: PostProcess failed", "id", result.ActivityID, "file", filename, "error", err)
		}
		// Delete source file after successful import
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("hotfolder: failed to delete source file after import", "path", filePath, "error", removeErr)
		} else {
			slog.Info("hotfolder: source file deleted after import", "path", filePath)
		}
		tracker.Completed(ctx, result.ActivityID)
	} else if result.Status == "skipped" {
		// Duplicate — keep file in place so user notices
		var existingID *string
		if result.ActivityID != "" {
			existingID = &result.ActivityID
		}
		tracker.Skipped(ctx, "duplicate", existingID)
	}

	return nil
}
