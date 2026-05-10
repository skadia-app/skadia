package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// TestResolveImportQueueItem_FlipsImportLogToCompleted verifies that when a
// queued media item is resolved (assigned to an activity), the corresponding
// import_log entry transitions from 'queued' to 'completed' so the UI doesn't
// show stale "In Queue" badges forever.
func TestResolveImportQueueItem_FlipsImportLogToCompleted(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		"queue-resolve-"+suffix, "queue-resolve-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// Create queue item + import_log entry with metadata.queue_id pointing to it.
	queueID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO import_queue (id, user_id, queue_type, status, original_filename, content_type)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		queueID, userID, "image_no_exif", "pending", "img.jpg", "image/jpeg",
	); err != nil {
		t.Fatalf("insert queue: %v", err)
	}

	photoType := model.ImportTypePhoto
	logID, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID: userID, SourcePlatform: "upload", Filename: "img.jpg",
		Status: "queued", ImportType: &photoType,
		Metadata: []byte(fmt.Sprintf(`{"queue_id":%q}`, queueID)),
	})
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	if err := db.ResolveImportQueueItem(ctx, queueID, userID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM import_log WHERE id = $1`, logID,
	).Scan(&status); err != nil {
		t.Fatalf("requery: %v", err)
	}
	if status != "completed" {
		t.Fatalf("import_log.status: want completed, got %s", status)
	}
}
