package shadowfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func activityRefreshTestPool(t *testing.T) *pgxpool.Pool {
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
	return pool
}

func TestRefreshActivityFilePathUsesCurrentActivityFieldsAndMovesSidecar(t *testing.T) {
	ctx := context.Background()
	pool := activityRefreshTestPool(t)
	basePath := t.TempDir()
	userID := insertActivityRefreshUser(t, pool)
	activityID := uuid.NewString()
	fileID := uuid.NewString()

	oldRel := filepath.Join(userID, DirForType("activity"), "2024", "old_cycling_electric.fit")
	oldFull := filepath.Join(basePath, oldRel)
	if err := os.MkdirAll(filepath.Dir(oldFull), 0o755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(oldFull, []byte("activity"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(oldFull+".meta", []byte("meta"), 0o644); err != nil {
		t.Fatalf("write old meta: %v", err)
	}

	collisionRel := filepath.Join(userID, DirForType("activity"), "2024", "Berlin_running_.fit")
	collisionFull := filepath.Join(basePath, collisionRel)
	if err := os.MkdirAll(filepath.Dir(collisionFull), 0o755); err != nil {
		t.Fatalf("mkdir collision: %v", err)
	}
	if err := os.WriteFile(collisionFull, []byte("other"), 0o644); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (
			id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
			started_at, title, start_city, distance_m, elapsed_time_s, elevation_gain_m,
			original_file_format, source, file_hash
		)
		VALUES ($1, $2, 'running', NULL, false, false, false, false,
		        '2024-03-15 14:30:00+00', 'Lunch Run', 'Berlin', 10000, 3600, 120,
		        'fit', 'upload', $3)`,
		activityID, userID, uuid.NewString()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_files (
			id, user_id, file_type, file_name, content_type, storage_backend,
			storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status
		)
		VALUES ($1, $2, 'activity', 'old.fit', 'application/octet-stream', 'local',
		        $3, 8, $4, 'activity', $5, true, 'active')`,
		fileID, userID, oldRel, uuid.NewString(), activityID); err != nil {
		t.Fatalf("insert user_file: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activities SET file_id = $1 WHERE id = $2`, fileID, activityID); err != nil {
		t.Fatalf("link file_id: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_files WHERE id = $1`, fileID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	store := settings.NewStoreForTest(map[string]string{
		"library.user_can_edit_schemas":   "false",
		"library.default_schema.activity": "{year}/{city}_{sport}_{modifiers}",
	})
	refreshed, err := RefreshActivityFilePath(ctx, ActivityFileRefreshRequest{
		Pool:       pool,
		Settings:   store,
		Renderer:   NewSchemaRenderer(),
		BasePath:   basePath,
		UserID:     userID,
		ActivityID: activityID,
	})
	if err != nil {
		t.Fatalf("RefreshActivityFilePath: %v", err)
	}
	if !refreshed {
		t.Fatal("RefreshActivityFilePath refreshed=false, want true")
	}

	wantRel := filepath.Join(userID, DirForType("activity"), "2024", "Berlin_running__1.fit")
	if _, err := os.Stat(filepath.Join(basePath, wantRel)); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(basePath, wantRel+".meta")); err != nil {
		t.Fatalf("renamed sidecar missing: %v", err)
	}
	if _, err := os.Stat(oldFull); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or stat failed differently: %v", err)
	}
	if _, err := os.Stat(oldFull + ".meta"); !os.IsNotExist(err) {
		t.Fatalf("old sidecar still exists or stat failed differently: %v", err)
	}

	var storagePath string
	if err := pool.QueryRow(ctx, `SELECT storage_path FROM user_files WHERE id = $1`, fileID).Scan(&storagePath); err != nil {
		t.Fatalf("query storage_path: %v", err)
	}
	if storagePath != wantRel {
		t.Fatalf("storage_path = %q, want %q", storagePath, wantRel)
	}
}

func TestRenderActivityFilePathIncludesCommuteAndRequiresFileOwner(t *testing.T) {
	ctx := context.Background()
	pool := activityRefreshTestPool(t)
	userID := insertActivityRefreshUser(t, pool)
	otherUserID := insertActivityRefreshUser(t, pool)
	activityID := uuid.NewString()
	fileID := uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (
			id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
			started_at, title, distance_m, elapsed_time_s, elevation_gain_m,
			original_file_format, source, file_hash
		)
		VALUES ($1, $2, 'cycling', '', false, false, false, true,
		        '2024-03-15 14:30:00+00', 'Commute', 10000, 3600, 120,
		        'fit', 'upload', $3)`,
		activityID, userID, uuid.NewString()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_files (
			id, user_id, file_type, file_name, content_type, storage_backend,
			storage_path, size_bytes, file_hash, parent_type, parent_id, status
		)
		VALUES ($1, $2, 'activity', 'commute.fit', 'application/octet-stream', 'local',
		        $3, 8, $4, 'activity', $5, 'active')`,
		fileID, otherUserID, filepath.Join(otherUserID, DirForType("activity"), "commute.fit"), uuid.NewString(), activityID); err != nil {
		t.Fatalf("insert other user_file: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activities SET file_id = $1 WHERE id = $2`, fileID, activityID); err != nil {
		t.Fatalf("link other file_id: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_files WHERE id = $1`, fileID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, userID, otherUserID)
	})

	rendered, err := RenderActivityFilePath(ctx, ActivityFileRefreshRequest{
		Pool:           pool,
		Renderer:       NewSchemaRenderer(),
		UserID:         userID,
		ActivityID:     activityID,
		SchemaOverride: "{sport}_{modifiers}",
	})
	if err != nil {
		t.Fatalf("RenderActivityFilePath with foreign file owner: %v", err)
	}
	if rendered.FileID != "" {
		t.Fatalf("RenderActivityFilePath returned foreign file_id %q, want no row", rendered.FileID)
	}

	if _, err := pool.Exec(ctx, `UPDATE user_files SET user_id = $1, storage_path = $2 WHERE id = $3`,
		userID, filepath.Join(userID, DirForType("activity"), "commute.fit"), fileID); err != nil {
		t.Fatalf("move file to activity owner: %v", err)
	}
	rendered, err = RenderActivityFilePath(ctx, ActivityFileRefreshRequest{
		Pool:           pool,
		Renderer:       NewSchemaRenderer(),
		UserID:         userID,
		ActivityID:     activityID,
		SchemaOverride: "{sport}_{modifiers}",
	})
	if err != nil {
		t.Fatalf("RenderActivityFilePath: %v", err)
	}
	wantRel := filepath.Join(userID, DirForType("activity"), "cycling_commute.fit")
	if rendered.NewPath != wantRel {
		t.Fatalf("NewPath = %q, want %q", rendered.NewPath, wantRel)
	}
}

func TestCopyActivityFileWithCollisionRetry(t *testing.T) {
	basePath := t.TempDir()
	oldRel := "old.fit"
	oldFull := filepath.Join(basePath, oldRel)
	if err := os.WriteFile(oldFull, []byte("activity"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePath, "target.fit"), []byte("existing"), 0o644); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	gotRel, err := copyActivityFileWithCollisionRetry(basePath, oldFull, "target.fit")
	if err != nil {
		t.Fatalf("copyActivityFileWithCollisionRetry: %v", err)
	}
	if gotRel != "target_1.fit" {
		t.Fatalf("got rel %q, want target_1.fit", gotRel)
	}
	got, err := os.ReadFile(filepath.Join(basePath, gotRel))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(got) != "activity" {
		t.Fatalf("copied content = %q, want activity", got)
	}
}

func insertActivityRefreshUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var userID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES (gen_random_uuid(), $1, $2, true, 'test') RETURNING id`,
		"shadow-refresh-"+uuid.NewString()[:8],
		"shadow-refresh-"+uuid.NewString()[:8]+"@example.com",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}
