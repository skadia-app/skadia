package handler

import (
	"context"
	"encoding/json"
	"fmt"
)

type documentProcessedMessage struct {
	DocID          string `json:"doc_id"`
	UserID         string `json:"user_id"`
	FileID         string `json:"file_id"`
	Success        bool   `json:"success"`
	MarkdownStatus string `json:"markdown_status"`
}

func (h *Handler) HandleDocumentProcessedMessage(ctx context.Context, data []byte) error {
	var msg documentProcessedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("decode document processed message: %w", err)
	}
	return h.DB.CompleteDocumentImportLog(ctx, msg.UserID, msg.DocID, msg.FileID, msg.Success, msg.MarkdownStatus)
}
