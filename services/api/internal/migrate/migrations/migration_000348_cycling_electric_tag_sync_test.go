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
		"SELECT pg_advisory_xact_lock(hashtext('cycling_electric_tag_sync'), 1)",
		"INSERT INTO tags",
		"ON CONFLICT (user_id, name) DO UPDATE SET",
		"is_system = true",
		"category = 'system'",
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags",
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags_insert_junction",
		"NEW.sport_type = 'cycling'",
		"OLD.sport_type = 'cycling'",
		"BEFORE INSERT OR UPDATE OF tags, sport_type, is_indoor, is_electric, is_commute ON activities",
		"AFTER INSERT ON activities",
		"DELETE FROM activity_tags",
		"name = 'E-Bike'",
		"is_system = true",
		"UPDATE tags",
		"is_electric = true",
		"EXISTS (",
		"IF v_tag_id IS NOT NULL THEN",
		"ON CONFLICT (activity_id, tag_id) DO NOTHING",
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
		"SELECT pg_advisory_xact_lock(hashtext('cycling_electric_tag_sync'), 1)",
		"restore broad E-Bike",
		"NEW.is_electric := 'E-Bike'",
		"DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags_insert_junction",
		"DROP FUNCTION IF EXISTS sync_system_flags_and_tags_insert_junction()",
		"BEFORE UPDATE OF tags, is_indoor, is_electric, is_commute ON activities",
		"INSERT INTO activity_tags",
		"ON CONFLICT (activity_id, tag_id) DO NOTHING",
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
	cyclingClearID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES
			($1, $3, 'cycling electric', 'cycling', NULL, ARRAY[]::text[], true, now()),
			($2, $3, 'running electric', 'running', NULL, ARRAY['E-Bike']::text[], true, now()),
			($4, $3, 'cycling clear', 'cycling', NULL, ARRAY['E-Bike']::text[], true, now())
	`, cyclingID, runningID, userID, cyclingClearID); err != nil {
		t.Fatalf("insert activities: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
		VALUES ($1, $4, 'manual'), ($2, $4, 'auto'), ($3, $4, 'auto')
		ON CONFLICT DO NOTHING
	`, cyclingID, runningID, cyclingClearID, systemEBikeTagID); err != nil {
		t.Fatalf("insert activity tag links: %v", err)
	}

	missingTagUserID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, missingTagUserID, "mig348_missing_"+missingTagUserID[:8], "mig348_missing_"+missingTagUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert missing-tag user: %v", err)
	}
	missingTagCyclingID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES ($1, $2, 'missing tag cycling electric', 'cycling', NULL, ARRAY[]::text[], true, now())
	`, missingTagCyclingID, missingTagUserID); err != nil {
		t.Fatalf("insert missing-tag cycling activity: %v", err)
	}

	customExistingUserID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, customExistingUserID, "mig348_existing_"+customExistingUserID[:8], "mig348_existing_"+customExistingUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert custom-existing user: %v", err)
	}
	customExistingCyclingID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
		VALUES ($1, 'E-Bike', '#111111', '#FFFFFF', 'custom', false, false, 'custom')
	`, customExistingUserID); err != nil {
		t.Fatalf("insert pre-existing custom E-Bike tag: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES ($1, $2, 'custom existing cycling electric', 'cycling', NULL, ARRAY[]::text[], true, now())
	`, customExistingCyclingID, customExistingUserID); err != nil {
		t.Fatalf("insert custom-existing cycling activity: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, cyclingID, true)
	assertHasSystemEBikeLink000348(t, ctx, tx, runningID, false)
	assertActivityElectric000348(t, ctx, tx, runningID, true)
	assertActivityTagArrayHas000348(t, ctx, tx, runningID, "E-Bike", false)
	assertSystemEBikeTagCount000348(t, ctx, tx, missingTagUserID, 1)
	assertHasSystemEBikeLink000348(t, ctx, tx, missingTagCyclingID, true)
	assertActivityTagArrayHas000348(t, ctx, tx, missingTagCyclingID, "E-Bike", true)
	assertActivityTagLinkCount000348(t, ctx, tx, missingTagCyclingID, 1)
	assertSystemEBikeTagCount000348(t, ctx, tx, customExistingUserID, 1)
	assertSystemEBikeTagStyle000348(t, ctx, tx, customExistingUserID, "#111111", "#FFFFFF", "custom")
	assertHasSystemEBikeLink000348(t, ctx, tx, customExistingCyclingID, true)
	assertActivityTagArrayHas000348(t, ctx, tx, customExistingCyclingID, "E-Bike", true)

	if _, err := tx.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto)
		VALUES
			($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false),
			($1, 'Pendeln', '#6B7280', '#FFFFFF', 'system', true, false)
		ON CONFLICT (user_id, name) DO UPDATE SET is_system = true
	`, userID); err != nil {
		t.Fatalf("insert insert-trigger system tags: %v", err)
	}
	insertCyclingID := uuid.NewString()
	insertRunningID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_commute, started_at)
		VALUES
			($1, $3, 'insert cycling electric', 'cycling', NULL, ARRAY[]::text[], true, true, true, now()),
			($2, $3, 'insert running electric', 'running', NULL, ARRAY['E-Bike']::text[], false, true, false, now())
	`, insertCyclingID, insertRunningID, userID); err != nil {
		t.Fatalf("insert post-migration activities: %v", err)
	}
	assertActivityTagArrayHas000348(t, ctx, tx, insertCyclingID, "Indoor", true)
	assertActivityTagArrayHas000348(t, ctx, tx, insertCyclingID, "Pendeln", true)
	assertActivityTagArrayHas000348(t, ctx, tx, insertCyclingID, "E-Bike", true)
	assertHasSystemTagLink000348(t, ctx, tx, insertCyclingID, "Indoor", true)
	assertHasSystemTagLink000348(t, ctx, tx, insertCyclingID, "Pendeln", true)
	assertHasSystemTagLink000348(t, ctx, tx, insertCyclingID, "E-Bike", true)
	assertActivityElectric000348(t, ctx, tx, insertRunningID, true)
	assertActivityTagArrayHas000348(t, ctx, tx, insertRunningID, "E-Bike", false)
	assertHasSystemEBikeLink000348(t, ctx, tx, insertRunningID, false)

	customUserID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, customUserID, "mig348_custom_"+customUserID[:8], "mig348_custom_"+customUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert custom user: %v", err)
	}
	customRunningID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
		VALUES ($1, 'E-Bike', '#111111', '#FFFFFF', 'custom', false, false, 'custom')
	`, customUserID); err != nil {
		t.Fatalf("insert custom E-Bike tag: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES ($1, $2, 'custom e-bike text', 'running', NULL, ARRAY['E-Bike']::text[], true, now())
	`, customRunningID, customUserID); err != nil {
		t.Fatalf("insert custom E-Bike activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), 'Urlaub') WHERE id = $1`, customRunningID); err != nil {
		t.Fatalf("add unrelated custom E-Bike activity tag: %v", err)
	}
	assertActivityTagArrayHas000348(t, ctx, tx, customRunningID, "E-Bike", false)
	assertActivityElectric000348(t, ctx, tx, customRunningID, true)

	if _, err := tx.Exec(ctx, `UPDATE activities SET sport_type = 'running' WHERE id = $1`, cyclingID); err != nil {
		t.Fatalf("change cycling to running: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, cyclingID, false)
	assertActivityElectric000348(t, ctx, tx, cyclingID, true)

	if _, err := tx.Exec(ctx, `UPDATE activities SET is_electric = false WHERE id = $1`, cyclingClearID); err != nil {
		t.Fatalf("clear cycling electric: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, cyclingClearID, false)
	assertActivityTagArrayHas000348(t, ctx, tx, cyclingClearID, "E-Bike", false)
	assertActivityElectric000348(t, ctx, tx, cyclingClearID, false)

	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), 'Urlaub') WHERE id = $1`, runningID); err != nil {
		t.Fatalf("add unrelated running tag: %v", err)
	}
	assertHasSystemEBikeLink000348(t, ctx, tx, runningID, false)
	assertActivityElectric000348(t, ctx, tx, runningID, true)

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("reapply up.sql: %v", err)
	}
	assertSystemEBikeTagCount000348(t, ctx, tx, missingTagUserID, 1)
	assertActivityTagLinkCount000348(t, ctx, tx, missingTagCyclingID, 1)

	cascadeUserID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, cascadeUserID, "mig348_cascade_"+cascadeUserID[:8], "mig348_cascade_"+cascadeUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert cascade user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
		VALUES
			($1, 'E-Bike', '#16A34A', '#FFFFFF', 'system', true, false, 'test'),
			($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false, 'test')
		ON CONFLICT (user_id, name) DO UPDATE SET is_system = true
	`, cascadeUserID); err != nil {
		t.Fatalf("insert cascade system tags: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, cascadeUserID); err != nil {
		t.Fatalf("delete user with system tags should cascade: %v", err)
	}

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
	assertHasSystemTagLink000348(t, ctx, q, activityID, "E-Bike", want)
}

func assertHasSystemTagLink000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, activityID string, tagName string, want bool) {
	t.Helper()
	var got bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.name = $2 AND t.is_system = true
		)
	`, activityID, tagName).Scan(&got); err != nil {
		t.Fatalf("query %s link: %v", tagName, err)
	}
	if got != want {
		t.Fatalf("%s link for %s = %v, want %v", tagName, activityID, got, want)
	}
}

func assertSystemEBikeTagCount000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string, want int) {
	t.Helper()
	var got int
	if err := q.QueryRow(ctx, `
		SELECT COUNT(*) FROM tags
		WHERE user_id = $1 AND name = 'E-Bike' AND is_system = true
	`, userID).Scan(&got); err != nil {
		t.Fatalf("query system E-Bike tag count: %v", err)
	}
	if got != want {
		t.Fatalf("system E-Bike tag count for user %s = %d, want %d", userID, got, want)
	}
}

func assertSystemEBikeTagStyle000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string, wantColor string, wantTextColor string, wantDescription string) {
	t.Helper()
	var gotColor, gotTextColor, gotDescription string
	if err := q.QueryRow(ctx, `
		SELECT color, text_color, description
		FROM tags
		WHERE user_id = $1 AND name = 'E-Bike' AND is_system = true
	`, userID).Scan(&gotColor, &gotTextColor, &gotDescription); err != nil {
		t.Fatalf("query system E-Bike style: %v", err)
	}
	if gotColor != wantColor || gotTextColor != wantTextColor || gotDescription != wantDescription {
		t.Fatalf("system E-Bike style = %q/%q/%q, want %q/%q/%q", gotColor, gotTextColor, gotDescription, wantColor, wantTextColor, wantDescription)
	}
}

func assertActivityTagLinkCount000348(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, activityID string, want int) {
	t.Helper()
	var got int
	if err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM activity_tags at
		JOIN tags t ON t.id = at.tag_id
		WHERE at.activity_id = $1 AND t.name = 'E-Bike' AND t.is_system = true
	`, activityID).Scan(&got); err != nil {
		t.Fatalf("query E-Bike link count: %v", err)
	}
	if got != want {
		t.Fatalf("E-Bike link count for %s = %d, want %d", activityID, got, want)
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
