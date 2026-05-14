package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestCompleteDocumentImportLogCompletesProcessingEntry(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()
	fileSize := int64(1234)
	metadata, _ := json.Marshal(map[string]any{
		"target_type": "gear_document",
		"target_id":   "doc-1",
		"file_id":     "file-1",
		"steps": []map[string]string{
			{"key": "upload", "label": "Upload", "status": "completed"},
			{"key": "text_extraction", "label": "Texterkennung", "status": "pending"},
			{"key": "markdown", "label": "Markdown", "status": "pending"},
		},
	})
	importType := model.ImportTypeDocument
	logID, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID:         userID,
		SourcePlatform: "upload",
		Filename:       "manual.pdf",
		FileSizeBytes:  &fileSize,
		Status:         "processing",
		Metadata:       metadata,
		ImportType:     &importType,
	})
	if err != nil {
		t.Fatalf("InsertImportLog: %v", err)
	}

	if err := db.CompleteDocumentImportLog(ctx, userID, "doc-1", "file-1", true, "completed"); err != nil {
		t.Fatalf("CompleteDocumentImportLog: %v", err)
	}

	var status, textStep, markdownStep string
	if err := db.Pool.QueryRow(ctx, `
		SELECT status, metadata #>> '{steps,1,status}', metadata #>> '{steps,2,status}'
		FROM import_log
		WHERE id = $1`,
		logID,
	).Scan(&status, &textStep, &markdownStep); err != nil {
		t.Fatalf("select import log: %v", err)
	}
	if status != "completed" || textStep != "completed" || markdownStep != "completed" {
		t.Fatalf("document import log = status %q text_step %q markdown_step %q, want completed/completed/completed",
			status, textStep, markdownStep)
	}
}
