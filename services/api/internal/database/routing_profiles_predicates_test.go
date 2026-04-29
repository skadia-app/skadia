package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func routingProfilePredicateTestDB(t *testing.T) (*DB, string) {
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

	var hasColumns bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'routing_profiles'
			  AND column_name IN ('predicates', 'modifier_filters')
			GROUP BY table_name
			HAVING COUNT(*) = 2
		)`,
	).Scan(&hasColumns); err != nil {
		t.Fatalf("check routing profile predicate columns: %v", err)
	}
	if !hasColumns {
		t.Skip("routing_profiles predicate columns not migrated")
	}

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "routing-profile-"+userID[:8], "routing-profile-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM routing_profiles WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return &DB{Pool: pool}, userID
}

func TestRoutingProfilePredicatePersistence(t *testing.T) {
	db, userID := routingProfilePredicateTestDB(t)
	ctx := context.Background()
	trueValue := true
	mountain := "mountain"
	preds := []sport.Predicate{{Sport: "cycling", SubType: &mountain}}
	filters := sport.ModifierFilters{IsElectric: &trueValue}

	created, err := db.CreateRoutingProfile(ctx, userID, "mtb-"+uuid.NewString(), "mtb", preds, filters, []byte(`{"profile":"mtb"}`))
	if err != nil {
		t.Fatalf("CreateRoutingProfile: %v", err)
	}

	loaded, err := db.GetRoutingProfile(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRoutingProfile: %v", err)
	}
	if len(loaded.Predicates) != 1 || loaded.Predicates[0].Sport != "cycling" ||
		loaded.Predicates[0].SubType == nil || *loaded.Predicates[0].SubType != "mountain" {
		t.Fatalf("predicates = %#v, want cycling/mountain", loaded.Predicates)
	}
	if loaded.ModifierFilters.IsElectric == nil || !*loaded.ModifierFilters.IsElectric {
		t.Fatalf("modifier_filters = %#v, want is_electric=true", loaded.ModifierFilters)
	}
	if len(loaded.SportTypes) != 1 || loaded.SportTypes[0] != "cycling" {
		t.Fatalf("sport_types = %#v, want compatibility [cycling]", loaded.SportTypes)
	}
}
