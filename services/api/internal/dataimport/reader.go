package dataimport

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxDataBytes is the decompression limit for OpenData() calls (JSON data files).
const maxDataBytes = 500 * 1024 * 1024 // 500 MB

// Source abstracts ZIP, directory, and multi-part GDPR exports.
type Source interface {
	// Manifest returns the parsed manifest.json from the export.
	Manifest() (*Manifest, error)
	// OpenData opens a data file (e.g. "data/activities.json") for reading.
	// Returns a limited reader — max 500 MB decompressed.
	OpenData(path string) (io.ReadCloser, error)
	// OpenFile opens an attached file (must have "files/" prefix).
	OpenFile(path string) (io.ReadCloser, error)
	// ListFiles returns all paths available in the source.
	ListFiles() ([]string, error)
	// Columns returns the parsed columns.json as a table→column-list map,
	// or nil if columns.json is absent.
	Columns() (map[string][]string, error)
	// EstimatedFileBytes returns the best-effort total on-disk size of the
	// export source (compressed ZIP size for ZipSource, sum of all part ZIPs
	// for MultiPartSource, sum of files/ entries for DirSource).
	// Returns 0 if the size cannot be determined.
	EstimatedFileBytes() int64
	// Close releases all resources held by the source.
	Close() error
}

// Manifest is the parsed representation of manifest.json inside a GDPR export.
type Manifest struct {
	FormatVersion    int                    `json:"format_version"`
	SkadiaVersion   string                 `json:"skadia_version"`
	SchemaMigration  int                    `json:"schema_migration"`
	ExportedAt       time.Time              `json:"exported_at"`
	UserID           string                 `json:"user_id"`
	Multipart        bool                   `json:"multipart,omitempty"`
	Parts            []ManifestPart         `json:"parts,omitempty"`
	EntityCounts     map[string]int         `json:"entity_counts,omitempty"`
	Categories       map[string][]string    `json:"categories,omitempty"`
	IncompleteTables []string               `json:"incomplete_tables,omitempty"`
	ExportErrors     []ManifestExportError  `json:"export_errors,omitempty"`
	FailedFiles      []string               `json:"failed_files,omitempty"`
}

// ManifestExportError mirrors the exportTableError struct written by the
// export handler into manifest.json: {"table":"...","error":"..."}.
type ManifestExportError struct {
	Table string `json:"table"`
	Error string `json:"error"`
}

// ManifestPart describes one part ZIP in a multi-part export.
type ManifestPart struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// TotalFileSizeBytes returns the sum of all part sizes.
// For single-part exports (Parts == nil) it returns 0.
func (m *Manifest) TotalFileSizeBytes() int64 {
	var total int64
	for _, p := range m.Parts {
		total += p.SizeBytes
	}
	return total
}

// isValidPath returns false if the path contains ".." components (path-traversal guard).
func isValidPath(p string) bool {
	clean := filepath.Clean(p)
	// filepath.Clean removes trailing slashes and resolves ".." — if the result
	// starts with ".." the caller tried to escape the root.
	return !strings.HasPrefix(clean, "..") && !strings.Contains(p, "..")
}

// limitedReadCloser wraps an io.ReadCloser with a hard size cap.
type limitedReadCloser struct {
	r io.ReadCloser
	n int64 // remaining bytes
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, fmt.Errorf("dataimport: decompression limit (%d bytes) exceeded", maxDataBytes)
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func (l *limitedReadCloser) Close() error { return l.r.Close() }

func newLimitedReadCloser(rc io.ReadCloser) io.ReadCloser {
	return &limitedReadCloser{r: rc, n: maxDataBytes}
}

// verifySHA256 reads the file at path and compares its SHA-256 against expected.
func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("verifySHA256 open %q: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("verifySHA256 hash %q: %w", path, err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("verifySHA256 %q: expected %s, got %s", path, expected, got)
	}
	return nil
}

// OpenSource auto-detects and opens a GDPR export at path.
// Supported formats:
//   - Single ZIP file (.zip)
//   - Extracted directory (plain DirSource)
//   - Extracted directory with multipart manifest → MultiPartSource
func OpenSource(path string) (Source, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("dataimport: stat %q: %w", path, err)
	}

	if !info.IsDir() {
		// Assume ZIP file.
		return NewZipSource(path)
	}

	// Directory: peek at manifest.json to decide if it is multi-part.
	manifestPath := filepath.Join(path, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		// No manifest → plain directory source.
		return NewDirSource(path), nil
	}

	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("dataimport: parse manifest.json in %q: %w", path, err)
	}

	if m.Multipart {
		return NewMultiPartSource(path)
	}
	return NewDirSource(path), nil
}

// ---------------------------------------------------------------------------
// ZipSource
// ---------------------------------------------------------------------------

// ZipSource reads from a single GDPR export ZIP file.
type ZipSource struct {
	path    string // absolute path to the ZIP file, used by EstimatedFileBytes
	rc      *zip.ReadCloser
	zr      *zip.Reader
	idx     map[string]*zip.File
	colOnce sync.Once
	colMap  map[string][]string
	colErr  error
}

// NewZipSource opens a ZIP file and indexes its entries.
// Symlink entries are silently skipped.
func NewZipSource(path string) (*ZipSource, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("dataimport: open zip %q: %w", path, err)
	}
	s := &ZipSource{path: path, rc: rc, zr: &rc.Reader}
	s.buildIndex()
	return s, nil
}

func (s *ZipSource) buildIndex() {
	s.idx = make(map[string]*zip.File, len(s.zr.File))
	for _, f := range s.zr.File {
		// Skip symlinks (Mode() & fs.ModeSymlink).
		if f.Mode()&0o120000 != 0 {
			continue
		}
		s.idx[f.Name] = f
	}
}

func (s *ZipSource) Manifest() (*Manifest, error) {
	rc, err := s.openZipEntry("manifest.json")
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return parseManifest(rc)
}

func (s *ZipSource) OpenData(path string) (io.ReadCloser, error) {
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	rc, err := s.openZipEntry(path)
	if err != nil {
		return nil, err
	}
	return newLimitedReadCloser(rc), nil
}

func (s *ZipSource) OpenFile(path string) (io.ReadCloser, error) {
	if !strings.HasPrefix(path, "files/") {
		return nil, fmt.Errorf("dataimport: OpenFile only allows paths under files/, got %q", path)
	}
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	return s.openZipEntry(path)
}

func (s *ZipSource) ListFiles() ([]string, error) {
	paths := make([]string, 0, len(s.idx))
	for name := range s.idx {
		paths = append(paths, name)
	}
	return paths, nil
}

func (s *ZipSource) Columns() (map[string][]string, error) {
	s.colOnce.Do(func() {
		rc, err := s.openZipEntry("columns.json")
		if err != nil {
			s.colMap = nil
			return // columns.json is optional
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

func (s *ZipSource) EstimatedFileBytes() int64 {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (s *ZipSource) Close() error {
	return s.rc.Close()
}

func (s *ZipSource) openZipEntry(name string) (io.ReadCloser, error) {
	f, ok := s.idx[name]
	if !ok {
		return nil, fmt.Errorf("dataimport: entry %q not found in zip: %w", name, fs.ErrNotExist)
	}
	return f.Open()
}

// ---------------------------------------------------------------------------
// DirSource
// ---------------------------------------------------------------------------

// DirSource reads from an extracted (unpacked) GDPR export directory.
type DirSource struct {
	root    string
	colOnce sync.Once
	colMap  map[string][]string
	colErr  error
}

// NewDirSource returns a DirSource rooted at dir.
func NewDirSource(dir string) *DirSource {
	return &DirSource{root: dir}
}

func (s *DirSource) Manifest() (*Manifest, error) {
	rc, err := s.open("manifest.json")
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return parseManifest(rc)
}

func (s *DirSource) OpenData(path string) (io.ReadCloser, error) {
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	rc, err := s.open(path)
	if err != nil {
		return nil, err
	}
	return newLimitedReadCloser(rc), nil
}

func (s *DirSource) OpenFile(path string) (io.ReadCloser, error) {
	if !strings.HasPrefix(path, "files/") {
		return nil, fmt.Errorf("dataimport: OpenFile only allows paths under files/, got %q", path)
	}
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	return s.open(path)
}

func (s *DirSource) ListFiles() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(s.root, path)
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, err
}

func (s *DirSource) Columns() (map[string][]string, error) {
	s.colOnce.Do(func() {
		rc, err := s.open("columns.json")
		if err != nil {
			s.colMap = nil
			return // optional
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

// EstimatedFileBytes returns the total size of all regular files under the
// files/ sub-directory (the attached-file tree). Returns 0 on any error.
func (s *DirSource) EstimatedFileBytes() int64 {
	var total int64
	filesDir := filepath.Join(s.root, "files")
	_ = filepath.WalkDir(filesDir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *DirSource) Close() error { return nil }

func (s *DirSource) open(rel string) (io.ReadCloser, error) {
	abs := filepath.Join(s.root, rel)
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("dataimport: open %q: %w", abs, err)
	}
	return f, nil
}

// ---------------------------------------------------------------------------
// MultiPartSource
// ---------------------------------------------------------------------------

// MultiPartSource handles a multi-part GDPR export directory.
// Layout:
//
//	<root>/
//	  manifest.json          (multipart: true, lists parts)
//	  metadata.zip           (contains columns.json + data/ JSON files)
//	  <part-name>.zip        (contains files/ entries, one or more)
//
// SHA-256 of every part ZIP is verified eagerly when the source is opened.
type MultiPartSource struct {
	root     string
	manifest *Manifest
	meta     *ZipSource            // inner source for data/ and columns.json
	parts    map[string]*ZipSource // part name → zip source for files/
}

// NewMultiPartSource opens and verifies a multi-part export rooted at dir.
func NewMultiPartSource(dir string) (*MultiPartSource, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("dataimport: read manifest.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("dataimport: parse manifest.json: %w", err)
	}
	if !m.Multipart {
		return nil, fmt.Errorf("dataimport: manifest.json does not have multipart=true")
	}

	// Verify + open metadata.zip.
	metaPath := filepath.Join(dir, "metadata.zip")
	// metadata.zip SHA-256 is not listed in parts (parts list covers data ZIPs).
	// We still open it, letting zip.OpenReader validate its own CRC entries.
	meta, err := NewZipSource(metaPath)
	if err != nil {
		return nil, fmt.Errorf("dataimport: open metadata.zip: %w", err)
	}

	// Verify + open each part ZIP.
	partSources := make(map[string]*ZipSource, len(m.Parts))
	for _, part := range m.Parts {
		partPath := filepath.Join(dir, part.Name)
		if part.SHA256 != "" {
			if err := verifySHA256(partPath, part.SHA256); err != nil {
				meta.Close()
				for _, ps := range partSources {
					ps.Close()
				}
				return nil, fmt.Errorf("dataimport: SHA-256 mismatch for part %q: %w", part.Name, err)
			}
		}
		ps, err := NewZipSource(partPath)
		if err != nil {
			meta.Close()
			for _, open := range partSources {
				open.Close()
			}
			return nil, fmt.Errorf("dataimport: open part %q: %w", part.Name, err)
		}
		partSources[part.Name] = ps
	}

	return &MultiPartSource{
		root:     dir,
		manifest: &m,
		meta:     meta,
		parts:    partSources,
	}, nil
}

func (s *MultiPartSource) Manifest() (*Manifest, error) {
	return s.manifest, nil
}

func (s *MultiPartSource) OpenData(path string) (io.ReadCloser, error) {
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	return s.meta.OpenData(path)
}

func (s *MultiPartSource) OpenFile(path string) (io.ReadCloser, error) {
	if !strings.HasPrefix(path, "files/") {
		return nil, fmt.Errorf("dataimport: OpenFile only allows paths under files/, got %q", path)
	}
	if !isValidPath(path) {
		return nil, fmt.Errorf("dataimport: invalid path %q", path)
	}
	// Search across all part ZIPs.
	for _, ps := range s.parts {
		rc, err := ps.openZipEntry(path)
		if err == nil {
			return rc, nil
		}
	}
	return nil, fmt.Errorf("dataimport: file %q not found in any part", path)
}

func (s *MultiPartSource) ListFiles() ([]string, error) {
	metaFiles, err := s.meta.ListFiles()
	if err != nil {
		return nil, err
	}
	all := make([]string, 0, len(metaFiles))
	all = append(all, metaFiles...)
	for _, ps := range s.parts {
		pf, err := ps.ListFiles()
		if err != nil {
			return nil, err
		}
		all = append(all, pf...)
	}
	return all, nil
}

func (s *MultiPartSource) Columns() (map[string][]string, error) {
	return s.meta.Columns()
}

// EstimatedFileBytes returns the sum of SizeBytes for all manifest parts.
// This mirrors the information already in TotalFileSizeBytes() on the Manifest,
// but satisfies the Source interface so callers can use a single method
// regardless of source type.
func (s *MultiPartSource) EstimatedFileBytes() int64 {
	return s.manifest.TotalFileSizeBytes()
}

func (s *MultiPartSource) Close() error {
	var first error
	if err := s.meta.Close(); err != nil && first == nil {
		first = err
	}
	for _, ps := range s.parts {
		if err := ps.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// parseManifest decodes a Manifest from r.
func parseManifest(r io.Reader) (*Manifest, error) {
	var m Manifest
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("dataimport: decode manifest: %w", err)
	}
	return &m, nil
}
