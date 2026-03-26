package shadowfs_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func TestLocalBackend_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	content := []byte("hello world")
	written, hash, err := be.Write(ctx, "sub/test.txt", bytes.NewReader(content), "text/plain", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len(content)) {
		t.Errorf("written = %d, want %d", written, len(content))
	}
	expectedHash := sha256.Sum256(content)
	if hash != hex.EncodeToString(expectedHash[:]) {
		t.Errorf("hash = %s, want %s", hash, hex.EncodeToString(expectedHash[:]))
	}

	// Read back
	rc, err := be.Read(ctx, "sub/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, content) {
		t.Errorf("read content mismatch")
	}
}

func TestLocalBackend_WriteExclusive(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	be.Write(ctx, "excl.txt", bytes.NewReader([]byte("first")), "", true, nil)
	_, _, err := be.Write(ctx, "excl.txt", bytes.NewReader([]byte("second")), "", true, nil)
	if err == nil {
		t.Error("expected O_EXCL error on second write")
	}
}

func TestLocalBackend_Exists(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	exists, _ := be.Exists(ctx, "nope.txt", nil)
	if exists {
		t.Error("should not exist")
	}

	be.Write(ctx, "yes.txt", bytes.NewReader([]byte("x")), "", false, nil)
	exists, _ = be.Exists(ctx, "yes.txt", nil)
	if !exists {
		t.Error("should exist")
	}
}

func TestLocalBackend_Delete(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	be.Write(ctx, "del.txt", bytes.NewReader([]byte("x")), "", false, nil)
	if err := be.Delete(ctx, "del.txt", nil); err != nil {
		t.Fatal(err)
	}
	exists, _ := be.Exists(ctx, "del.txt", nil)
	if exists {
		t.Error("should be deleted")
	}
}

func TestLocalBackend_DeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	// Should NOT error on non-existent file
	if err := be.Delete(ctx, "ghost.txt", nil); err != nil {
		t.Errorf("delete non-existent should not error: %v", err)
	}
}

func TestLocalBackend_ListFunc(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	be.Write(ctx, "a/1.txt", bytes.NewReader([]byte("1")), "", false, nil)
	be.Write(ctx, "a/2.txt", bytes.NewReader([]byte("2")), "", false, nil)
	be.Write(ctx, "b/3.txt", bytes.NewReader([]byte("3")), "", false, nil)

	var paths []string
	be.ListFunc(ctx, "a/", nil, func(path string) error {
		paths = append(paths, path)
		return nil
	})
	if len(paths) != 2 {
		t.Errorf("expected 2 paths under a/, got %d", len(paths))
	}
}

func TestLocalBackend_DeletePrefix(t *testing.T) {
	dir := t.TempDir()
	be := shadowfs.NewLocalBackend(dir)
	ctx := context.Background()

	be.Write(ctx, "pfx/a.txt", bytes.NewReader([]byte("a")), "", false, nil)
	be.Write(ctx, "pfx/b.txt", bytes.NewReader([]byte("b")), "", false, nil)
	be.Write(ctx, "other/c.txt", bytes.NewReader([]byte("c")), "", false, nil)

	be.DeletePrefix(ctx, "pfx/", nil)

	exists, _ := be.Exists(ctx, "pfx/a.txt", nil)
	if exists {
		t.Error("pfx/a.txt should be deleted")
	}
	exists, _ = be.Exists(ctx, "other/c.txt", nil)
	if !exists {
		t.Error("other/c.txt should still exist")
	}
}
