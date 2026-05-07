package database

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/rules"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ruleChangeTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	userID := uuid.New().String()
	email := "rule-change-" + uuid.New().String() + "@example.com"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "rule-change-"+uuid.New().String(), email); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return &DB{Pool: pool}, userID
}

func insertRuleChangeActivity(t *testing.T, db *DB, userID string, manual bool) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.New().String()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, sub_type, parsed_sport_type, parsed_sub_type, sport_type_is_manual, started_at, is_electric)
		VALUES
			($1, $2, 'cycling', 'mountain', 'cycling', 'road', $3, NOW(), false)`,
		activityID, userID, manual); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

func TestApplyRuleChangeSportTupleUpdatesSportAndSubTypeAtomically(t *testing.T) {
	db, userID := ruleChangeTestDB(t)
	activityID := insertRuleChangeActivity(t, db, userID, false)

	if err := db.ApplyRuleChange(context.Background(), activityID, "sport_tuple", rules.SportTupleChange{
		SportType: "running",
		SubType:   "",
	}); err != nil {
		t.Fatalf("ApplyRuleChange sport_tuple: %v", err)
	}

	var sportType string
	var subType *string
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT sport_type, sub_type FROM activities WHERE id = $1`, activityID,
	).Scan(&sportType, &subType); err != nil {
		t.Fatalf("select activity: %v", err)
	}
	if sportType != "running" {
		t.Fatalf("sport_type = %q, want running", sportType)
	}
	if subType != nil {
		t.Fatalf("sub_type = %q, want NULL", *subType)
	}
}

func TestApplyRuleChangeSportTupleManualGuardSkipsBothFields(t *testing.T) {
	db, userID := ruleChangeTestDB(t)
	activityID := insertRuleChangeActivity(t, db, userID, true)

	if err := db.ApplyRuleChange(context.Background(), activityID, "sport_tuple", rules.SportTupleChange{
		SportType: "running",
		SubType:   "",
	}); err != nil {
		t.Fatalf("ApplyRuleChange sport_tuple: %v", err)
	}

	var sportType string
	var subType string
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT sport_type, sub_type FROM activities WHERE id = $1`, activityID,
	).Scan(&sportType, &subType); err != nil {
		t.Fatalf("select activity: %v", err)
	}
	if sportType != "cycling" || subType != "mountain" {
		t.Fatalf("tuple = %s/%s, want cycling/mountain", sportType, subType)
	}
}

func TestApplyRuleChangeElectricUpdatesActivityFlag(t *testing.T) {
	db, userID := ruleChangeTestDB(t)
	activityID := insertRuleChangeActivity(t, db, userID, false)

	if err := db.ApplyRuleChange(context.Background(), activityID, "is_electric", true); err != nil {
		t.Fatalf("ApplyRuleChange is_electric: %v", err)
	}

	var isElectric bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT is_electric FROM activities WHERE id = $1`, activityID,
	).Scan(&isElectric); err != nil {
		t.Fatalf("select activity: %v", err)
	}
	if !isElectric {
		t.Fatal("is_electric = false, want true")
	}
}
