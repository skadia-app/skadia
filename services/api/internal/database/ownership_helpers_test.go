package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestOwnershipHelpers(t *testing.T) {
	db, userA := ownershipTestDB(t)
	_, userB := ownershipTestDB(t)
	ctx := context.Background()

	gearA, err := db.CreateGear(ctx, userA, &model.GearCreate{Name: "Bike A", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear A: %v", err)
	}
	gearB, err := db.CreateGear(ctx, userB, &model.GearCreate{Name: "Bike B", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear B: %v", err)
	}
	activityA := insertOwnershipActivity(t, db, userA)
	activityB := insertOwnershipActivity(t, db, userB)

	owned, err := db.GearOwnedBy(ctx, gearA.ID, userA)
	assertOwned(t, "own gear", owned, err, true)
	owned, err = db.GearOwnedBy(ctx, gearB.ID, userA)
	assertOwned(t, "foreign gear", owned, err, false)
	owned, err = db.GearOwnedBy(ctx, uuid.NewString(), userA)
	assertOwned(t, "missing gear", owned, err, false)
	if owned, err := db.GearOwnedBy(ctx, gearA.ID, ""); err == nil || owned {
		t.Fatalf("empty gear userID owned=%v err=%v, want false + error", owned, err)
	}

	owned, err = db.ActivityOwnedBy(ctx, activityA, userA)
	assertOwned(t, "own activity", owned, err, true)
	owned, err = db.ActivityOwnedBy(ctx, activityB, userA)
	assertOwned(t, "foreign activity", owned, err, false)
	owned, err = db.ActivityOwnedBy(ctx, uuid.NewString(), userA)
	assertOwned(t, "missing activity", owned, err, false)
	if owned, err := db.ActivityOwnedBy(ctx, activityA, ""); err == nil || owned {
		t.Fatalf("empty activity userID owned=%v err=%v, want false + error", owned, err)
	}
}

func ownershipTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "ownership-"+userID[:8], "ownership-"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func insertOwnershipActivity(t *testing.T, db *DB, userID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(),
		`INSERT INTO activities (id, user_id, title, sport_type, started_at)
		 VALUES ($1, $2, $3, 'running', $4)`,
		id, userID, "Ownership Activity", time.Now().UTC())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return id
}

func assertOwned(t *testing.T, name string, got bool, err error, want bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
