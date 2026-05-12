package dataimport

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
)

type ArtifactZipSource struct {
	ref     gdprartifacts.ObjectRef
	reader  gdprartifacts.SizedReaderAt
	zr      *zip.Reader
	idx     map[string]*zip.File
	colOnce sync.Once
	colMap  map[string][]string
	colErr  error
}

func NewArtifactZipSource(ctx context.Context, store gdprartifacts.Store, ref gdprartifacts.ObjectRef) (*ArtifactZipSource, error) {
	reader, err := store.OpenReaderAt(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("dataimport: open artifact reader %q: %w", ref.Name, err)
	}
	zr, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("dataimport: open artifact zip %q: %w", ref.Name, err)
	}
	src := &ArtifactZipSource{ref: ref, reader: reader, zr: zr}
	src.buildIndex()
	return src, nil
}

func (s *ArtifactZipSource) buildIndex() {
	s.idx = make(map[string]*zip.File, len(s.zr.File))
	for _, f := range s.zr.File {
		if f.Mode()&0o120000 != 0 {
			continue
		}
		s.idx[f.Name] = f
	}
}

func (s *ArtifactZipSource) Manifest() (*Manifest, error) {
	rc, err := s.openZipEntry("manifest.json")
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return parseManifest(rc)
}

func (s *ArtifactZipSource) OpenData(path string) (io.ReadCloser, error) {
	rc, err := s.OpenDataStream(path)
	if err != nil {
		return nil, err
	}
	return newLimitedReadCloser(rc), nil
}

func (s *ArtifactZipSource) OpenDataStream(path string) (io.ReadCloser, error) {
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	f, ok := s.idx[path]
	if !ok {
		return nil, fmt.Errorf("dataimport: entry %q not found in artifact zip", path)
	}
	if f.UncompressedSize64 > uint64(maxStreamDataBytes) {
		return nil, fmt.Errorf("dataimport: data entry %q exceeds stream limit", path)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	return newStreamDataReadCloser(rc), nil
}

func (s *ArtifactZipSource) OpenFile(path string) (io.ReadCloser, error) {
	if !strings.HasPrefix(path, "files/") {
		return nil, fmt.Errorf("dataimport: OpenFile only allows paths under files/, got %q", path)
	}
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	return s.openZipEntry(path)
}

func (s *ArtifactZipSource) ListFiles() ([]string, error) {
	paths := make([]string, 0, len(s.idx))
	for name := range s.idx {
		paths = append(paths, name)
	}
	return paths, nil
}

func (s *ArtifactZipSource) Columns() (map[string][]string, error) {
	s.colOnce.Do(func() {
		rc, err := s.openZipEntry("columns.json")
		if err != nil {
			return
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			s.colErr = fmt.Errorf("dataimport: read columns.json: %w", err)
			return
		}
		var m map[string][]string
		if err := json.Unmarshal(raw, &m); err != nil {
			s.colErr = fmt.Errorf("dataimport: parse columns.json: %w", err)
			return
		}
		s.colMap = m
	})
	return s.colMap, s.colErr
}

func (s *ArtifactZipSource) EstimatedFileBytes() int64 { return s.reader.Size() }

func (s *ArtifactZipSource) Close() error { return s.reader.Close() }

func (s *ArtifactZipSource) openZipEntry(name string) (io.ReadCloser, error) {
	f, ok := s.idx[name]
	if !ok {
		return nil, fmt.Errorf("dataimport: entry %q not found in artifact zip", name)
	}
	return f.Open()
}

type ArtifactMultiPartSource struct {
	manifest *Manifest
	meta     *ArtifactZipSource
	parts    map[string]*ArtifactZipSource
	closers  []io.Closer
}

func NewArtifactMultiPartSource(ctx context.Context, store gdprartifacts.Store, artifact gdprartifacts.Artifact) (*ArtifactMultiPartSource, error) {
	manifestRef, ok := artifactObjectByRole(artifact, gdprartifacts.RoleManifest)
	if !ok {
		return nil, fmt.Errorf("dataimport: multipart artifact missing manifest")
	}
	manifest, err := readArtifactManifest(ctx, store, manifestRef)
	if err != nil {
		return nil, err
	}
	refsByName := make(map[string]gdprartifacts.ObjectRef, len(artifact.Objects))
	for _, ref := range artifact.Objects {
		refsByName[ref.Name] = ref
	}

	parts, _, err := normalizeManifestParts("", manifest.Parts)
	if err != nil {
		return nil, err
	}
	manifest.Parts = parts

	src := &ArtifactMultiPartSource{manifest: manifest, parts: make(map[string]*ArtifactZipSource)}
	for _, part := range manifest.Parts {
		ref, ok := refsByName[part.Name]
		if !ok {
			src.Close()
			return nil, fmt.Errorf("dataimport: artifact object %q missing", part.Name)
		}
		if err := verifyArtifactChecksum(ctx, store, ref, part.SHA256); err != nil {
			src.Close()
			return nil, fmt.Errorf("dataimport: SHA-256 mismatch for artifact part %q: %w", part.Name, err)
		}
		zipSrc, err := NewArtifactZipSource(ctx, store, ref)
		if err != nil {
			src.Close()
			return nil, err
		}
		src.closers = append(src.closers, zipSrc)
		if part.Role == "metadata" {
			src.meta = zipSrc
		} else {
			src.parts[part.Name] = zipSrc
		}
	}
	if src.meta == nil {
		src.Close()
		return nil, fmt.Errorf("dataimport: multipart artifact missing metadata.zip")
	}
	if err := src.rejectDuplicateFileEntries(); err != nil {
		src.Close()
		return nil, err
	}
	return src, nil
}

func (s *ArtifactMultiPartSource) Manifest() (*Manifest, error) { return s.manifest, nil }

func (s *ArtifactMultiPartSource) OpenData(path string) (io.ReadCloser, error) {
	rc, err := s.OpenDataStream(path)
	if err != nil {
		return nil, err
	}
	return newLimitedReadCloser(rc), nil
}

func (s *ArtifactMultiPartSource) OpenDataStream(path string) (io.ReadCloser, error) {
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	return s.meta.OpenDataStream(path)
}

func (s *ArtifactMultiPartSource) OpenFile(path string) (io.ReadCloser, error) {
	if !strings.HasPrefix(path, "files/") {
		return nil, fmt.Errorf("dataimport: OpenFile only allows paths under files/, got %q", path)
	}
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	for _, part := range s.parts {
		rc, err := part.openZipEntry(path)
		if err == nil {
			return rc, nil
		}
	}
	return nil, fmt.Errorf("dataimport: file %q not found in any artifact part", path)
}

func (s *ArtifactMultiPartSource) ListFiles() ([]string, error) {
	metaFiles, err := s.meta.ListFiles()
	if err != nil {
		return nil, err
	}
	all := make([]string, 0, len(metaFiles))
	all = append(all, metaFiles...)
	for _, part := range s.parts {
		paths, err := part.ListFiles()
		if err != nil {
			return nil, err
		}
		all = append(all, paths...)
	}
	return all, nil
}

func (s *ArtifactMultiPartSource) Columns() (map[string][]string, error) {
	return s.meta.Columns()
}

func (s *ArtifactMultiPartSource) EstimatedFileBytes() int64 {
	return s.manifest.TotalFileSizeBytes()
}

func (s *ArtifactMultiPartSource) Close() error {
	var first error
	for _, closer := range s.closers {
		if err := closer.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *ArtifactMultiPartSource) rejectDuplicateFileEntries() error {
	seen := make(map[string]struct{})
	for _, part := range s.parts {
		paths, err := part.ListFiles()
		if err != nil {
			return err
		}
		for _, p := range paths {
			if !strings.HasPrefix(p, "files/") {
				continue
			}
			if _, exists := seen[p]; exists {
				return fmt.Errorf("dataimport: duplicate file entry across multipart artifacts: %s", p)
			}
			seen[p] = struct{}{}
		}
	}
	return nil
}

func readArtifactManifest(ctx context.Context, store gdprartifacts.Store, ref gdprartifacts.ObjectRef) (*Manifest, error) {
	reader, err := store.Open(ctx, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("dataimport: open artifact manifest: %w", err)
	}
	defer reader.Close()
	var manifest Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("dataimport: decode artifact manifest: %w", err)
	}
	if !manifest.Multipart {
		return nil, fmt.Errorf("dataimport: artifact manifest is not multipart")
	}
	return &manifest, nil
}

func verifyArtifactChecksum(ctx context.Context, store gdprartifacts.Store, ref gdprartifacts.ObjectRef, expected string) error {
	if expected == "" {
		return nil
	}
	reader, err := store.Open(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer reader.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, reader); err != nil {
		return err
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != expected {
		return fmt.Errorf("expected %s, got %s", expected, got)
	}
	return nil
}

func artifactObjectByRole(artifact gdprartifacts.Artifact, role gdprartifacts.ObjectRole) (gdprartifacts.ObjectRef, bool) {
	for _, ref := range artifact.Objects {
		if ref.Role == role {
			return ref, true
		}
	}
	return gdprartifacts.ObjectRef{}, false
}

var _ Source = (*ArtifactZipSource)(nil)
var _ Source = (*ArtifactMultiPartSource)(nil)
