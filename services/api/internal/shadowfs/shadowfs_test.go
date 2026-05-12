package shadowfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testUnifiedStore(t *testing.T, pool *pgxpool.Pool) (*UnifiedShadowFS, string) {
	t.Helper()
	dir := t.TempDir()
	localBE := NewLocalBackend(dir)
	fs := NewUnifiedShadowFS(pool, localBE, nil, "local", dir, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	return fs, dir
}

func createShadowFSTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "shadowfs_"+userID[:8], "shadowfs_"+userID+"@example.com",
	); err != nil {
		t.Fatalf("insert shadowfs test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func testQuotaEntry(userID, fileType, name, hash string, size int64) FileEntry {
	return FileEntry{
		UserID:      userID,
		FileType:    fileType,
		ContentType: "application/octet-stream",
		FileName:    name,
		SizeBytes:   size,
		FileHash:    hash,
		Tags: map[string]string{
			"year": "2026", "month": "05", "day": "12", "time": "1200", "sport": "cycling",
		},
	}
}

func setShadowFSSetting(t *testing.T, store *settings.SettingsStore, key, value string) {
	t.Helper()
	if _, err := store.Put(context.Background(), map[string]*string{key: &value}, ""); err != nil {
		t.Fatalf("set %s=%s: %v", key, value, err)
	}
}

func TestUnifiedShadowFS_PutGetDelete(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)

	fs, dir := testUnifiedStore(t, pool)

	var userID string
	if err := pool.QueryRow(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Skip("no users in DB")
	}

	entry := FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "test.fit",
		SizeBytes:   5,
		FileHash:    "abc123",
		Tags:        map[string]string{"year": "2024", "month": "03", "day": "15", "time": "1000", "sport": "cycling"},
	}
	data := bytes.NewReader([]byte("hello"))

	rec, err := fs.Put(ctx, entry, data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if rec.ID == "" {
		t.Fatal("Put returned empty ID")
	}
	if rec.StorageBackend != "local" {
		t.Errorf("backend = %q, want local", rec.StorageBackend)
	}

	fullPath := filepath.Join(dir, rec.StoragePath)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("file not on disk: %v", err)
	}

	rc, err := fs.Get(ctx, userID, rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "hello" {
		t.Errorf("Get data = %q, want %q", got, "hello")
	}

	if err := fs.Delete(ctx, userID, rec.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Error("file still on disk after Delete")
	}
}

func TestUnifiedShadowFSLiveQuotaUsesSettingsStore(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	dir := t.TempDir()
	localBE := NewLocalBackend(dir)
	store := settings.NewSettingsStore(&database.DB{Pool: pool}, "", "local")

	setShadowFSSetting(t, store, "limits.max_user_storage_bytes", "0")
	fs := NewUnifiedShadowFS(pool, localBE, nil, "local", dir, NewSchemaRenderer(), store, 1, 1, "", "", "")
	if _, err := fs.Put(ctx, testQuotaEntry(userID, "activity", "unlimited.fit", "quota-live-1", 5), bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatalf("Put with live 0 quota should succeed: %v", err)
	}

	setShadowFSSetting(t, store, "limits.max_user_storage_bytes", "1")
	_, err := fs.Put(ctx, testQuotaEntry(userID, "activity", "limited.fit", "quota-live-2", 5), bytes.NewReader([]byte("hello")))
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Put with live tight quota err=%v, want ErrQuotaExceeded", err)
	}
}

func TestUnifiedShadowFSQuotaErrorIsTyped(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	dir := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(dir), nil, "local", dir, NewSchemaRenderer(), nil, 1, 1, "", "", "")

	_, err := fs.Put(ctx, testQuotaEntry(userID, "activity", "too-big.fit", "quota-typed-1", 5), bytes.NewReader([]byte("hello")))
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Put err=%v, want ErrQuotaExceeded", err)
	}
}

func TestUnifiedShadowFSThumbPreviewStillBypassQuota(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	dir := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(dir), nil, "local", dir, NewSchemaRenderer(), nil, 1, 1, "", "", "")

	if _, err := fs.Put(ctx, testQuotaEntry(userID, "thumb", "thumb.jpg", "quota-thumb-1", 5), bytes.NewReader([]byte("thumb"))); err != nil {
		t.Fatalf("thumb Put should bypass quota: %v", err)
	}
	if _, err := fs.Put(ctx, testQuotaEntry(userID, "preview", "preview.jpg", "quota-preview-1", 7), bytes.NewReader([]byte("preview"))); err != nil {
		t.Fatalf("preview Put should bypass quota: %v", err)
	}
}
