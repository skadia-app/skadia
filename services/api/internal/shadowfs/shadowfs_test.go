package shadowfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testUnifiedStore(t *testing.T, pool *pgxpool.Pool) (*UnifiedShadowFS, string) {
	t.Helper()
	dir := t.TempDir()
	localBE := NewLocalBackend(dir)
	fs := NewUnifiedShadowFS(pool, localBE, nil, "local", dir, NewSchemaRenderer(), 1, 0, "", "", "")
	return fs, dir
}

func TestUnifiedShadowFS_PutGetDelete(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

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
		Tags:        map[string]string{"year": "2024", "month": "03", "day": "15", "time": "10-00", "sport": "cycling"},
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
