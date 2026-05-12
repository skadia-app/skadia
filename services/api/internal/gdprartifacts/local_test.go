package gdprartifacts

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLocalStoreCommitAbortStatOpenAndDelete(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newLocalStoreForTest(t)
	spec := localObjectSpec("export.zip")

	writer, err := store.CreateWriter(ctx, spec)
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if filepath.IsAbs(ref.Path) {
		t.Fatalf("ObjectRef.Path exposes absolute path: %q", ref.Path)
	}
	wantPath := filepath.ToSlash(filepath.Join("exports", spec.UserID, spec.JobID, spec.Name))
	if ref.Path != wantPath {
		t.Fatalf("ObjectRef.Path = %q, want %q", ref.Path, wantPath)
	}
	if ref.Backend != "local" || ref.Kind != spec.Kind || ref.Role != spec.Role || ref.Name != spec.Name {
		t.Fatalf("ObjectRef identity mismatch: %#v", ref)
	}
	if ref.SizeBytes != int64(len("hello world")) {
		t.Fatalf("SizeBytes = %d", ref.SizeBytes)
	}
	if ref.SHA256 != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatalf("SHA256 = %q", ref.SHA256)
	}

	stat, err := store.Stat(ctx, ref)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.State != ObjectPresent || stat.SizeBytes != ref.SizeBytes || stat.SHA256 != ref.SHA256 {
		t.Fatalf("Stat = %#v, want present size/hash", stat)
	}

	full, err := store.Open(ctx, ref, nil)
	if err != nil {
		t.Fatalf("Open(full): %v", err)
	}
	fullBytes, err := io.ReadAll(full)
	if closeErr := full.Close(); closeErr != nil {
		t.Fatalf("Close(full): %v", closeErr)
	}
	if err != nil {
		t.Fatalf("ReadAll(full): %v", err)
	}
	if string(fullBytes) != "hello world" {
		t.Fatalf("full read = %q", fullBytes)
	}
	if full.Size() != int64(len("hello world")) || full.ContentType() != spec.ContentType {
		t.Fatalf("full metadata size=%d content_type=%q", full.Size(), full.ContentType())
	}

	start, end := int64(6), int64(10)
	ranged, err := store.Open(ctx, ref, &RangeSpec{Start: &start, End: &end})
	if err != nil {
		t.Fatalf("Open(range): %v", err)
	}
	rangeBytes, err := io.ReadAll(ranged)
	if closeErr := ranged.Close(); closeErr != nil {
		t.Fatalf("Close(range): %v", closeErr)
	}
	if err != nil {
		t.Fatalf("ReadAll(range): %v", err)
	}
	if string(rangeBytes) != "world" {
		t.Fatalf("range read = %q", rangeBytes)
	}
	if ranged.Size() != 5 {
		t.Fatalf("range Size = %d, want 5", ranged.Size())
	}

	readerAt, err := store.OpenReaderAt(ctx, ref)
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	buf := make([]byte, 5)
	n, err := readerAt.ReadAt(buf, 6)
	if closeErr := readerAt.Close(); closeErr != nil {
		t.Fatalf("Close(readerAt): %v", closeErr)
	}
	if err != nil || n != 5 || string(buf) != "world" {
		t.Fatalf("ReadAt = n=%d buf=%q err=%v", n, buf, err)
	}
	if readerAt.Size() != int64(len("hello world")) {
		t.Fatalf("readerAt Size = %d", readerAt.Size())
	}

	if err := store.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stat, err = store.Stat(ctx, ref)
	if err != nil {
		t.Fatalf("Stat after delete: %v", err)
	}
	if stat.State != ObjectMissing {
		t.Fatalf("Stat after delete = %#v, want missing", stat)
	}
}

func TestLocalStoreAbortRemovesTempFile(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newLocalStoreForTest(t)

	writer, err := store.CreateWriter(ctx, localObjectSpec("abort.zip"))
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	localWriter, ok := writer.(*localObjectWriter)
	if !ok {
		t.Fatalf("writer type = %T, want *localObjectWriter", writer)
	}
	if _, err := writer.Write([]byte("partial")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Abort(ctx); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if _, err := os.Lstat(localWriter.tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file after Abort: err=%v", err)
	}
	if _, err := os.Lstat(localWriter.finalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final file after Abort: err=%v", err)
	}
}

func TestLocalStoreUsesPrivatePermissions(t *testing.T) {
	ctx := context.Background()
	store, exportsBase, _ := newLocalStoreForTest(t)
	spec := localObjectSpec("private.zip")

	writer, err := store.CreateWriter(ctx, spec)
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte("private")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	for _, path := range []string{
		exportsBase,
		filepath.Join(exportsBase, spec.UserID),
		filepath.Join(exportsBase, spec.UserID, spec.JobID),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%s): %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s mode = %o, want 0700", path, got)
		}
	}
	filePath := filepath.Join(exportsBase, spec.UserID, spec.JobID, spec.Name)
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat(file): %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 0600", got)
	}
	if ref.Path == filePath {
		t.Fatalf("ObjectRef.Path uses host path: %q", ref.Path)
	}
}

func TestLocalStoreRejectsPathTraversalAndSymlinkTraversal(t *testing.T) {
	ctx := context.Background()
	store, exportsBase, _ := newLocalStoreForTest(t)
	baseSpec := localObjectSpec("safe.zip")

	for _, name := range []string{"../escape.zip", "/absolute.zip", "nested/../../escape.zip"} {
		spec := baseSpec
		spec.Name = name
		if _, err := store.CreateWriter(ctx, spec); err == nil {
			t.Fatalf("CreateWriter accepted unsafe name %q", name)
		}
	}

	outside := t.TempDir()
	userLinkSpec := baseSpec
	userLinkSpec.UserID = uuid.NewString()
	if err := os.Symlink(outside, filepath.Join(exportsBase, userLinkSpec.UserID)); err != nil {
		t.Fatalf("Symlink(user): %v", err)
	}
	if _, err := store.CreateWriter(ctx, userLinkSpec); err == nil {
		t.Fatal("CreateWriter accepted symlink user directory")
	}

	componentSpec := baseSpec
	componentSpec.Name = "link/file.zip"
	jobDir := filepath.Join(exportsBase, componentSpec.UserID, componentSpec.JobID)
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(jobDir): %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(jobDir, "link")); err != nil {
		t.Fatalf("Symlink(name component): %v", err)
	}
	if _, err := store.CreateWriter(ctx, componentSpec); err == nil {
		t.Fatal("CreateWriter accepted symlink object-name component")
	}
}

func TestLocalStoreOpenUsesNoFollowFinalPath(t *testing.T) {
	ctx := context.Background()
	store, exportsBase, _ := newLocalStoreForTest(t)
	spec := localObjectSpec("swap.zip")
	ref := committedLocalObject(t, ctx, store, spec, "safe")

	finalPath := filepath.Join(exportsBase, spec.UserID, spec.JobID, spec.Name)
	if err := os.Remove(finalPath); err != nil {
		t.Fatalf("Remove(final): %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile(outside): %v", err)
	}
	if err := os.Symlink(outside, finalPath); err != nil {
		t.Fatalf("Symlink(final): %v", err)
	}

	if _, err := store.Open(ctx, ref, nil); err == nil {
		t.Fatal("Open followed final-path symlink")
	}
	if _, err := store.OpenReaderAt(ctx, ref); err == nil {
		t.Fatal("OpenReaderAt followed final-path symlink")
	}
	stat, err := store.Stat(ctx, ref)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.State == ObjectMissing {
		t.Fatalf("Stat reported symlink as confirmed missing: %#v", stat)
	}
}

func TestLocalStoreCommitIsAtomicSameDirectoryRename(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newLocalStoreForTest(t)

	writer, err := store.CreateWriter(ctx, localObjectSpec("atomic.zip"))
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	localWriter, ok := writer.(*localObjectWriter)
	if !ok {
		t.Fatalf("writer type = %T, want *localObjectWriter", writer)
	}
	if filepath.Dir(localWriter.tempPath) != filepath.Dir(localWriter.finalPath) {
		t.Fatalf("temp dir = %q, final dir = %q", filepath.Dir(localWriter.tempPath), filepath.Dir(localWriter.finalPath))
	}
	if !strings.Contains(filepath.Base(localWriter.tempPath), filepath.Base(localWriter.finalPath)+".tmp-") {
		t.Fatalf("temp path %q does not include final basename and random suffix", localWriter.tempPath)
	}
	if _, err := writer.Write([]byte("atomic")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := writer.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestLocalStoreDeletePrefixRefusesBaseDirectory(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newLocalStoreForTest(t)
	spec := localObjectSpec("keep.zip")
	ref := committedLocalObject(t, ctx, store, spec, "keep")

	for _, prefix := range []ArtifactPrefix{
		{},
		{Kind: "export"},
		{Kind: "export", UserID: spec.UserID},
		{Kind: "export", UserID: spec.UserID, JobID: "not-a-uuid"},
	} {
		if err := store.DeletePrefix(ctx, prefix); err == nil {
			t.Fatalf("DeletePrefix accepted unsafe prefix %#v", prefix)
		}
	}
	if stat, err := store.Stat(ctx, ref); err != nil || stat.State != ObjectPresent {
		t.Fatalf("object after refused DeletePrefix: stat=%#v err=%v", stat, err)
	}

	if err := store.DeletePrefix(ctx, ArtifactPrefix{Kind: "export", UserID: spec.UserID, JobID: spec.JobID}); err != nil {
		t.Fatalf("DeletePrefix(scoped job): %v", err)
	}
	if stat, err := store.Stat(ctx, ref); err != nil || stat.State != ObjectMissing {
		t.Fatalf("object after scoped DeletePrefix: stat=%#v err=%v", stat, err)
	}
}

func newLocalStoreForTest(t *testing.T) (*LocalStore, string, string) {
	t.Helper()
	root := t.TempDir()
	exportsBase := filepath.Join(root, "exports")
	importsBase := filepath.Join(root, "imports")
	store, err := NewLocalStore(exportsBase, importsBase)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return store, exportsBase, importsBase
}

func localObjectSpec(name string) ObjectSpec {
	return ObjectSpec{
		Kind:        "export",
		UserID:      uuid.NewString(),
		JobID:       uuid.NewString(),
		Role:        RoleSingleZIP,
		Name:        name,
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
}

func committedLocalObject(t *testing.T, ctx context.Context, store *LocalStore, spec ObjectSpec, content string) ObjectRef {
	t.Helper()
	writer, err := store.CreateWriter(ctx, spec)
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return ref
}
