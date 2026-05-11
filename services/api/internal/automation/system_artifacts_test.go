package automation

// Tests for the central system-artifacts spec list + EnsureSystemArtifacts.
// Phase 4b.A.2 — see docs/internal/2026-05-04-lift-detection-go-migration-plan.md.
//
// Integration tests against TEST_DATABASE_URL (skipped when unset).

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// systemArtifactsTestPool returns a pgxpool to the per-worktree test DB via testdb.RequireSafeDSN.
func systemArtifactsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// createTestUser inserts a synthetic user and registers cleanup. Returns the user UUID.
// Uses random UUIDs and synthetic usernames — no production data.
func createTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"sysart_"+userID[:8],
		"sysart_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		// CASCADE removes tags + automation_rules.
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
	return userID
}

func TestEnsureSystemArtifacts_Idempotent(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	for i := 0; i < 2; i++ {
		if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
			t.Fatalf("EnsureSystemArtifacts call %d: %v", i, err)
		}
	}

	// Each spec produces exactly one tag and one rule.
	var tagCount, ruleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tags WHERE user_id = $1 AND is_system = true AND name = ANY($2)`,
		userID, namesFromTagSpecs(SystemTags),
	).Scan(&tagCount); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagCount != len(SystemTags) {
		t.Errorf("tag count = %d, want %d (one per SystemTagSpec)", tagCount, len(SystemTags))
	}

	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM automation_rules WHERE user_id = $1 AND is_system = true AND system_type = ANY($2)`,
		userID, systemTypesFromRuleSpecs(SystemRules),
	).Scan(&ruleCount); err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if ruleCount != len(SystemRules) {
		t.Errorf("rule count = %d, want %d (one per SystemRuleSpec)", ruleCount, len(SystemRules))
	}
}

func TestEnsureSystemArtifacts_NewSpec_BackfillsExistingUser(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	// Simulate a pre-Phase-4b user: no system artefacts, version stays at 0.
	// (Tags table protects system tags from delete via DB trigger from
	// migration 000170, so we cannot simulate "tag missing" — but the realistic
	// new-spec backfill scenario is "rule missing, tag may or may not be
	// present", which is what we test here.)
	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Now simulate a brand-new spec entry having been added since the user
	// was last seeded: drop one rule and reset the user's version, then re-run.
	if _, err := pool.Exec(ctx,
		`DELETE FROM automation_rules WHERE user_id = $1 AND system_type = $2`,
		userID, SystemRules[0].SystemType,
	); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET system_artifacts_version = 0 WHERE id = $1`, userID,
	); err != nil {
		t.Fatalf("reset version: %v", err)
	}

	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var ruleExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = $1 AND system_type = $2 AND is_system = true)`,
		userID, SystemRules[0].SystemType,
	).Scan(&ruleExists); err != nil {
		t.Fatalf("check rule: %v", err)
	}
	if !ruleExists {
		t.Error("expected the dropped rule to be re-created by backfill")
	}

	// Version must have been bumped to current.
	var version int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, userID,
	).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != SystemArtifactsVersion {
		t.Errorf("version after backfill = %d, want %d", version, SystemArtifactsVersion)
	}
}

func TestEnsureSystemArtifacts_RespectsUserDeactivation(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Pick the first rule, deactivate it as the user would.
	systemType := SystemRules[0].SystemType
	if _, err := pool.Exec(ctx,
		`UPDATE automation_rules SET is_active = false
		   WHERE user_id = $1 AND system_type = $2 AND is_system = true`,
		userID, systemType,
	); err != nil {
		t.Fatalf("deactivate rule: %v", err)
	}

	// Re-run EnsureSystemArtifacts — must NOT flip the user's choice back to true.
	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("re-run: %v", err)
	}

	var isActive bool
	if err := pool.QueryRow(ctx,
		`SELECT is_active FROM automation_rules
		   WHERE user_id = $1 AND system_type = $2 AND is_system = true`,
		userID, systemType,
	).Scan(&isActive); err != nil {
		t.Fatalf("read is_active: %v", err)
	}
	if isActive {
		t.Errorf("expected user-deactivated rule to remain is_active=false; got true")
	}
}

func TestSystemTypeWhitelist_RejectsUnknown(t *testing.T) {
	if IsValidSystemType("foobar") {
		t.Error("IsValidSystemType(\"foobar\") = true, want false")
	}
	if IsValidSystemType("") {
		t.Error("IsValidSystemType(\"\") = true, want false")
	}
	for _, r := range SystemRules {
		if !IsValidSystemType(r.SystemType) {
			t.Errorf("SystemRule %q must be a valid system type", r.SystemType)
		}
	}
	for _, t2 := range OnDemandSystemTypes {
		if !IsValidSystemType(t2) {
			t.Errorf("OnDemandSystemType %q must be a valid system type", t2)
		}
	}
}

func TestSystemRules_TriggerEventInWhitelist(t *testing.T) {
	allowed := map[string]bool{
		"on_import":              true,
		"on_enrichment_complete": true,
		"on_manual_apply":        true,
	}
	for _, r := range SystemRules {
		if !allowed[r.TriggerEvent] {
			t.Errorf("SystemRule %q has TriggerEvent %q — must be one of {on_import, on_enrichment_complete, on_manual_apply}",
				r.SystemType, r.TriggerEvent)
		}
	}
}

func TestEnsureSystemArtifacts_TagBeforeRule(t *testing.T) {
	// Every rule's TagName must exist in SystemTags. Otherwise the tag-id
	// lookup at insert time would fail with pgx.ErrNoRows.
	tagNames := map[string]bool{}
	for _, t2 := range SystemTags {
		tagNames[t2.Name] = true
	}
	for _, r := range SystemRules {
		if !tagNames[r.TagName] {
			t.Errorf("SystemRule %q references TagName %q which is not in SystemTags",
				r.SystemType, r.TagName)
		}
	}
}

func TestEnsureSystemArtifacts_PromotesExistingCustomTagNameCollision(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	if _, err := pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto)
		VALUES ($1, 'Lift', '#000000', '#FFFFFF', 'custom', false, false)
	`, userID); err != nil {
		t.Fatalf("insert custom Lift tag: %v", err)
	}

	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("EnsureSystemArtifacts: %v", err)
	}

	var isSystem bool
	var category string
	var ruleCount int
	if err := pool.QueryRow(ctx, `
		SELECT is_system, category
		FROM tags
		WHERE user_id = $1 AND name = 'Lift'
	`, userID).Scan(&isSystem, &category); err != nil {
		t.Fatalf("query Lift tag: %v", err)
	}
	if !isSystem || category != "system" {
		t.Fatalf("Lift collision was not promoted to system tag: is_system=%v category=%q", isSystem, category)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM automation_rules
		WHERE user_id = $1 AND is_system = true AND system_type = 'lift_detected'
	`, userID).Scan(&ruleCount); err != nil {
		t.Fatalf("query lift rule count: %v", err)
	}
	if ruleCount != 1 {
		t.Fatalf("lift_detected rule count = %d, want 1", ruleCount)
	}
}

func TestEnsureSystemArtifacts_SetsHasAutomationForSeededRuleTags(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("EnsureSystemArtifacts: %v", err)
	}

	var hasAutomation bool
	if err := pool.QueryRow(ctx, `
		SELECT has_automation
		FROM tags
		WHERE user_id = $1 AND name = 'Lift'
	`, userID).Scan(&hasAutomation); err != nil {
		t.Fatalf("query Lift tag has_automation: %v", err)
	}
	if !hasAutomation {
		t.Fatal("Lift tag has_automation=false after seeding lift_detected rule")
	}
}

func TestEnsureSystemArtifacts_VersionGate_Skips_Up_To_Date(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	// Mark the user as up-to-date BEFORE seeding any artefact.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET system_artifacts_version = $1 WHERE id = $2`,
		SystemArtifactsVersion, userID,
	); err != nil {
		t.Fatalf("set version: %v", err)
	}

	if err := EnsureSystemArtifactsForAllUsers(ctx, db); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	// Sweep must NOT have inserted artefacts for the up-to-date user.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tags WHERE user_id = $1 AND is_system = true`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tags for skipped user, got %d", count)
	}
}

func TestEnsureSystemArtifacts_VersionGate_Marks_User_After_Run(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	if err := EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	var got int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, userID,
	).Scan(&got); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if got != SystemArtifactsVersion {
		t.Errorf("system_artifacts_version = %d, want %d", got, SystemArtifactsVersion)
	}
}

func TestEnsureSystemArtifactsForAllUsers_AdvisoryLock(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	// Hold the sweep advisory lock from outside the function.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, sweepAdvisoryLockKey).Scan(&locked); err != nil {
		t.Fatalf("take lock: %v", err)
	}
	if !locked {
		t.Skip("could not acquire sweep advisory lock — concurrent test holds it")
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, sweepAdvisoryLockKey)

	// While the lock is held, parallel calls must return nil without work.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = EnsureSystemArtifactsForAllUsers(ctx, db)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: expected nil while lock held, got %v", i, err)
		}
	}
}

func TestEnsureSystemArtifactsForAllUsers_DoesNotRetrySameFailingUserForever(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	failUserID := createTestUser(t, pool)
	okUserID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	suffix := failUserID[:8]
	triggerName := "test_fail_system_artifacts_tags_" + suffix
	functionName := "test_fail_system_artifacts_tags_fn_" + suffix
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.user_id = %s::uuid THEN
				RAISE EXCEPTION 'intentional system artifact test failure';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER %s
		BEFORE INSERT ON tags
		FOR EACH ROW EXECUTE FUNCTION %s();
	`, functionName, quoteLiteral(failUserID), triggerName, functionName)); err != nil {
		t.Fatalf("install failing trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON tags`, triggerName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})

	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := EnsureSystemArtifactsForAllUsers(runCtx, db); err != nil {
		t.Fatalf("sweep should finish one pass despite a per-user failure, got: %v", err)
	}

	var failVersion, okVersion int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, failUserID,
	).Scan(&failVersion); err != nil {
		t.Fatalf("read fail user version: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, okUserID,
	).Scan(&okVersion); err != nil {
		t.Fatalf("read ok user version: %v", err)
	}
	if failVersion != 0 {
		t.Fatalf("failing user version = %d, want 0 for retry on a future sweep", failVersion)
	}
	if okVersion != SystemArtifactsVersion {
		t.Fatalf("ok user version = %d, want %d", okVersion, SystemArtifactsVersion)
	}
}

// helpers ---------------------------------------------------------------

func namesFromTagSpecs(specs []SystemTagSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}

func systemTypesFromRuleSpecs(specs []SystemRuleSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.SystemType
	}
	return out
}

func quoteLiteral(s string) string {
	return "'" + s + "'"
}
