//go:build integration

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// readinessTestDB connects via TEST_DATABASE_URL (skipped when unset).
func readinessTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("readinessTestDB: connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &DB{Pool: pool}
}

// seedReadinessUser inserts a user and registers cleanup for the user row plus
// any user_readiness_preferences entry (CASCADE handles preferences too, but
// being explicit keeps tests independent of cascade order).
func seedReadinessUser(t *testing.T, db *DB) string {
	t.Helper()
	ctx := context.Background()

	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])

	var uid string
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"readiness-pref-"+suffix,
		"readiness-pref-"+suffix+"@test.localhost",
	).Scan(&uid); err != nil {
		t.Fatalf("seedReadinessUser: insert user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
		_, _ = db.Pool.Exec(context.Background(),
			`DELETE FROM analytics_cache WHERE user_id = $1`, uid)
		_, _ = db.Pool.Exec(context.Background(),
			`DELETE FROM users WHERE id = $1`, uid)
	})

	return uid
}

// TestUserReadinessPreferenceRoundTrip verifies the full Get/Upsert/UpsertNull
// cycle, including the populated *string and the updated_at near NOW().
func TestUserReadinessPreferenceRoundTrip(t *testing.T) {
	db := readinessTestDB(t)
	ctx := context.Background()
	userID := seedReadinessUser(t, db)

	// 1. No row yet.
	pref, err := db.GetUserReadinessPreference(ctx, userID)
	if err != nil {
		t.Fatalf("Get (no row): %v", err)
	}
	if pref != nil {
		t.Fatalf("Get (no row): expected nil, got %+v", pref)
	}

	// 2. Upsert("skadia").
	beforeSkadia := time.Now().UTC()
	if err := db.UpsertUserReadinessPreference(ctx, userID, "skadia"); err != nil {
		t.Fatalf("Upsert(skadia): %v", err)
	}
	pref, err = db.GetUserReadinessPreference(ctx, userID)
	if err != nil {
		t.Fatalf("Get after Upsert: %v", err)
	}
	if pref == nil {
		t.Fatalf("Get after Upsert: expected row, got nil")
	}
	if pref.PreferredSource == nil || *pref.PreferredSource != "skadia" {
		t.Errorf("PreferredSource: expected *\"skadia\", got %v", pref.PreferredSource)
	}
	if pref.UpdatedAt.Before(beforeSkadia.Add(-time.Second)) ||
		pref.UpdatedAt.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("UpdatedAt out of expected window: %v (before=%v)", pref.UpdatedAt, beforeSkadia)
	}

	// 3. UpsertNull → automatic.
	if err := db.UpsertUserReadinessPreferenceNull(ctx, userID); err != nil {
		t.Fatalf("UpsertNull: %v", err)
	}
	pref, err = db.GetUserReadinessPreference(ctx, userID)
	if err != nil {
		t.Fatalf("Get after UpsertNull: %v", err)
	}
	if pref == nil {
		t.Fatalf("Get after UpsertNull: expected row, got nil")
	}
	if pref.PreferredSource != nil {
		t.Errorf("PreferredSource after UpsertNull: expected nil, got *%q", *pref.PreferredSource)
	}

	// 4. Reject unknown source value.
	if err := db.UpsertUserReadinessPreference(ctx, userID, "bogus"); err == nil {
		t.Errorf("Upsert(bogus): expected error, got nil")
	}
}

// TestUserReadinessPreferenceMonotonicUpdatedAt proves that updated_at is set
// explicitly via NOW() in the ON CONFLICT branch — without it the column would
// retain the original DEFAULT value from the first INSERT.
func TestUserReadinessPreferenceMonotonicUpdatedAt(t *testing.T) {
	db := readinessTestDB(t)
	ctx := context.Background()
	userID := seedReadinessUser(t, db)

	if err := db.UpsertUserReadinessPreference(ctx, userID, "skadia"); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	first, err := db.GetUserReadinessPreference(ctx, userID)
	if err != nil || first == nil {
		t.Fatalf("Get after first Upsert: pref=%v err=%v", first, err)
	}

	time.Sleep(2 * time.Millisecond)

	if err := db.UpsertUserReadinessPreference(ctx, userID, "skadia"); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	second, err := db.GetUserReadinessPreference(ctx, userID)
	if err != nil || second == nil {
		t.Fatalf("Get after second Upsert: pref=%v err=%v", second, err)
	}

	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("expected strictly increasing updated_at, got first=%v second=%v",
			first.UpdatedAt, second.UpdatedAt)
	}
}

// TestUserReadinessPreferenceNoRow verifies the (nil, nil) contract for
// unknown users. Uses a random UUID that has no users row, but the FK on
// preferences would prevent any accidental write — Get must just return empty.
func TestUserReadinessPreferenceNoRow(t *testing.T) {
	db := readinessTestDB(t)
	ctx := context.Background()

	unknown := uuid.New().String()
	pref, err := db.GetUserReadinessPreference(ctx, unknown)
	if err != nil {
		t.Fatalf("Get(unknown): %v", err)
	}
	if pref != nil {
		t.Fatalf("Get(unknown): expected nil, got %+v", pref)
	}
}

// TestAnalyticsCacheWithMetaRoundTrip covers the new Get/Upsert helpers:
// insert with TTL, update with different data + TTL, Get for missing key.
func TestAnalyticsCacheWithMetaRoundTrip(t *testing.T) {
	db := readinessTestDB(t)
	ctx := context.Background()
	userID := seedReadinessUser(t, db)

	cacheKey := "readiness_test_" + uuid.New().String()[:8]
	dataA := json.RawMessage(`{"score":42}`)
	dataB := json.RawMessage(`{"score":77}`)

	// 1. Missing key.
	row, err := db.GetAnalyticsCacheWithMeta(ctx, userID, cacheKey)
	if err != nil {
		t.Fatalf("Get(missing): %v", err)
	}
	if row != nil {
		t.Fatalf("Get(missing): expected nil, got %+v", row)
	}

	// 2. Insert with 6h TTL.
	insertedAt := time.Now().UTC()
	if err := db.UpsertAnalyticsCacheWithExpiry(ctx, userID, cacheKey, dataA, 6*time.Hour); err != nil {
		t.Fatalf("Upsert(6h): %v", err)
	}
	row, err = db.GetAnalyticsCacheWithMeta(ctx, userID, cacheKey)
	if err != nil {
		t.Fatalf("Get after first Upsert: %v", err)
	}
	if row == nil {
		t.Fatalf("Get after first Upsert: row missing")
	}
	if !jsonEqual(t, row.Data, dataA) {
		t.Errorf("Data mismatch: got %s want %s", row.Data, dataA)
	}
	if row.ExpiresAt == nil {
		t.Fatalf("ExpiresAt: expected non-nil after Upsert with TTL")
	}
	expectedExpiry := insertedAt.Add(6 * time.Hour)
	delta := row.ExpiresAt.Sub(expectedExpiry)
	if delta < -time.Minute || delta > time.Minute {
		t.Errorf("ExpiresAt out of ±1min window: got %v want ≈%v (delta=%v)",
			*row.ExpiresAt, expectedExpiry, delta)
	}

	// 3. Update existing row with different data + 2h TTL.
	updatedAt := time.Now().UTC()
	if err := db.UpsertAnalyticsCacheWithExpiry(ctx, userID, cacheKey, dataB, 2*time.Hour); err != nil {
		t.Fatalf("Upsert(2h update): %v", err)
	}
	row, err = db.GetAnalyticsCacheWithMeta(ctx, userID, cacheKey)
	if err != nil {
		t.Fatalf("Get after second Upsert: %v", err)
	}
	if row == nil {
		t.Fatalf("Get after second Upsert: row missing")
	}
	if !jsonEqual(t, row.Data, dataB) {
		t.Errorf("Data after update: got %s want %s", row.Data, dataB)
	}
	if row.ExpiresAt == nil {
		t.Fatalf("ExpiresAt after update: expected non-nil")
	}
	expectedExpiry2 := updatedAt.Add(2 * time.Hour)
	delta2 := row.ExpiresAt.Sub(expectedExpiry2)
	if delta2 < -time.Minute || delta2 > time.Minute {
		t.Errorf("ExpiresAt after update out of ±1min window: got %v want ≈%v (delta=%v)",
			*row.ExpiresAt, expectedExpiry2, delta2)
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var av, bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal a: %v (%s)", err, a)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal b: %v (%s)", err, b)
	}
	return reflect.DeepEqual(av, bv)
}
