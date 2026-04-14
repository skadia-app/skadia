package enrichment

// Integration tests for retractIntervalTag — verify the transaction wrapper
// from #22 and the legacy-name cleanup from #23.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

func retractTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func retractTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"retract_test_"+userID[:8],
		"retract_test_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM activities WHERE user_id = $1", userID)    //nolint:errcheck
		pool.Exec(context.Background(), "DELETE FROM tags WHERE user_id = $1", userID)          //nolint:errcheck
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)              //nolint:errcheck
	})
	return userID
}

func retractTestActivity(t *testing.T, pool *pgxpool.Pool, userID string, initialTags []string) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	_, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash, tags)
		VALUES ($1, $2, 'cycling', $3, $4, $5, $6)
	`, activityID, userID, now, now.Add(30*time.Minute), uuid.New().String(), initialTags)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

func retractInsertTag(t *testing.T, pool *pgxpool.Pool, userID, name string) string {
	t.Helper()
	var tagID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO tags (user_id, name, color, text_color, is_auto, category)
		VALUES ($1, $2, '#EF4444', '#FFFFFF', true, 'activity')
		ON CONFLICT (user_id, name) DO UPDATE SET name = tags.name
		RETURNING id
	`, userID, name).Scan(&tagID)
	if err != nil {
		t.Fatalf("insert tag %q: %v", name, err)
	}
	return tagID
}

func retractLinkTag(t *testing.T, pool *pgxpool.Pool, activityID, tagID, assignedBy string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (activity_id, tag_id) DO UPDATE SET assigned_by = EXCLUDED.assigned_by
	`, activityID, tagID, assignedBy)
	if err != nil {
		t.Fatalf("link tag: %v", err)
	}
}

// TestRetractIntervalTag_RemovesBothCanonicalAndLegacy verifies that
// retractIntervalTag cleans up both "Intervalltraining" (canonical) and the
// legacy "Intervall-Training" name in a single transaction (#22, #23).
func TestRetractIntervalTag_RemovesBothCanonicalAndLegacy(t *testing.T) {
	pool := retractTestPool(t)
	db := &database.DB{Pool: pool}
	userID := retractTestUser(t, pool)
	activityID := retractTestActivity(t, pool, userID,
		[]string{"Intervalltraining", "Intervall-Training", "manual-tag"})

	// Seed both tag names linked to the activity as auto-assigned.
	canonicalID := retractInsertTag(t, pool, userID, "Intervalltraining")
	legacyID := retractInsertTag(t, pool, userID, "Intervall-Training")
	retractLinkTag(t, pool, activityID, canonicalID, "auto")
	retractLinkTag(t, pool, activityID, legacyID, "auto")

	// Insert a fake interval row so we can verify it is wiped by the retraction.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activity_intervals
		    (activity_id, segment_num, start_time, end_time, duration_s, intensity)
		VALUES ($1, 1, NOW(), NOW() + INTERVAL '60 seconds', 60, 'hard')
	`, activityID)
	if err != nil {
		t.Fatalf("seed interval: %v", err)
	}

	retractIntervalTag(context.Background(), db, activityID, userID)

	// Verify both tag names are unlinked.
	var remainingLinks int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM activity_tags at
		JOIN tags t ON at.tag_id = t.id
		WHERE at.activity_id = $1
		  AND t.name IN ('Intervalltraining', 'Intervall-Training')
	`, activityID).Scan(&remainingLinks); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if remainingLinks != 0 {
		t.Errorf("expected 0 remaining interval-tag links, got %d", remainingLinks)
	}

	// Verify both names are stripped from the denormalised tags array.
	var remainingTags []string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(tags, '{}') FROM activities WHERE id = $1`,
		activityID).Scan(&remainingTags); err != nil {
		t.Fatalf("fetch tags: %v", err)
	}
	for _, name := range []string{"Intervalltraining", "Intervall-Training"} {
		for _, t2 := range remainingTags {
			if t2 == name {
				t.Errorf("tags array still contains %q: %v", name, remainingTags)
			}
		}
	}
	// "manual-tag" must be preserved.
	hasManual := false
	for _, t2 := range remainingTags {
		if t2 == "manual-tag" {
			hasManual = true
		}
	}
	if !hasManual {
		t.Errorf("non-interval tag 'manual-tag' was erroneously removed: %v", remainingTags)
	}

	// Verify intervals are wiped.
	var intervalCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_intervals WHERE activity_id = $1`,
		activityID).Scan(&intervalCount); err != nil {
		t.Fatalf("count intervals: %v", err)
	}
	if intervalCount != 0 {
		t.Errorf("expected 0 intervals after retraction, got %d", intervalCount)
	}
}

// TestRetractIntervalTag_PreservesManualTag verifies that a manually-assigned
// "Intervalltraining" tag survives the retraction (auto vs. manual distinction).
func TestRetractIntervalTag_PreservesManualTag(t *testing.T) {
	pool := retractTestPool(t)
	db := &database.DB{Pool: pool}
	userID := retractTestUser(t, pool)
	activityID := retractTestActivity(t, pool, userID, []string{"Intervalltraining"})

	tagID := retractInsertTag(t, pool, userID, "Intervalltraining")
	retractLinkTag(t, pool, activityID, tagID, "manual")

	retractIntervalTag(context.Background(), db, activityID, userID)

	// Manual link must remain.
	var remainingLinks int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM activity_tags
		WHERE activity_id = $1 AND tag_id = $2
	`, activityID, tagID).Scan(&remainingLinks); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if remainingLinks != 1 {
		t.Errorf("manual-assigned Intervalltraining link was removed (got %d, want 1)", remainingLinks)
	}

	// Tag must still be in the denormalised array.
	var remainingTags []string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(tags, '{}') FROM activities WHERE id = $1`,
		activityID).Scan(&remainingTags); err != nil {
		t.Fatalf("fetch tags: %v", err)
	}
	found := false
	for _, t2 := range remainingTags {
		if t2 == "Intervalltraining" {
			found = true
		}
	}
	if !found {
		t.Errorf("manually-tagged Intervalltraining was stripped from tags array: %v", remainingTags)
	}
}
