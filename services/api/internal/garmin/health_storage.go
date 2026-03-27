package garmin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// storeHealthFIT stores a health FIT file in ShadowFS for GDPR export and storage migration.
// Returns the file record ID. Skips storage if fs is nil or file already exists (file_hash dedup).
func storeHealthFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID, fileType, filename string, ts time.Time) (string, error) {
	if fs == nil {
		return "", nil
	}

	fileHash := sha256hex(data)

	// File-level dedup: skip Put if already stored
	var existingID string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM user_files WHERE user_id = $1 AND file_type = 'health' AND file_hash = $2`,
		userID, fileHash).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	rec, err := fs.Put(ctx, shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "health",
		ContentType: "application/vnd.ant.fit",
		FileName:    filename,
		SizeBytes:   int64(len(data)),
		FileHash:    fileHash,
		Tags: map[string]string{
			"type":  fileType,
			"year":  ts.Format("2006"),
			"month": ts.Format("01"),
			"day":   ts.Format("02"),
		},
	}, bytes.NewReader(data))
	if err != nil {
		slog.Warn("storeHealthFIT: ShadowFS.Put failed", "type", fileType, "error", err)
		return "", nil // non-fatal
	}
	return rec.ID, nil
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
