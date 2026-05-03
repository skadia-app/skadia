package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func gearTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "gear-"+userID[:8], "gear-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func TestCreateGearRejectsTupleDefaultSport(t *testing.T) {
	db, userID := gearTestDB(t)
	_, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:             "MTB",
		Type:             "bike",
		DefaultForSports: []string{"cycling:mountain"},
	})
	if err == nil {
		t.Fatal("CreateGear accepted tuple default sport")
	}
}

func TestCreateGearNormalizesDefaultSports(t *testing.T) {
	db, userID := gearTestDB(t)
	gear, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:             "Road",
		Type:             "bike",
		DefaultForSports: []string{" cycling ", "cycling"},
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	if len(gear.DefaultForSports) != 1 || gear.DefaultForSports[0] != "cycling" {
		t.Fatalf("DefaultForSports = %#v, want []string{cycling}", gear.DefaultForSports)
	}
}

func TestCreateGearAllowsStorageOnlyBroadFallbackDefaultSport(t *testing.T) {
	db, userID := gearTestDB(t)
	gear, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:             "Machine",
		Type:             "fitness",
		DefaultForSports: []string{"fitness_equipment"},
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	if len(gear.DefaultForSports) != 1 || gear.DefaultForSports[0] != "fitness_equipment" {
		t.Fatalf("DefaultForSports = %#v, want []string{fitness_equipment}", gear.DefaultForSports)
	}
}

func TestUpdateGearRejectsUnknownDefaultSport(t *testing.T) {
	db, userID := gearTestDB(t)
	gear, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name: "Road",
		Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	_, err = db.UpdateGear(context.Background(), gear.ID, &model.GearUpdate{
		DefaultForSports: []string{"unknown"},
	})
	if err == nil {
		t.Fatal("UpdateGear accepted unknown default sport")
	}
}
