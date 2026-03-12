package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// UploadFromFile imports a single activity file from disk on behalf of userID.
// This is used by the hotfolder watcher.
func (h *Handler) UploadFromFile(ctx context.Context, userID, filePath string) error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	filename := filepath.Base(filePath)
	format := detectFormat(fileBytes, filename)
	if format == "" {
		return fmt.Errorf("unsupported format: %s", filename)
	}

	var data *model.ActivityData
	switch format {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filename)
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", format, err)
	}

	// SHA256 hash — must be set on data before ProcessActivity so InsertActivity
	// can persist it.
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])
	data.FileHash = hashStr

	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, format)
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}

	// Upload original file to S3 before inserting to DB so the key is always
	// resolvable even if a later step fails.
	if err := h.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
		return fmt.Errorf("S3 upload: %w", err)
	}

	// Run duplicate detection + DB insert via the shared ImportService.
	result, err := h.ImportService.ProcessActivity(ctx, userID, data, fileBytes, importer.ImportMetadata{
		SourcePlatform: "hotfolder",
	})
	if err != nil {
		slog.Error("hotfolder: ProcessActivity failed", "file", filename, "error", err)
		return fmt.Errorf("process activity: %w", err)
	}

	// PostProcess (streams, heatmap, events, shadow FS, segment matching …) only
	// runs for newly inserted activities; duplicates are handled inside
	// ProcessActivity.
	if result.Status == "created" {
		if err := h.ImportService.PostProcess(ctx, userID, result.ActivityID, data, fileBytes, s3Key, format, filename); err != nil {
			slog.Warn("hotfolder: PostProcess failed", "id", result.ActivityID, "file", filename, "error", err)
		}
	}

	slog.Info("hotfolder: activity processed",
		"id", result.ActivityID,
		"status", result.Status,
		"sport", data.SportType,
		"format", format,
		"file", filename,
	)
	return nil
}
