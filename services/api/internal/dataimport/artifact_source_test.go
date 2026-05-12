package dataimport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
)

func TestArtifactZipSourceUsesReaderAtAndOpenDataStream(t *testing.T) {
	ctx := context.Background()
	zipBytes := buildZipBytesForArtifactTest(t, map[string][]byte{
		"manifest.json":        []byte(`{"format_version":1,"skadia_version":"test","schema_migration":1,"exported_at":"2026-05-12T12:00:00Z","user_id":"source"}`),
		"columns.json":         []byte(`{"activities":["id"]}`),
		"data/activities.json": []byte(`[{"id":"a"}]`),
		"files/activity/a.fit": []byte("file"),
	})
	store := newArtifactSourceTestStore()
	ref := store.put("export.zip", gdprartifacts.RoleSingleZIP, "application/zip", zipBytes)

	src, err := NewArtifactZipSource(ctx, store, ref)
	if err != nil {
		t.Fatalf("NewArtifactZipSource: %v", err)
	}
	defer src.Close()
	if store.openReaderAtCalls != 1 {
		t.Fatalf("OpenReaderAt calls = %d, want 1", store.openReaderAtCalls)
	}
	rc, err := src.OpenDataStream("data/activities.json")
	if err != nil {
		t.Fatalf("OpenDataStream: %v", err)
	}
	defer rc.Close()
	raw, _ := io.ReadAll(rc)
	if string(raw) != `[{"id":"a"}]` {
		t.Fatalf("OpenDataStream bytes = %s", raw)
	}
}

func TestArtifactMultiPartSourceUsesReaderAtAndVerifiesChecksums(t *testing.T) {
	ctx := context.Background()
	store, artifact := buildArtifactMultipartSourceForTest(t, map[string][]byte{
		"files/activity/a.fit": []byte("file"),
	})

	src, err := NewArtifactMultiPartSource(ctx, store, artifact)
	if err != nil {
		t.Fatalf("NewArtifactMultiPartSource: %v", err)
	}
	defer src.Close()
	if store.openReaderAtCalls < 2 {
		t.Fatalf("OpenReaderAt calls = %d, want metadata and files parts", store.openReaderAtCalls)
	}
	rc, err := src.OpenFile("files/activity/a.fit")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer rc.Close()
	raw, _ := io.ReadAll(rc)
	if string(raw) != "file" {
		t.Fatalf("file bytes = %q", raw)
	}
}

func TestArtifactMultiPartSourceRejectsDuplicateFilesAcrossParts(t *testing.T) {
	ctx := context.Background()
	store := newArtifactSourceTestStore()
	metadata := buildZipBytesForArtifactTest(t, map[string][]byte{"data/activities.json": []byte(`[]`)})
	filesA := buildZipBytesForArtifactTest(t, map[string][]byte{"files/activity/a.fit": []byte("a")})
	filesB := buildZipBytesForArtifactTest(t, map[string][]byte{"files/activity/a.fit": []byte("b")})
	metadataRef := store.put("metadata.zip", gdprartifacts.RoleMetadataZIP, "application/zip", metadata)
	filesARef := store.put("files-0001.zip", gdprartifacts.RoleFilesZIP, "application/zip", filesA)
	filesBRef := store.put("files-0002.zip", gdprartifacts.RoleFilesZIP, "application/zip", filesB)
	manifestRef := store.putManifest(t, []ManifestPart{
		{Name: metadataRef.Name, Role: "metadata", SHA256: metadataRef.SHA256, SizeBytes: metadataRef.SizeBytes},
		{Name: filesARef.Name, Role: "files", SHA256: filesARef.SHA256, SizeBytes: filesARef.SizeBytes},
		{Name: filesBRef.Name, Role: "files", SHA256: filesBRef.SHA256, SizeBytes: filesBRef.SizeBytes},
	})
	artifact := artifactForSourceTest(manifestRef, metadataRef, filesARef, filesBRef)

	if _, err := NewArtifactMultiPartSource(ctx, store, artifact); err == nil {
		t.Fatal("NewArtifactMultiPartSource accepted duplicate files entry")
	}
}

func TestArtifactMultiPartSourceVerifiesChecksumBeforeOpen(t *testing.T) {
	ctx := context.Background()
	store, artifact := buildArtifactMultipartSourceForTest(t, map[string][]byte{
		"files/activity/a.fit": []byte("file"),
	})
	store.objects["files-0001.zip"] = []byte("not the zip named in manifest")
	if _, err := NewArtifactMultiPartSource(ctx, store, artifact); err == nil {
		t.Fatal("NewArtifactMultiPartSource accepted checksum mismatch")
	}
}

type artifactSourceTestStore struct {
	objects           map[string][]byte
	refs              map[string]gdprartifacts.ObjectRef
	openReaderAtCalls int
}

func newArtifactSourceTestStore() *artifactSourceTestStore {
	return &artifactSourceTestStore{objects: make(map[string][]byte), refs: make(map[string]gdprartifacts.ObjectRef)}
}

func (s *artifactSourceTestStore) put(name string, role gdprartifacts.ObjectRole, contentType string, data []byte) gdprartifacts.ObjectRef {
	sum := sha256.Sum256(data)
	ref := gdprartifacts.ObjectRef{
		Backend:     "local",
		Kind:        "export",
		UserID:      "11111111-1111-1111-1111-111111111111",
		JobID:       "22222222-2222-2222-2222-222222222222",
		Role:        role,
		Name:        name,
		Path:        name,
		SHA256:      hex.EncodeToString(sum[:]),
		SizeBytes:   int64(len(data)),
		ContentType: contentType,
		CreatedAt:   time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		ExpiresAt:   time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	}
	s.objects[name] = data
	s.refs[name] = ref
	return ref
}

func (s *artifactSourceTestStore) putManifest(t *testing.T, parts []ManifestPart) gdprartifacts.ObjectRef {
	t.Helper()
	raw, err := json.Marshal(Manifest{
		FormatVersion:   1,
		SkadiaVersion:  "test",
		SchemaMigration: 1,
		ExportedAt:      time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		UserID:          "source",
		Multipart:       true,
		Parts:           parts,
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return s.put("manifest.json", gdprartifacts.RoleManifest, "application/json", raw)
}

func (s *artifactSourceTestStore) CreateWriter(context.Context, gdprartifacts.ObjectSpec) (gdprartifacts.ObjectWriter, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *artifactSourceTestStore) Open(_ context.Context, ref gdprartifacts.ObjectRef, _ *gdprartifacts.RangeSpec) (gdprartifacts.ObjectReader, error) {
	data, ok := s.objects[ref.Name]
	if !ok {
		return nil, fmt.Errorf("missing object %s", ref.Name)
	}
	return &artifactSourceObjectReader{Reader: bytes.NewReader(data), size: int64(len(data)), contentType: ref.ContentType}, nil
}

func (s *artifactSourceTestStore) OpenReaderAt(_ context.Context, ref gdprartifacts.ObjectRef) (gdprartifacts.SizedReaderAt, error) {
	s.openReaderAtCalls++
	data, ok := s.objects[ref.Name]
	if !ok {
		return nil, fmt.Errorf("missing object %s", ref.Name)
	}
	return &artifactSourceReaderAt{Reader: bytes.NewReader(data), size: int64(len(data))}, nil
}

func (s *artifactSourceTestStore) Stat(context.Context, gdprartifacts.ObjectRef) (gdprartifacts.ObjectStat, error) {
	return gdprartifacts.ObjectStat{}, nil
}
func (s *artifactSourceTestStore) Delete(context.Context, gdprartifacts.ObjectRef) error { return nil }
func (s *artifactSourceTestStore) DeletePrefix(context.Context, gdprartifacts.ArtifactPrefix) error {
	return nil
}

type artifactSourceObjectReader struct {
	*bytes.Reader
	size        int64
	contentType string
}

func (r *artifactSourceObjectReader) Close() error        { return nil }
func (r *artifactSourceObjectReader) Size() int64         { return r.size }
func (r *artifactSourceObjectReader) ContentType() string { return r.contentType }

type artifactSourceReaderAt struct {
	*bytes.Reader
	size int64
}

func (r *artifactSourceReaderAt) Close() error { return nil }
func (r *artifactSourceReaderAt) Size() int64  { return r.size }

func buildArtifactMultipartSourceForTest(t *testing.T, fileEntries map[string][]byte) (*artifactSourceTestStore, gdprartifacts.Artifact) {
	t.Helper()
	store := newArtifactSourceTestStore()
	metadata := buildZipBytesForArtifactTest(t, map[string][]byte{
		"columns.json":         []byte(`{"activities":["id"]}`),
		"data/activities.json": []byte(`[]`),
	})
	files := buildZipBytesForArtifactTest(t, fileEntries)
	metadataRef := store.put("metadata.zip", gdprartifacts.RoleMetadataZIP, "application/zip", metadata)
	filesRef := store.put("files-0001.zip", gdprartifacts.RoleFilesZIP, "application/zip", files)
	manifestRef := store.putManifest(t, []ManifestPart{
		{Name: metadataRef.Name, Role: "metadata", SHA256: metadataRef.SHA256, SizeBytes: metadataRef.SizeBytes},
		{Name: filesRef.Name, Role: "files", SHA256: filesRef.SHA256, SizeBytes: filesRef.SizeBytes},
	})
	return store, artifactForSourceTest(manifestRef, metadataRef, filesRef)
}

func artifactForSourceTest(refs ...gdprartifacts.ObjectRef) gdprartifacts.Artifact {
	return gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatMultipart,
		ExpiresAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
		Objects:   refs,
	}
}

func buildZipBytesForArtifactTest(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
