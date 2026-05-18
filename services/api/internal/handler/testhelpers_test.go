package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fusionTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func newTestHandler(t *testing.T, pool *pgxpool.Pool) *Handler {
	t.Helper()
	return &Handler{
		DB: &database.DB{Pool: pool},
	}
}

type TestUserOpt func(*testUserOpts)
type testUserOpts struct {
	isAdmin bool
}

func WithIsAdmin(b bool) TestUserOpt { return func(o *testUserOpts) { o.isAdmin = b } }

func insertTestUser(t *testing.T, pool *pgxpool.Pool, opts ...TestUserOpt) string {
	t.Helper()
	o := testUserOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	email := "test-" + randomHex(8) + "@example.com"
	username := "testuser-" + randomHex(4)
	var userID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		 VALUES (gen_random_uuid(), $1, $2, true, 'test', $3) RETURNING id`,
		username, email, o.isAdmin).Scan(&userID)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		// segments.created_by hat keinen CASCADE auf users → erst segments weg,
		// sonst scheitert der users-DELETE silent an der FK und Test-Rows leaken.
		// segment_versions + segment_efforts kommen über ihre CASCADEs mit.
		if _, err := pool.Exec(ctx, "DELETE FROM segments WHERE created_by = $1", userID); err != nil {
			t.Logf("cleanup segments: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM activities WHERE user_id = $1", userID); err != nil {
			t.Logf("cleanup activities: %v", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE tags SET is_system = false WHERE user_id = $1", userID); err != nil {
			t.Logf("cleanup system tags: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Logf("cleanup user: %v", err)
		}
	})
	return userID
}

type TestSegmentOpts struct {
	Track         string // WKT
	SegmentType   string
	Bidirectional *bool // nil = default true; use boolPtr(false) to explicitly disable
}

// boolPtr returns a pointer to the given bool value.
// Use with TestSegmentOpts.Bidirectional to explicitly set false (the zero value
// of *bool is nil, which insertTestSegment treats as the default true).
func boolPtr(b bool) *bool { return &b }

func insertTestSegment(t *testing.T, pool *pgxpool.Pool, userID string, opts TestSegmentOpts) string {
	t.Helper()
	if opts.Track == "" {
		opts.Track = "LINESTRING(0 0, 1 1)"
	}
	if opts.SegmentType == "" {
		opts.SegmentType = "standard"
	}
	bidi := true
	if opts.Bidirectional != nil {
		bidi = *opts.Bidirectional
	}

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	var segID string
	err = tx.QueryRow(ctx, `
		INSERT INTO segments (id, name, sport_group, segment_type, created_by, visibility, bidirectional)
		VALUES (gen_random_uuid(), 'test-seg-'||gen_random_uuid(), 'generic', $1, $2, 'public', $3)
		RETURNING id`,
		opts.SegmentType, userID, bidi).Scan(&segID)
	if err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES (gen_random_uuid(), $1, 1, ST_GeomFromText($2, 4326), CURRENT_DATE, $3)`,
		segID, opts.Track, userID)
	if err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	return segID
}

func insertTestEffort(t *testing.T, pool *pgxpool.Pool, segmentID, userID string, isReverse bool) string {
	t.Helper()
	versionID := lookupLatestTestSegmentVersion(t, pool, segmentID)
	activityID := insertTestActivityAt(t, pool, userID, time.Now().UTC())

	return insertTestEffortForActivity(t, pool, segmentID, versionID, activityID, userID, isReverse)
}

func lookupLatestTestSegmentVersion(t *testing.T, pool *pgxpool.Pool, segmentID string) string {
	t.Helper()
	var versionID string
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM segment_versions WHERE segment_id = $1 ORDER BY version_number DESC LIMIT 1`,
		segmentID).Scan(&versionID); err != nil {
		t.Fatalf("lookup version: %v", err)
	}
	return versionID
}

func insertTestActivityAt(t *testing.T, pool *pgxpool.Pool, userID string, startedAt time.Time) string {
	t.Helper()
	var activityID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO activities (id, user_id, sport_type, started_at)
			 VALUES (gen_random_uuid(), $1, 'running', $2) RETURNING id`,
		userID, startedAt).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

func insertTestEffortForActivity(t *testing.T, pool *pgxpool.Pool, segmentID, versionID, activityID, userID string, isReverse bool) string {
	t.Helper()
	var effortID string
	err := pool.QueryRow(context.Background(), `
			INSERT INTO segment_efforts
			  (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, is_reverse, sport_type, sub_type)
			SELECT gen_random_uuid(), $1, $2, $3, $4, a.started_at, 100, $5, 'running', 'generic'
			FROM activities a
			WHERE a.id = $3
			RETURNING id`,
		segmentID, versionID, activityID, userID, isReverse).Scan(&effortID)
	if err != nil {
		t.Fatalf("insert effort: %v", err)
	}
	return effortID
}

func insertMinimalActivity(t *testing.T, db *database.DB, userID string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: now,
		EndedAt:   now.Add(30 * time.Minute),
		FileHash:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("insertMinimalActivity: %v", err)
	}
	return act.ID
}

func withUserAndAdmin(ctx context.Context, userID string, isAdmin bool) context.Context {
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.IsAdminKey, isAdmin)
	return ctx
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func fusionPtrF64(v float64) *float64 { return &v }
func fusionPtrI16(v int16) *int16     { return &v }
