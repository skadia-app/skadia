package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ImportQueueItem represents a queued file needing manual review.
type ImportQueueItem struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id"`
	QueueType        string         `json:"queue_type"` // image_no_exif|image_ambiguous|pdf_untyped|duplicate|import_error
	Status           string         `json:"status"`     // pending|resolved|dismissed
	OriginalFilename string         `json:"original_filename"`
	ContentType      string         `json:"content_type"`
	FileSizeBytes    *int64         `json:"file_size_bytes,omitempty"`
	S3Bucket         *string        `json:"s3_bucket,omitempty"`
	S3Key            *string        `json:"s3_key,omitempty"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	ResolvedAt       *time.Time     `json:"resolved_at,omitempty"`
}

// ListImportQueue returns all queue items for the user, optionally filtered by status.
// status="" or status="all" returns all items.
func (db *DB) ListImportQueue(ctx context.Context, userID, status string) ([]ImportQueueItem, error) {
	query := `
		SELECT id, user_id, queue_type, status, original_filename, content_type,
		       file_size_bytes, s3_bucket, s3_key, metadata, created_at, resolved_at
		FROM import_queue
		WHERE user_id = $1`
	args := []any{userID}

	if status != "" && status != "all" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ImportQueueItem, 0)
	for rows.Next() {
		var item ImportQueueItem
		var metaRaw []byte
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.QueueType, &item.Status,
			&item.OriginalFilename, &item.ContentType,
			&item.FileSizeBytes, &item.S3Bucket, &item.S3Key,
			&metaRaw, &item.CreatedAt, &item.ResolvedAt,
		); err != nil {
			return nil, err
		}
		if metaRaw != nil {
			_ = json.Unmarshal(metaRaw, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CountPendingImportQueue returns the number of pending items for the user.
func (db *DB) CountPendingImportQueue(ctx context.Context, userID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM import_queue WHERE user_id = $1 AND status = 'pending'`,
		userID,
	).Scan(&count)
	return count, err
}

// CreateImportQueueItem inserts a new item and returns its generated ID.
func (db *DB) CreateImportQueueItem(ctx context.Context, item ImportQueueItem) (string, error) {
	id := uuid.New().String()

	metaJSON, err := json.Marshal(item.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO import_queue
			(id, user_id, queue_type, status, original_filename, content_type,
			 file_size_bytes, s3_bucket, s3_key, metadata)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7, $8, $9)
	`, id, item.UserID, item.QueueType, item.OriginalFilename, item.ContentType,
		item.FileSizeBytes, item.S3Bucket, item.S3Key, metaJSON,
	)
	return id, err
}

// UpdateImportQueueStatus updates the status (and optionally resolved_at) of an item.
func (db *DB) UpdateImportQueueStatus(ctx context.Context, id, userID, status string, resolvedAt *time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE import_queue
		SET status = $1, resolved_at = $2
		WHERE id = $3 AND user_id = $4
	`, status, resolvedAt, id, userID)
	return err
}

// DeleteImportQueueItem hard-deletes an item (dismiss).
func (db *DB) DeleteImportQueueItem(ctx context.Context, id, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM import_queue WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

// GetImportQueueItem returns a single item by ID for the given user.
func (db *DB) GetImportQueueItem(ctx context.Context, id, userID string) (*ImportQueueItem, error) {
	var item ImportQueueItem
	var metaRaw []byte
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, queue_type, status, original_filename, content_type,
		       file_size_bytes, s3_bucket, s3_key, metadata, created_at, resolved_at
		FROM import_queue
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&item.ID, &item.UserID, &item.QueueType, &item.Status,
		&item.OriginalFilename, &item.ContentType,
		&item.FileSizeBytes, &item.S3Bucket, &item.S3Key,
		&metaRaw, &item.CreatedAt, &item.ResolvedAt,
	)
	if err != nil {
		return nil, err
	}
	if metaRaw != nil {
		_ = json.Unmarshal(metaRaw, &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return &item, nil
}

// FindActivitiesCoveringTime returns activities whose time window contains ts.
func (db *DB) FindActivitiesCoveringTime(ctx context.Context, userID string, ts time.Time) ([]ActivityTimeMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, title, started_at, ended_at
		FROM activities
		WHERE user_id = $1
		  AND started_at <= $2
		  AND ended_at >= $2
		ORDER BY started_at DESC
		LIMIT 10
	`, userID, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ActivityTimeMatch
	for rows.Next() {
		var m ActivityTimeMatch
		if err := rows.Scan(&m.ID, &m.Title, &m.StartedAt, &m.EndedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// ActivityTimeMatch is a lightweight activity record used for time-based matching.
type ActivityTimeMatch struct {
	ID        string     `json:"id"`
	Title     *string    `json:"title,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}
