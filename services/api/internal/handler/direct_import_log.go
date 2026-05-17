package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

type directImportStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

func directMediaImportType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "video/"):
		return model.ImportTypeVideo
	case strings.HasPrefix(contentType, "image/"):
		return model.ImportTypePhoto
	default:
		return model.ImportTypeDocument
	}
}

func directUploadTracker(ctx context.Context, db *database.DB, userID, filename string, size int64, importType, fileHash string, metadata map[string]any) *database.ImportTracker {
	tracker := db.NewImportTracker(ctx, userID, "upload", filename, &size, nil, importType)
	tracker.SetFileHash(ctx, fileHash)
	setDirectImportMetadata(ctx, tracker, metadata)
	tracker.Processing(ctx)
	return tracker
}

func setDirectImportMetadata(ctx context.Context, tracker *database.ImportTracker, metadata map[string]any) {
	if tracker == nil || metadata == nil {
		return
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		slog.Warn("direct import metadata marshal failed", "error", err)
		return
	}
	tracker.SetMetadata(ctx, data)
}

func directDocumentImportMetadata(targetType, targetID, fileID, parentKey, parentID, docType, contentType string, processing bool) map[string]any {
	steps := []directImportStep{{Key: "upload", Label: "Upload", Status: "completed"}}
	if processing {
		steps = append(steps,
			directImportStep{Key: "text_extraction", Label: "Texterkennung", Status: "pending"},
			directImportStep{Key: "markdown", Label: "Markdown", Status: "pending"},
		)
	}
	metadata := map[string]any{
		"target_type":  targetType,
		"target_id":    targetID,
		"doc_type":     docType,
		"content_type": contentType,
		"steps":        steps,
	}
	if fileID != "" {
		metadata["file_id"] = fileID
	}
	if parentKey != "" && parentID != "" {
		metadata[parentKey] = parentID
	}
	return metadata
}

func markDirectDocumentStep(metadata map[string]any, key, status string) {
	steps, ok := metadata["steps"].([]directImportStep)
	if !ok {
		return
	}
	for i := range steps {
		if steps[i].Key == key {
			steps[i].Status = status
			break
		}
	}
	metadata["steps"] = steps
}

func failPendingDirectDocumentSteps(metadata map[string]any) {
	steps, ok := metadata["steps"].([]directImportStep)
	if !ok {
		return
	}
	for i := range steps {
		if steps[i].Status == "pending" || steps[i].Status == "processing" {
			steps[i].Status = "failed"
		}
	}
	metadata["steps"] = steps
}
