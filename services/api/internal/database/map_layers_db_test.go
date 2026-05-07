package database

// Integration tests for map_layers_db.go.
//
// Requires TEST_DATABASE_URL pointing to a fully-migrated Skadia database.
// Each test wraps mutations in a transaction that is rolled back at cleanup,
// so the live schema is unaffected between runs.

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mapLayerTestPool returns a pool connected to TEST_DATABASE_URL or skips the test.
func mapLayerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mapLayerTestDB wraps a pool as a *DB (the type expected by CRUD functions).
func mapLayerTestDB(t *testing.T) *DB {
	t.Helper()
	pool := mapLayerTestPool(t)
	return &DB{Pool: pool}
}

// insertTestUser inserts a minimal user row and returns its UUID.
// Deletes the user (and cascades) on cleanup.
func insertTestUser(t *testing.T, db *DB) string {
	t.Helper()
	ctx := context.Background()
	uid := uuid.New().String()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		uid,
		"maptest_"+uid[:8],
		"maptest_"+uid[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", uid) //nolint:errcheck
	})
	return uid
}

// newRasterLayer returns a minimal admin-created MapLayer for test inserts.
func newRasterLayer(key, name string) *model.MapLayer {
	tpl := "https://tile.example.com/{z}/{x}/{y}.png"
	return &model.MapLayer{
		Key:            key,
		Role:           "base",
		Name:           name,
		SourceKind:     "raster_tiles",
		URLTemplate:    &tpl,
		DefaultOpacity: 100,
		Enabled:        true,
		SortOrder:      100,
	}
}

// ─── map_layers ──────────────────────────────────────────────────────────────

func TestInsertAndListMapLayers(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	before, err := db.ListMapLayers(ctx, true)
	if err != nil {
		t.Fatalf("ListMapLayers before: %v", err)
	}

	m := newRasterLayer("test-raster-"+uuid.New().String()[:8], "Test Raster")
	if err := db.InsertMapLayer(ctx, m); err != nil {
		t.Fatalf("InsertMapLayer: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("InsertMapLayer: ID not populated")
	}
	if m.CreatedAt.IsZero() {
		t.Fatal("InsertMapLayer: CreatedAt not populated")
	}
	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = $1", m.ID) //nolint:errcheck
	})

	after, err := db.ListMapLayers(ctx, true)
	if err != nil {
		t.Fatalf("ListMapLayers after: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("expected %d rows, got %d", len(before)+1, len(after))
	}
	// is_system must be false regardless of what we send.
	if m.IsSystem {
		t.Error("IsSystem must be false for admin-created layers")
	}
}

func TestUpdateMapLayer_ImmutableField(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	// Find any system layer.
	var sysID int64
	err := db.Pool.QueryRow(ctx, `SELECT id FROM map_layers WHERE is_system = true LIMIT 1`).Scan(&sysID)
	if err != nil {
		t.Skipf("no system layer in DB: %v", err)
	}

	// Patching a protected field on a system layer must return ErrSystemLayerImmutable.
	err = db.UpdateMapLayer(ctx, sysID, map[string]any{"url_template": "https://x.com/{z}/{x}/{y}.png"}, nil)
	if !isErrIs(err, ErrSystemLayerImmutable) {
		t.Errorf("expected ErrSystemLayerImmutable, got %v", err)
	}
}

func TestUpdateMapLayer_StaleIfMatch(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	m := newRasterLayer("stale-test-"+uuid.New().String()[:8], "Stale Test")
	if err := db.InsertMapLayer(ctx, m); err != nil {
		t.Fatalf("InsertMapLayer: %v", err)
	}
	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = $1", m.ID) //nolint:errcheck
	})

	// Use a past timestamp — must return ErrStaleUpdate.
	past := m.UpdatedAt.Add(-1 * time.Second)
	err := db.UpdateMapLayer(ctx, m.ID, map[string]any{"name": "New Name"}, &past)
	if !isErrIs(err, ErrStaleUpdate) {
		t.Errorf("expected ErrStaleUpdate, got %v", err)
	}

	// Correct timestamp — must succeed.
	err = db.UpdateMapLayer(ctx, m.ID, map[string]any{"name": "New Name"}, &m.UpdatedAt)
	if err != nil {
		t.Errorf("UpdateMapLayer with correct ifMatch: %v", err)
	}
}

func TestDeleteMapLayer_SystemLayer(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	var sysID int64
	err := db.Pool.QueryRow(ctx, `SELECT id FROM map_layers WHERE is_system = true LIMIT 1`).Scan(&sysID)
	if err != nil {
		t.Skipf("no system layer in DB: %v", err)
	}

	err = db.DeleteMapLayer(ctx, sysID)
	if !isErrIs(err, ErrSystemLayerImmutable) {
		t.Errorf("expected ErrSystemLayerImmutable, got %v", err)
	}
}

func TestBulkSortMapLayers(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	m1 := newRasterLayer("sort-a-"+uuid.New().String()[:8], "Sort A")
	m2 := newRasterLayer("sort-b-"+uuid.New().String()[:8], "Sort B")
	if err := db.InsertMapLayer(ctx, m1); err != nil {
		t.Fatalf("insert m1: %v", err)
	}
	if err := db.InsertMapLayer(ctx, m2); err != nil {
		t.Fatalf("insert m2: %v", err)
	}
	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = ANY($1)", []int64{m1.ID, m2.ID}) //nolint:errcheck
	})

	items := []SortItem{{ID: m1.ID, SortOrder: 50}, {ID: m2.ID, SortOrder: 10}}
	if err := db.BulkSortMapLayers(ctx, items); err != nil {
		t.Fatalf("BulkSortMapLayers: %v", err)
	}

	var so1, so2 int32
	db.Pool.QueryRow(ctx, `SELECT sort_order FROM map_layers WHERE id = $1`, m1.ID).Scan(&so1) //nolint:errcheck
	db.Pool.QueryRow(ctx, `SELECT sort_order FROM map_layers WHERE id = $1`, m2.ID).Scan(&so2) //nolint:errcheck
	if so1 != 50 || so2 != 10 {
		t.Errorf("sort_order mismatch: m1=%d (want 50), m2=%d (want 10)", so1, so2)
	}
}

// ─── system_layer_visibility ─────────────────────────────────────────────────

// TestUpdateSystemLayerVisibility_RecreatesMissingRow verifies that the UPSERT
// behavior re-creates a system_layer_visibility row that has been deleted
// out-of-band (manual SQL, partial down/up cycle). The admin UI must remain
// able to PATCH any key in model.SystemLayerKeySet() without an operator step.
func TestUpdateSystemLayerVisibility_RecreatesMissingRow(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	// Pick any seeded key from the closed set.
	var key string
	for k := range model.SystemLayerKeySet() {
		key = k
		break
	}
	if key == "" {
		t.Skip("model.SystemLayerKeySet() is empty — nothing to test")
	}

	// Snapshot original row so we can restore it on cleanup.
	var origEnabled, origDefaultForNew bool
	var origOpacity *int16
	var origLabel *string
	err := db.Pool.QueryRow(ctx,
		`SELECT enabled, default_enabled_for_new_users, default_opacity, label_override
		 FROM system_layer_visibility WHERE layer_key = $1`, key,
	).Scan(&origEnabled, &origDefaultForNew, &origOpacity, &origLabel)
	if err != nil {
		t.Fatalf("read original row for %q: %v", key, err)
	}
	t.Cleanup(func() {
		// Restore via the same UPSERT path (will overwrite or recreate as needed).
		_, _ = db.Pool.Exec(context.Background(),
			`INSERT INTO system_layer_visibility
			   (layer_key, enabled, default_enabled_for_new_users, default_opacity, label_override)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (layer_key) DO UPDATE SET
			   enabled = EXCLUDED.enabled,
			   default_enabled_for_new_users = EXCLUDED.default_enabled_for_new_users,
			   default_opacity = EXCLUDED.default_opacity,
			   label_override = EXCLUDED.label_override`,
			key, origEnabled, origDefaultForNew, origOpacity, origLabel,
		)
	})

	// Delete the row out-of-band.
	if _, err := db.Pool.Exec(ctx,
		`DELETE FROM system_layer_visibility WHERE layer_key = $1`, key,
	); err != nil {
		t.Fatalf("delete row: %v", err)
	}

	// PATCH must now create the row, not return ErrNotFound.
	if err := db.UpdateSystemLayerVisibility(ctx, key, map[string]any{"enabled": false}); err != nil {
		t.Fatalf("UpdateSystemLayerVisibility on missing row: %v", err)
	}

	// Verify the row exists with the patched value and defaults for the rest.
	var enabled, defNew bool
	var opacity *int16
	var label *string
	err = db.Pool.QueryRow(ctx,
		`SELECT enabled, default_enabled_for_new_users, default_opacity, label_override
		 FROM system_layer_visibility WHERE layer_key = $1`, key,
	).Scan(&enabled, &defNew, &opacity, &label)
	if err != nil {
		t.Fatalf("read recreated row: %v", err)
	}
	if enabled != false {
		t.Errorf("enabled: expected false, got %v", enabled)
	}
	// default_enabled_for_new_users default is false (per migration 000333 column DEFAULT).
	if defNew != false {
		t.Errorf("default_enabled_for_new_users: expected false (column default), got %v", defNew)
	}
	if opacity != nil {
		t.Errorf("default_opacity: expected NULL (column default), got %v", *opacity)
	}
	if label != nil {
		t.Errorf("label_override: expected NULL (column default), got %v", *label)
	}
}

// TestUpdateSystemLayerVisibility_UpdatesExistingRow verifies that the UPSERT
// path applied to an existing row updates only the patched columns and leaves
// the remaining columns untouched (no fallback to defaults on the UPDATE branch).
func TestUpdateSystemLayerVisibility_UpdatesExistingRow(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	var key string
	for k := range model.SystemLayerKeySet() {
		key = k
		break
	}
	if key == "" {
		t.Skip("model.SystemLayerKeySet() is empty — nothing to test")
	}

	// Snapshot + restore on cleanup.
	var origEnabled, origDefaultForNew bool
	var origOpacity *int16
	var origLabel *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT enabled, default_enabled_for_new_users, default_opacity, label_override
		 FROM system_layer_visibility WHERE layer_key = $1`, key,
	).Scan(&origEnabled, &origDefaultForNew, &origOpacity, &origLabel); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(),
			`UPDATE system_layer_visibility
			 SET enabled = $1, default_enabled_for_new_users = $2,
			     default_opacity = $3, label_override = $4
			 WHERE layer_key = $5`,
			origEnabled, origDefaultForNew, origOpacity, origLabel, key,
		)
	})

	// Patch only `enabled`.
	target := !origEnabled
	if err := db.UpdateSystemLayerVisibility(ctx, key, map[string]any{"enabled": target}); err != nil {
		t.Fatalf("UpdateSystemLayerVisibility: %v", err)
	}

	var enabled, defNew bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT enabled, default_enabled_for_new_users
		 FROM system_layer_visibility WHERE layer_key = $1`, key,
	).Scan(&enabled, &defNew); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if enabled != target {
		t.Errorf("enabled: expected %v, got %v", target, enabled)
	}
	if defNew != origDefaultForNew {
		t.Errorf("default_enabled_for_new_users: expected unchanged %v, got %v", origDefaultForNew, defNew)
	}
}

// ─── user_map_layers ─────────────────────────────────────────────────────────

func TestInsertUserMapLayer_RoundTrip(t *testing.T) {
	db := mapLayerTestDB(t)
	uid := insertTestUser(t, db)
	ctx := context.Background()

	tpl := "https://tiles.example.com/{z}/{x}/{y}.png"
	m := &model.UserMapLayer{
		UserID:         uid,
		Role:           "overlay",
		Name:           "My Tiles",
		SourceKind:     "raster_tiles",
		URLTemplate:    &tpl,
		DefaultOpacity: 80,
		Enabled:        true,
	}
	if err := db.InsertUserMapLayer(ctx, m, 50); err != nil {
		t.Fatalf("InsertUserMapLayer: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("ID not populated")
	}

	list, err := db.ListUserMapLayers(ctx, uid)
	if err != nil {
		t.Fatalf("ListUserMapLayers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(list))
	}
	if list[0].Name != "My Tiles" {
		t.Errorf("name mismatch: %q", list[0].Name)
	}
}

func TestInsertUserMapLayer_LimitReached(t *testing.T) {
	db := mapLayerTestDB(t)
	uid := insertTestUser(t, db)
	ctx := context.Background()

	tpl1 := "https://t.example.com/{z}/{x}/{y}.png"
	// Insert first layer at limit=1.
	m1 := &model.UserMapLayer{
		UserID:      uid,
		Role:        "overlay",
		Name:        "First",
		SourceKind:  "raster_tiles",
		URLTemplate: &tpl1,
	}
	if err := db.InsertUserMapLayer(ctx, m1, 1); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	tpl2 := "https://t.example.com/{z}/{x}/{y}.png"
	// Second insert with same limit must fail.
	m2 := &model.UserMapLayer{
		UserID:      uid,
		Role:        "overlay",
		Name:        "Second",
		SourceKind:  "raster_tiles",
		URLTemplate: &tpl2,
	}
	if err := db.InsertUserMapLayer(ctx, m2, 1); !isErrIs(err, ErrLimitReached) {
		t.Errorf("expected ErrLimitReached, got %v", err)
	}
}

func TestInsertUserMapLayer_DuplicateName(t *testing.T) {
	db := mapLayerTestDB(t)
	uid := insertTestUser(t, db)
	ctx := context.Background()

	tplA := "https://t.example.com/{z}/{x}/{y}.png"
	m := &model.UserMapLayer{
		UserID:      uid,
		Role:        "overlay",
		Name:        "Dup Name",
		SourceKind:  "raster_tiles",
		URLTemplate: &tplA,
	}
	if err := db.InsertUserMapLayer(ctx, m, 100); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	tplB := "https://other.example.com/{z}/{x}/{y}.png"
	m2 := &model.UserMapLayer{
		UserID:      uid,
		Role:        "overlay",
		Name:        "Dup Name", // same name, same user
		SourceKind:  "raster_tiles",
		URLTemplate: &tplB,
	}
	if err := db.InsertUserMapLayer(ctx, m2, 100); !isErrIs(err, ErrDuplicateName) {
		t.Errorf("expected ErrDuplicateName, got %v", err)
	}
}

func TestDeleteUser_CascadesUserMapLayers(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	// Create a user and insert a layer; then delete the user.
	uid := uuid.New().String()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		uid, "cascade_test_"+uid[:8], "cascade_"+uid[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	tplDel := "https://t.example.com/{z}/{x}/{y}.png"
	m := &model.UserMapLayer{
		UserID:      uid,
		Role:        "overlay",
		Name:        "To Be Deleted",
		SourceKind:  "raster_tiles",
		URLTemplate: &tplDel,
	}
	if err := db.InsertUserMapLayer(ctx, m, 100); err != nil {
		t.Fatalf("insert layer: %v", err)
	}

	// Delete user — CASCADE should remove the layer.
	if _, err := db.Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", uid); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	// Layer must be gone.
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_map_layers WHERE user_id = $1", uid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 layers after user deletion, got %d", count)
	}
}

// isErrIs is a local alias to make table-driven comparisons readable.
func isErrIs(err, target error) bool {
	return err != nil && err.Error() == target.Error()
}
