package database

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ImportTracker wraps a single import_log entry and provides a clean API
// for state transitions. All methods are safe to call with a nil tracker
// (when InsertImportLog failed at creation).
type ImportTracker struct {
	db    *DB
	logID string
	start time.Time
}

// NewImportTracker inserts a new import_log entry (status=pending) and returns
// a tracker for it. importType should be one of the model.ImportType* constants.
// Unknown values are accepted but logged as a warning (no DB CHECK constraint).
// batchID may be nil for single-file imports (hotfolder, HTTP upload).
// The tracker is always non-nil; if the DB insert fails, subsequent calls are no-ops.
func (db *DB) NewImportTracker(ctx context.Context, userID, platform, filename string, fileSize *int64, batchID *string, importType string) *ImportTracker {
	if importType != "" && !model.IsValidImportType(importType) {
		slog.Warn("import_log: unknown import_type",
			"type", importType, "platform", platform, "file", filename)
	}
	// Empty-string guard — otherwise "" lands in the column instead of NULL.
	var importTypePtr *string
	if importType != "" {
		v := importType
		importTypePtr = &v
	}
	entry := model.ImportLogEntry{
		UserID:         userID,
		SourcePlatform: platform,
		Filename:       filename,
		FileSizeBytes:  fileSize,
		Status:         "pending",
		BatchID:        batchID,
		ImportType:     importTypePtr,
	}
	logID, err := db.InsertImportLog(ctx, entry)
	if err != nil {
		slog.Warn("import_log: failed to create tracker", "file", filename, "error", err)
		logID = ""
	}
	return &ImportTracker{db: db, logID: logID, start: time.Now()}
}

// SetImportType updates the import_type for this tracker entry.
// Use when the type isn't known at tracker creation time.
// An empty importType writes NULL (same coercion as NewImportTracker).
func (t *ImportTracker) SetImportType(ctx context.Context, importType string) {
	if t.logID == "" {
		return
	}
	if importType != "" && !model.IsValidImportType(importType) {
		slog.Warn("import_log: unknown import_type in SetImportType", "type", importType, "id", t.logID)
	}
	var importTypePtr *string
	if importType != "" {
		v := importType
		importTypePtr = &v
	}
	if _, err := t.db.Pool.Exec(ctx,
		`UPDATE import_log SET import_type = $2 WHERE id = $1`,
		t.logID, importTypePtr,
	); err != nil {
		slog.Warn("import_log: failed to set import_type", "id", t.logID, "error", err)
	}
}

// Queued marks the entry as 'queued' — a terminal status for files stored in
// import_queue awaiting manual type/activity assignment. Unlike 'pending',
// this is NOT picked up by useImportQueue (which polls 'pending,processing'),
// so the "currently importing" indicator stays clean.
// queueID is merged into metadata under "queue_id" — pre-existing metadata
// (e.g. candidate_ids from queueMediaFile) is preserved.
func (t *ImportTracker) Queued(ctx context.Context, queueID string) {
	if t.logID == "" {
		return
	}
	elapsed := int(time.Since(t.start).Milliseconds())
	meta, _ := json.Marshal(map[string]string{"queue_id": queueID})
	_, err := t.db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = 'queued',
		    duration_ms = $2,
		    metadata = COALESCE(metadata, '{}'::jsonb) || $3::jsonb
		WHERE id = $1`,
		t.logID, elapsed, meta,
	)
	if err != nil {
		slog.Warn("import_log: failed to set queued", "id", t.logID, "error", err)
	}
}

// LogID returns the import_log row ID (empty string if creation failed).
func (t *ImportTracker) LogID() string { return t.logID }

// Processing transitions the entry to "processing". Call this before heavy I/O.
func (t *ImportTracker) Processing(ctx context.Context) {
	if t.logID == "" {
		return
	}
	if err := t.db.UpdateImportLogStatus(ctx, t.logID, "processing", nil, nil, nil); err != nil {
		slog.Warn("import_log: failed to set processing", "id", t.logID, "error", err)
	}
}

// SetFileHash writes the computed file hash back to the DB.
// Call this as soon as the hash is available (after reading the file).
func (t *ImportTracker) SetFileHash(ctx context.Context, hash string) {
	if t.logID == "" || hash == "" {
		return
	}
	if err := t.db.UpdateImportLogFileHash(ctx, t.logID, hash); err != nil {
		slog.Warn("import_log: failed to set file_hash", "id", t.logID, "error", err)
	}
}

// Completed marks the entry as completed. activityID may be empty (e.g. monitoring FITs).
func (t *ImportTracker) Completed(ctx context.Context, activityID string) {
	if t.logID == "" {
		return
	}
	elapsed := int(time.Since(t.start).Milliseconds())
	var actPtr *string
	if activityID != "" {
		actPtr = &activityID
	}
	if err := t.db.UpdateImportLogStatus(ctx, t.logID, "completed", nil, actPtr, &elapsed); err != nil {
		slog.Warn("import_log: failed to set completed", "id", t.logID, "error", err)
	}
}

// Skipped marks the entry as skipped. reason identifies why (e.g. "duplicate",
// "date_filtered", "format_unsupported", "not_activity_type").
// activityID may be nil; for duplicates pass the existing activity's ID.
// status, skip_reason, activity_id and duration_ms are written in a single UPDATE.
func (t *ImportTracker) Skipped(ctx context.Context, reason string, activityID *string) {
	if t.logID == "" {
		return
	}
	elapsed := int(time.Since(t.start).Milliseconds())
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	_, err := t.db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = 'skipped', activity_id = $2, skip_reason = $3, duration_ms = $4
		WHERE id = $1`,
		t.logID, activityID, reasonPtr, elapsed,
	)
	if err != nil {
		slog.Warn("import_log: failed to set skipped", "id", t.logID, "error", err)
	}
}

// Error marks the entry as error with the given error message.
func (t *ImportTracker) Error(ctx context.Context, err error) {
	if t.logID == "" {
		return
	}
	elapsed := int(time.Since(t.start).Milliseconds())
	msg := err.Error()
	if dbErr := t.db.UpdateImportLogStatus(ctx, t.logID, "error", &msg, nil, &elapsed); dbErr != nil {
		slog.Warn("import_log: failed to set error", "id", t.logID, "error", dbErr)
	}
}

// SetMetadata writes JSONB metadata to the entry.
func (t *ImportTracker) SetMetadata(ctx context.Context, metadata []byte) {
	if t.logID == "" {
		return
	}
	if err := t.db.UpdateImportLogMetadata(ctx, t.logID, metadata); err != nil {
		slog.Warn("import_log: failed to set metadata", "id", t.logID, "error", err)
	}
}
