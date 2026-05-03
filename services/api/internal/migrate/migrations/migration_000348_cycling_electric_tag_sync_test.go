package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func readMigration000348CyclingElectricTagSyncSQL(t *testing.T) (upSQL, downSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000348_cycling_electric_tag_sync.up.sql")
	if err != nil {
		t.Fatalf("read 000348 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000348_cycling_electric_tag_sync.down.sql")
	if err != nil {
		t.Fatalf("read 000348 down.sql: %v", err)
	}
	return string(upBytes), string(downBytes)
}

func TestMigration000348CyclingElectricTagSyncDeclaresRequiredContracts(t *testing.T) {
	upSQL, downSQL := readMigration000348CyclingElectricTagSyncSQL(t)
	for _, needle := range []string{
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags",
		"NEW.sport_type = 'cycling'",
		"OLD.sport_type = 'cycling'",
		"BEFORE UPDATE OF tags, sport_type, is_indoor, is_electric, is_commute ON activities",
		"DELETE FROM activity_tags",
		"name = 'E-Bike'",
		"is_system = true",
		"UPDATE tags",
		"is_electric = true",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up migration missing %q", needle)
		}
	}
	for _, forbidden := range []string{
		"trg_sync_virtual_tag",
		"sync_virtual_tag",
		"NEW.is_electric := false",
	} {
		if strings.Contains(upSQL, forbidden) {
			t.Fatalf("up migration must not touch virtual triggers or clear generic electric state, found %q", forbidden)
		}
	}
	for _, needle := range []string{
		"restore broad E-Bike",
		"NEW.is_electric := 'E-Bike'",
		"BEFORE UPDATE OF tags, is_indoor, is_electric, is_commute ON activities",
		"INSERT INTO activity_tags",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("down migration missing %q", needle)
		}
	}
}

func TestMigration000348CyclingElectricTagSyncAppliesToRealTables(t *testing.T) {
	upSQL, downSQL := readMigration000348CyclingElectricTagSyncSQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "mig348_"+userID[:8], "mig348_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var systemEBikeTagID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
		VALUES ($1, 'E-Bike', '#16A34A', '#FFFFFF', 'system', true, false, 'old')
		ON CONFLICT (user_id, name) DO UPDATE SET is_system = true
		RETURNING id
	`, userID).Scan(&systemEBikeTagID); err != nil {
		t.Fatalf("insert system E-Bike tag: %v", err)
	}

	cyclingID := uuid.NewString()
	runningID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES
			($1, $3, 'cycling electric', 'cycling', NULL, ARRAY[]::text[], true, now()),
			($2, $3, 'running electric', 'running', NULL, ARRAY['E-Bike']::text[], true, now())
	`, cyclingID, runningID, userID); err != nil {
		t.Fatalf("insert activities: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
		VALUES ($1, $3, 'manual'), ($2, $3, 'auto')
		ON CONFLICT DO NOTHING
	`, cyclingID, runningID, systemEBikeTagID); err != nil {
		t.Fatalf("insert activity tag links: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, cyclingID, true)
	assertHasSystemEBikeLink000348(t, ctx, tx, runningID, false)
	assertActivityElectric000348(t, ctx, tx, runningID, true)
	assertActivityTagArrayHas000348(t, ctx, tx, runningID, "E-Bike", false)

	if _, err := tx.Exec(ctx, `UPDATE activities SET sport_type = 'running' WHERE id = $1`, cyclingID); err != nil {
		t.Fatalf("change cycling to running: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, cyclingID, false)
	assertActivityElectric000348(t, ctx, tx, cyclingID, true)

	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), 'Urlaub') WHERE id = $1`, runningID); err != nil {
		t.Fatalf("add unrelated running tag: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, runningID, false)
	assertActivityElectric000348(t, ctx, tx, runningID, true)

	if _, err := tx.Exec(ctx, downSQL); err != nil {
		t.Fatalf("apply down.sql: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE activities SET is_electric = false WHERE id = $1`, runningID); err != nil {
		t.Fatalf("down clear running electric: %v", err)
	}
	assertActivityTagArrayHas000348(t, ctx, tx, runningID, "E-Bike", false)
}

func assertHasSystemEBikeLink000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, activityID string, want bool) {
	t.Helper()
	var got bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.name = 'E-Bike' AND t.is_system = true
		)
	`, activityID).Scan(&got); err != nil {
		t.Fatalf("query E-Bike link: %v", err)
	}
	if got != want {
		t.Fatalf("E-Bike link for %s = %v, want %v", activityID, got, want)
	}
}

func assertActivityElectric000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, activityID string, want bool) {
	t.Helper()
	var got bool
	if err := q.QueryRow(ctx, `SELECT is_electric FROM activities WHERE id = $1`, activityID).Scan(&got); err != nil {
		t.Fatalf("query is_electric: %v", err)
	}
	if got != want {
		t.Fatalf("is_electric for %s = %v, want %v", activityID, got, want)
	}
}

func assertActivityTagArrayHas000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, activityID string, tag string, want bool) {
	t.Helper()
	var got bool
	if err := q.QueryRow(ctx, `SELECT $2 = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1`, activityID, tag).Scan(&got); err != nil {
		t.Fatalf("query tags array: %v", err)
	}
	if got != want {
		t.Fatalf("activity tags array contains %q for %s = %v, want %v", tag, activityID, got, want)
	}
}
