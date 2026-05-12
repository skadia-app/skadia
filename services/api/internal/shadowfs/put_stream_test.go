package shadowfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func createPutStreamTestUser(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	id := uuid.NewString()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, display_name, timezone)
		VALUES ($1, $2, $3, 'PutStream User', 'UTC')
	`, id, "putstream_"+id[:8], "putstream-"+id[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func putStreamActivityEntry(userID string, content []byte) FileEntry {
	return FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "restore.fit",
		SizeBytes:   int64(len(content)),
		FileHash:    sha256Hex(content),
		Tags:        map[string]string{"year": "2026", "month": "05", "day": "12", "time": "120000", "sport": "cycling"},
	}
}

func TestUnifiedShadowFSPutStreamLocalCommitsAfterStorageSuccess(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	userID := createPutStreamTestUser(t)
	content := []byte("streamed restore content")

	rec, err := fs.PutStream(ctx, putStreamActivityEntry(userID, content), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutStream: %v", err)
	}

	fullPath := filepath.Join(base, rec.StoragePath)
	if got, err := os.ReadFile(fullPath); err != nil {
		t.Fatalf("read stored file: %v", err)
	} else if !bytes.Equal(got, content) {
		t.Fatalf("stored content = %q, want %q", got, content)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_files WHERE id = $1 AND user_id = $2`, rec.ID, userID).Scan(&rows); err != nil {
		t.Fatalf("count user_files: %v", err)
	}
	if rows != 1 {
		t.Fatalf("user_files rows = %d, want 1", rows)
	}
	if rec.FileHash == nil || *rec.FileHash != sha256Hex(content) {
		t.Fatalf("record hash = %v, want %s", rec.FileHash, sha256Hex(content))
	}
}

func TestUnifiedShadowFSPutStreamLocalDeletesTempOnHashMismatch(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	userID := createPutStreamTestUser(t)
	content := []byte("restore content")
	entry := putStreamActivityEntry(userID, content)
	entry.FileHash = sha256Hex([]byte("different"))

	if _, err := fs.PutStream(ctx, entry, bytes.NewReader(content)); err == nil {
		t.Fatal("PutStream succeeded with mismatched expected hash")
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_files WHERE user_id = $1`, userID).Scan(&rows); err != nil {
		t.Fatalf("count user_files: %v", err)
	}
	if rows != 0 {
		t.Fatalf("user_files rows = %d, want 0", rows)
	}

	var files []string
	if err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk base: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("unexpected files left after hash mismatch: %v", files)
	}
}

type recordingStreamingBackend struct {
	writeCalled       bool
	writeStreamCalled bool
	lastPath          string
	lastContentType   string
	data              []byte
}

func (b *recordingStreamingBackend) Read(context.Context, string, *BackendOpts) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.data)), nil
}

func (b *recordingStreamingBackend) Write(context.Context, string, io.Reader, string, bool, *BackendOpts) (int64, string, error) {
	b.writeCalled = true
	return 0, "", errors.New("buffered Write must not be called")
}

func (b *recordingStreamingBackend) WriteStream(_ context.Context, path string, data io.Reader, contentType string, _ bool, expectedHash string, _ *BackendOpts) (int64, string, error) {
	b.writeStreamCalled = true
	b.lastPath = path
	b.lastContentType = contentType
	content, err := io.ReadAll(data)
	if err != nil {
		return 0, "", err
	}
	hash := sha256Hex(content)
	if expectedHash != "" && expectedHash != hash {
		return 0, "", errors.New("hash mismatch")
	}
	b.data = content
	return int64(len(content)), hash, nil
}

func (b *recordingStreamingBackend) Delete(context.Context, string, *BackendOpts) error { return nil }
func (b *recordingStreamingBackend) Exists(context.Context, string, *BackendOpts) (bool, error) {
	return false, nil
}
func (b *recordingStreamingBackend) DeletePrefix(context.Context, string, *BackendOpts) error {
	return nil
}
func (b *recordingStreamingBackend) ListFunc(context.Context, string, *BackendOpts, func(string) error) error {
	return nil
}

func TestUnifiedShadowFSPutStreamS3UsesMultipartNotPutBytesForLargeRestore(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	backend := &recordingStreamingBackend{}
	fs := NewUnifiedShadowFS(pool, nil, backend, "s3", t.TempDir(), NewSchemaRenderer(), nil, 1, 0, "library", "", "")
	userID := createPutStreamTestUser(t)
	content := []byte("large restore placeholder")

	rec, err := fs.PutStream(ctx, putStreamActivityEntry(userID, content), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutStream: %v", err)
	}
	if backend.writeCalled {
		t.Fatal("buffered Write was called for s3 restore")
	}
	if !backend.writeStreamCalled {
		t.Fatal("streaming backend was not used")
	}
	if rec.StorageBackend != "s3" {
		t.Fatalf("storage_backend = %q, want s3", rec.StorageBackend)
	}
	if backend.lastPath == "" || backend.lastContentType == "" {
		t.Fatalf("streaming write did not receive path/content type: path=%q contentType=%q", backend.lastPath, backend.lastContentType)
	}
}

func TestUnifiedShadowFSPutDelegatesToPutStream(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	base := t.TempDir()
	backend := &recordingStreamingBackend{}
	fs := NewUnifiedShadowFS(pool, backend, nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	userID := createPutStreamTestUser(t)
	content := []byte("small put content")
	entry := putStreamActivityEntry(userID, content)
	entry.FileHash = ""

	if _, err := fs.Put(ctx, entry, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if backend.writeCalled {
		t.Fatal("Put used buffered backend Write")
	}
	if !backend.writeStreamCalled {
		t.Fatal("Put did not delegate to streaming backend")
	}
}
