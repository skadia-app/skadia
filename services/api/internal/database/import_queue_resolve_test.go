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

	if err := db.ResolveImportQueueItem(ctx, queueID, userID, ""); err != nil {
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

// TestResolveImportQueueItem_PropagatesActivityID verifies that when activityID is
// provided to ResolveImportQueueItem, it is written to the import_log.activity_id column.
func TestResolveImportQueueItem_PropagatesActivityID(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		"queue-actid-"+suffix, "queue-actid-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// Create a minimal activity row so activity_id FK is satisfied.
	var activityID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO activities (user_id, sport_type, started_at, elapsed_time_s)
		 VALUES ($1, 'running', NOW(), 3600) RETURNING id`,
		userID,
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	queueID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO import_queue (id, user_id, queue_type, status, original_filename, content_type)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		queueID, userID, "image_no_exif", "pending", "img2.jpg", "image/jpeg",
	); err != nil {
		t.Fatalf("insert queue: %v", err)
	}

	photoType := model.ImportTypePhoto
	logID, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID: userID, SourcePlatform: "upload", Filename: "img2.jpg",
		Status: "queued", ImportType: &photoType,
		Metadata: []byte(fmt.Sprintf(`{"queue_id":%q}`, queueID)),
	})
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	if err := db.ResolveImportQueueItem(ctx, queueID, userID, activityID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var status string
	var gotActivityID *string
	if err := pool.QueryRow(ctx,
		`SELECT status, activity_id::text FROM import_log WHERE id = $1`, logID,
	).Scan(&status, &gotActivityID); err != nil {
		t.Fatalf("requery: %v", err)
	}
	if status != "completed" {
		t.Fatalf("import_log.status: want completed, got %s", status)
	}
	if gotActivityID == nil || *gotActivityID != activityID {
		got := "<nil>"
		if gotActivityID != nil {
			got = *gotActivityID
		}
		t.Fatalf("import_log.activity_id: want %s, got %s", activityID, got)
	}
}

// TestResolveImportQueueItem_CrossUserIsolation verifies that resolving a queue item
// for user A does not flip import_log rows belonging to user B that share the same queue_id.
func TestResolveImportQueueItem_CrossUserIsolation(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]

	var userAID, userBID string
	for _, row := range []struct {
		id   *string
		slug string
	}{
		{&userAID, "resolve-a-" + suffix},
		{&userBID, "resolve-b-" + suffix},
	} {
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (username, email, registration_source)
			 VALUES ($1, $2, 'test') RETURNING id`,
			row.slug, row.slug+"@test.localhost",
		).Scan(row.id); err != nil {
			t.Fatalf("insert user %s: %v", row.slug, err)
		}
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{userAID, userBID})
	})

	// Both users share the same queue_id value in their import_log metadata,
	// simulating a scenario where user B's row happens to reference the same UUID.
	sharedQueueID := uuid.NewString()

	// Create queue item owned by user A.
	if _, err := pool.Exec(ctx,
		`INSERT INTO import_queue (id, user_id, queue_type, status, original_filename, content_type)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sharedQueueID, userAID, "image_no_exif", "pending", "a.jpg", "image/jpeg",
	); err != nil {
		t.Fatalf("insert queue: %v", err)
	}

	photoType := model.ImportTypePhoto

	// User A's import_log row, status='queued', metadata points to sharedQueueID.
	logAID, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID: userAID, SourcePlatform: "upload", Filename: "a.jpg",
		Status: "queued", ImportType: &photoType,
		Metadata: []byte(fmt.Sprintf(`{"queue_id":%q}`, sharedQueueID)),
	})
	if err != nil {
		t.Fatalf("insert log A: %v", err)
	}

	// User B's import_log row, same queue_id in metadata but owned by userB.
	logBID, err := db.InsertImportLog(ctx, model.ImportLogEntry{
		UserID: userBID, SourcePlatform: "upload", Filename: "b.jpg",
		Status: "queued", ImportType: &photoType,
		Metadata: []byte(fmt.Sprintf(`{"queue_id":%q}`, sharedQueueID)),
	})
	if err != nil {
		t.Fatalf("insert log B: %v", err)
	}

	// Resolve the queue item as user A.
	if err := db.ResolveImportQueueItem(ctx, sharedQueueID, userAID, ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// User A's row must be completed.
	var statusA string
	if err := pool.QueryRow(ctx, `SELECT status FROM import_log WHERE id = $1`, logAID).Scan(&statusA); err != nil {
		t.Fatalf("requery A: %v", err)
	}
	if statusA != "completed" {
		t.Fatalf("user A import_log.status: want completed, got %s", statusA)
	}

	// User B's row must remain 'queued' — cross-user isolation.
	var statusB string
	if err := pool.QueryRow(ctx, `SELECT status FROM import_log WHERE id = $1`, logBID).Scan(&statusB); err != nil {
		t.Fatalf("requery B: %v", err)
	}
	if statusB != "queued" {
		t.Fatalf("user B import_log.status: want queued (untouched), got %s", statusB)
	}
}
