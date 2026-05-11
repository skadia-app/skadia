package database

// Phase 4b.A.3 contract test: SeedSystemDefaults must NOT inline-INSERT the
// system tags + automation rules anymore — those moved to
// automation.EnsureSystemArtifacts. The DB-side seed function is kept for
// weather tags + recommended rules + migrate-existing-tags (none of which
// are part of the SystemTags/SystemRules registry), and is called by the
// handler AFTER automation.EnsureSystemArtifacts.
//
// We verify by running SeedSystemDefaults on a fresh user and asserting:
//   - 0 rows for system_type IN (round_trip, electric_sync, indoor_sync,
//     virtual_sync, frost, nacht) → these are now EnsureSystemArtifacts'
//     responsibility, NOT SeedSystemDefaults'.
//   - 0 rows for tag names {Rundtour, E-Bike, Pendeln, Indoor, virtuell,
//     Frost, Nacht} where category='system' → same reason.
//   - non-zero weather tags (sonnig, bewölkt, …) → still seeded here.
//
// Test is skipped when TEST_DATABASE_URL is unset.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func tagsDBSeedTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSeedSystemDefaults_DoesNotSeedSystemTagsOrRules(t *testing.T) {
	pool := tagsDBSeedTestPool(t)
	ctx := context.Background()
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"seedtest_"+userID[:8],
		"seedtest_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	db := &DB{Pool: pool}
	if err := db.SeedSystemDefaults(ctx, userID); err != nil {
		t.Fatalf("SeedSystemDefaults: %v", err)
	}

	systemTagNames := []string{"Rundtour", "E-Bike", "Pendeln", "Indoor", "virtuell", "Frost", "Nacht"}
	systemTypes := []string{"round_trip", "electric_sync", "indoor_sync", "virtual_sync", "frost", "nacht"}

	var sysTags, sysRules int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tags
		   WHERE user_id = $1 AND is_system = true AND name = ANY($2)`,
		userID, systemTagNames,
	).Scan(&sysTags); err != nil {
		t.Fatalf("count system tags: %v", err)
	}
	if sysTags != 0 {
		t.Errorf("expected SeedSystemDefaults to NOT seed system tags, got %d rows; that is now EnsureSystemArtifacts' job", sysTags)
	}

	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM automation_rules
		   WHERE user_id = $1 AND is_system = true AND system_type = ANY($2)`,
		userID, systemTypes,
	).Scan(&sysRules); err != nil {
		t.Fatalf("count system rules: %v", err)
	}
	if sysRules != 0 {
		t.Errorf("expected SeedSystemDefaults to NOT seed system rules, got %d rows; that is now EnsureSystemArtifacts' job", sysRules)
	}

	// Weather tags MUST still be seeded by SeedSystemDefaults (not part of the
	// SystemTags registry — they're a separate category).
	var weatherTags int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tags
		   WHERE user_id = $1 AND category = 'weather'`,
		userID,
	).Scan(&weatherTags); err != nil {
		t.Fatalf("count weather tags: %v", err)
	}
	if weatherTags == 0 {
		t.Errorf("expected SeedSystemDefaults to still seed weather tags, got 0")
	}
}
