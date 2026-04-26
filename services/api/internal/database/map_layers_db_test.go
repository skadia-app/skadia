package database

// Integration tests for map_layers_db.go.
//
// Requires TEST_DATABASE_URL pointing to a fully-migrated Skadia database.
// Each test wraps mutations in a transaction that is rolled back at cleanup,
// so the live schema is unaffected between runs.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mapLayerTestPool returns a pool connected to TEST_DATABASE_URL or skips the test.
func mapLayerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping map_layers integration test")
	}
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

// ─── user_map_layers ─────────────────────────────────────────────────────────

func TestInsertUserMapLayer_RoundTrip(t *testing.T) {
	db := mapLayerTestDB(t)
	uid := insertTestUser(t, db)
	ctx := context.Background()

	m := &model.UserMapLayer{
		UserID:         uid,
		Name:           "My Tiles",
		URLTemplate:    "https://tiles.example.com/{z}/{x}/{y}.png",
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

	// Insert first layer at limit=1.
	m1 := &model.UserMapLayer{
		UserID:      uid,
		Name:        "First",
		URLTemplate: "https://t.example.com/{z}/{x}/{y}.png",
	}
	if err := db.InsertUserMapLayer(ctx, m1, 1); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second insert with same limit must fail.
	m2 := &model.UserMapLayer{
		UserID:      uid,
		Name:        "Second",
		URLTemplate: "https://t.example.com/{z}/{x}/{y}.png",
	}
	if err := db.InsertUserMapLayer(ctx, m2, 1); !isErrIs(err, ErrLimitReached) {
		t.Errorf("expected ErrLimitReached, got %v", err)
	}
}

func TestInsertUserMapLayer_DuplicateName(t *testing.T) {
	db := mapLayerTestDB(t)
	uid := insertTestUser(t, db)
	ctx := context.Background()

	m := &model.UserMapLayer{
		UserID:      uid,
		Name:        "Dup Name",
		URLTemplate: "https://t.example.com/{z}/{x}/{y}.png",
	}
	if err := db.InsertUserMapLayer(ctx, m, 100); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	m2 := &model.UserMapLayer{
		UserID:      uid,
		Name:        "Dup Name", // same name, same user
		URLTemplate: "https://other.example.com/{z}/{x}/{y}.png",
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

	m := &model.UserMapLayer{
		UserID:      uid,
		Name:        "To Be Deleted",
		URLTemplate: "https://t.example.com/{z}/{x}/{y}.png",
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
