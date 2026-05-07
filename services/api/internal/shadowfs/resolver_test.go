// services/api/internal/shadowfs/resolver_test.go
package shadowfs

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolverTestPool bootstraps a pgxpool from TEST_DATABASE_URL (consistent with
// handler/testhelpers_test.go and settings/testhelpers_test.go conventions).
// Skip the test if no DB is available.
func resolverTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// resolverTestUser inserts a throwaway user and returns its ID.
// Matches what handler/testhelpers_test.go:insertTestUser does, but inline
// because that helper is private to the handler package.
func resolverTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, is_active, email_verified, created_at)
		 VALUES ($1, $2, 'testhash', true, true, NOW())`,
		userID, "resolver-test-"+userID[:8])
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

// resolverTestSettings inits a SettingsStore against the test pool.
// Bypass: tests write to server_settings via direct SQL and then call
// store.Reload (no ENV-override handling needed in tests).
func resolverTestSettings(t *testing.T, pool *pgxpool.Pool) *settings.SettingsStore {
	t.Helper()
	return settings.NewSettingsStore(&database.DB{Pool: pool}, "", "local")
}

func resolverSetSetting(t *testing.T, pool *pgxpool.Pool, key, value string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value)
	if err != nil {
		t.Fatalf("setSetting %q: %v", key, err)
	}
}

func TestResolveUserSchemaUserOverrideEditable(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)

	_, _ = pool.Exec(ctx,
		`INSERT INTO user_file_schemas (user_id, file_type, schema_template)
		 VALUES ($1, 'activity', '{year}/{title}-custom')`, userID)

	resolverSetSetting(t, pool, "library.user_can_edit_schemas", "true")
	resolverSetSetting(t, pool, "library.default_schema.activity", "{year}/{month}_default")
	store := resolverTestSettings(t, pool)
	_ = store.Reload(ctx)

	got, err := ResolveUserSchema(ctx, pool, store, NewSchemaRenderer(), userID, "activity")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "{year}/{title}-custom" {
		t.Errorf("got %q, want user override", got)
	}
}

func TestResolveUserSchemaLockdownOverridesUserValue(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)

	_, _ = pool.Exec(ctx,
		`INSERT INTO user_file_schemas (user_id, file_type, schema_template)
		 VALUES ($1, 'activity', '{year}/{title}-custom')`, userID)

	resolverSetSetting(t, pool, "library.user_can_edit_schemas", "false")
	resolverSetSetting(t, pool, "library.default_schema.activity", "{year}/{month}_admin")
	store := resolverTestSettings(t, pool)
	_ = store.Reload(ctx)

	got, err := ResolveUserSchema(ctx, pool, store, NewSchemaRenderer(), userID, "activity")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "{year}/{month}_admin" {
		t.Errorf("got %q, want admin default (lockdown)", got)
	}
}

func TestResolveUserSchemaNoOverrideFallsBackToGlobalDefault(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)

	resolverSetSetting(t, pool, "library.user_can_edit_schemas", "true")
	resolverSetSetting(t, pool, "library.default_schema.activity", "{year}/{month}_global")
	store := resolverTestSettings(t, pool)
	_ = store.Reload(ctx)

	got, _ := ResolveUserSchema(ctx, pool, store, NewSchemaRenderer(), userID, "activity")
	if got != "{year}/{month}_global" {
		t.Errorf("got %q, want global default", got)
	}
}

func TestResolveUserSchemaNoGlobalFallsBackToRenderer(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)

	// Remove any pre-seeded default, clear user_can_edit explicitly set
	_, _ = pool.Exec(ctx, `DELETE FROM server_settings WHERE key = 'library.default_schema.activity'`)
	resolverSetSetting(t, pool, "library.user_can_edit_schemas", "true")
	store := resolverTestSettings(t, pool)
	_ = store.Reload(ctx)

	renderer := NewSchemaRenderer()
	got, _ := ResolveUserSchema(ctx, pool, store, renderer, userID, "activity")
	if got != renderer.DefaultSchema("activity") {
		t.Errorf("got %q, want Renderer built-in", got)
	}
}

func TestResolveLibraryDirNameOverridePresent(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)

	_, _ = pool.Exec(ctx, `UPDATE users SET library_dir_name = 'my-lib' WHERE id = $1`, userID)

	got, _ := ResolveLibraryDirName(ctx, pool, userID)
	if got != "my-lib" {
		t.Errorf("got %q, want my-lib", got)
	}
}

func TestResolveLibraryDirNameFallbackToUUID(t *testing.T) {
	pool := resolverTestPool(t)
	ctx := context.Background()
	userID := resolverTestUser(t, pool)
	// library_dir_name stays NULL

	got, _ := ResolveLibraryDirName(ctx, pool, userID)
	if got != userID {
		t.Errorf("got %q, want userID %q", got, userID)
	}
}
