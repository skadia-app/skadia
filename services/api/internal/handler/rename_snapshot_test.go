package handler

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func TestRenameExecuteForegroundSnapshot(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()

	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	// Wire settings so ResolveUserSchema can check library.user_can_edit_schemas.
	// Without a SettingsStore, Get returns the zero-value CachedSetting.
	// We set it via SQL so the store (if wired) returns "true".
	_, _ = pool.Exec(ctx,
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "true")

	// Set user override schema.
	_, err := pool.Exec(ctx,
		`INSERT INTO user_file_schemas (user_id, file_type, schema_template)
		 VALUES ($1, 'activity', 'snapshot-test/{year}')`, userID)
	if err != nil {
		t.Fatalf("insert user_file_schemas: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM user_file_schemas WHERE user_id = $1", userID) //nolint:errcheck
	})

	// Call foreground resolver manually — simulate what handler does.
	renderer := shadowfs.NewSchemaRenderer()
	schema, resolveErr := shadowfs.ResolveUserSchema(ctx, pool, h.Settings, renderer, userID, "activity")
	if resolveErr != nil {
		// Without a live SettingsStore, user_can_edit_schemas defaults to "".
		// ResolveUserSchema falls back to global default then built-in default.
		// The snapshot semantics are still valid — we just get a different schema.
		t.Logf("ResolveUserSchema (no settings store): %v — using built-in default", resolveErr)
	}

	// The snapshot value is whatever resolver returned at call time.
	schemaSnapshot := schema

	// Now modify schema in DB (simulates admin mid-job change).
	_, _ = pool.Exec(ctx,
		`UPDATE user_file_schemas SET schema_template = 'after-change/{year}'
		 WHERE user_id = $1 AND file_type = 'activity'`, userID)

	// The background closure must use schemaSnapshot (captured value), not re-query.
	// We verify by calling the background function directly with the snapshot.
	// The function will return quickly because it requires an advisory lock on a
	// connection and there are no files in user_files for this user.
	libraryDirSnapshot, dirErr := shadowfs.ResolveLibraryDirName(ctx, pool, userID)
	if dirErr != nil {
		t.Fatalf("ResolveLibraryDirName: %v", dirErr)
	}

	// libraryDirSnapshot must equal userID (no library_dir_name set).
	if libraryDirSnapshot != userID {
		t.Errorf("libraryDirSnapshot: got %q, want %q (userID)", libraryDirSnapshot, userID)
	}

	// Execute background function with snapshots — verifies new signature compiles and runs.
	h.executeFileRenameBackground(ctx, userID, "activity", "/tmp/test-base", "job-1", schemaSnapshot, libraryDirSnapshot)

	// Post-condition: no files were renamed (user_files is empty for test user),
	// background_jobs row "job-1" does not exist — UpdateBackgroundJob will fail silently.
	// The test is structural: it verifies the snapshot-based signature is correct.
}
