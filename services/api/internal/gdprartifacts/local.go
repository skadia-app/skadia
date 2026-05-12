package gdprartifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const localBackend = "local"

type LocalStore struct {
	exportsBase string
	importsBase string
}

type localObjectWriter struct {
	store     *LocalStore
	spec      ObjectSpec
	file      *os.File
	hasher    hash.Hash
	tempPath  string
	finalPath string
	relPath   string
	sizeBytes int64
	closed    bool
}

type localObjectReader struct {
	reader      io.Reader
	file        *os.File
	sizeBytes   int64
	contentType string
}

type localReaderAt struct {
	*os.File
	sizeBytes int64
}

func NewLocalStore(exportsBase, importsBase string) (*LocalStore, error) {
	if exportsBase == "" {
		return nil, errors.New("exports base is required")
	}
	if importsBase == "" {
		return nil, errors.New("imports base is required")
	}
	store := &LocalStore{
		exportsBase: filepath.Clean(exportsBase),
		importsBase: filepath.Clean(importsBase),
	}
	if err := ensurePrivateBaseDir(store.exportsBase); err != nil {
		return nil, fmt.Errorf("prepare exports base: %w", err)
	}
	if err := ensurePrivateBaseDir(store.importsBase); err != nil {
		return nil, fmt.Errorf("prepare imports base: %w", err)
	}
	return store, nil
}

func (s *LocalStore) CreateWriter(ctx context.Context, spec ObjectSpec) (ObjectWriter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveSpec(spec, true)
	if err != nil {
		return nil, err
	}
	tempPath, err := tempPathFor(resolved.finalPath)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &localObjectWriter{
		store:     s,
		spec:      spec,
		file:      file,
		hasher:    sha256.New(),
		tempPath:  tempPath,
		finalPath: resolved.finalPath,
		relPath:   resolved.relPath,
	}, nil
}

func (s *LocalStore) Open(ctx context.Context, ref ObjectRef, rangeSpec *RangeSpec) (ObjectReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveRef(ref)
	if err != nil {
		return nil, err
	}
	file, info, err := openRegularNoFollow(resolved.finalPath)
	if err != nil {
		return nil, err
	}
	size := info.Size()
	start, end, err := normalizeRange(rangeSpec, size)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if rangeSpec == nil {
		return &localObjectReader{
			reader:      file,
			file:        file,
			sizeBytes:   size,
			contentType: ref.ContentType,
		}, nil
	}
	return &localObjectReader{
		reader:      io.NewSectionReader(file, start, end-start),
		file:        file,
		sizeBytes:   end - start,
		contentType: ref.ContentType,
	}, nil
}

func (s *LocalStore) OpenReaderAt(ctx context.Context, ref ObjectRef) (SizedReaderAt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveRef(ref)
	if err != nil {
		return nil, err
	}
	file, info, err := openRegularNoFollow(resolved.finalPath)
	if err != nil {
		return nil, err
	}
	return &localReaderAt{File: file, sizeBytes: info.Size()}, nil
}

func (s *LocalStore) Stat(ctx context.Context, ref ObjectRef) (ObjectStat, error) {
	if err := ctx.Err(); err != nil {
		return ObjectStat{}, err
	}
	resolved, err := s.resolveRef(ref)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectStat{State: ObjectMissing}, nil
		}
		return ObjectStat{State: ObjectUnknown, Err: err}, nil
	}
	file, info, err := openRegularNoFollow(resolved.finalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectStat{State: ObjectMissing}, nil
		}
		return ObjectStat{State: ObjectUnknown, Err: err}, nil
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return ObjectStat{State: ObjectUnknown, Err: err}, nil
	}
	return ObjectStat{
		State:     ObjectPresent,
		SizeBytes: info.Size(),
		SHA256:    hex.EncodeToString(sum.Sum(nil)),
	}, nil
}

func (s *LocalStore) Delete(ctx context.Context, ref ObjectRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := s.resolveRef(ref)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	info, err := os.Lstat(resolved.finalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to delete symlink artifact %q", ref.Name)
	}
	if err := os.Remove(resolved.finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fsyncDir(filepath.Dir(resolved.finalPath))
}

func (s *LocalStore) DeletePrefix(ctx context.Context, prefix ArtifactPrefix) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	base, _, err := s.baseForKind(prefix.Kind)
	if err != nil {
		return err
	}
	if err := validateUUID("user_id", prefix.UserID); err != nil {
		return fmt.Errorf("unsafe prefix: %w", err)
	}
	if err := validateUUID("job_id", prefix.JobID); err != nil {
		return fmt.Errorf("unsafe prefix: %w", err)
	}
	parent := filepath.Join(base, prefix.UserID)
	if err := checkDirChainNoSymlink(base, []string{prefix.UserID}); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	target := filepath.Join(parent, prefix.JobID)
	if info, err := os.Lstat(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to delete symlink prefix %q", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return fsyncDir(parent)
}

func (w *localObjectWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("artifact writer is closed")
	}
	n, err := w.file.Write(p)
	if n > 0 {
		_, _ = w.hasher.Write(p[:n])
		w.sizeBytes += int64(n)
	}
	return n, err
}

func (w *localObjectWriter) Commit(ctx context.Context) (ObjectRef, error) {
	if err := ctx.Err(); err != nil {
		return ObjectRef{}, err
	}
	if w.closed {
		return ObjectRef{}, errors.New("artifact writer is closed")
	}
	w.closed = true
	if err := w.file.Sync(); err != nil {
		_ = w.file.Close()
		_ = os.Remove(w.tempPath)
		return ObjectRef{}, err
	}
	if err := w.file.Close(); err != nil {
		_ = os.Remove(w.tempPath)
		return ObjectRef{}, err
	}
	if err := os.Rename(w.tempPath, w.finalPath); err != nil {
		_ = os.Remove(w.tempPath)
		return ObjectRef{}, err
	}
	if err := fsyncDir(filepath.Dir(w.finalPath)); err != nil {
		return ObjectRef{}, err
	}
	now := time.Now().UTC()
	return ObjectRef{
		Backend:     localBackend,
		Kind:        w.spec.Kind,
		UserID:      w.spec.UserID,
		JobID:       w.spec.JobID,
		Role:        w.spec.Role,
		Name:        w.spec.Name,
		Path:        w.relPath,
		SHA256:      hex.EncodeToString(w.hasher.Sum(nil)),
		SizeBytes:   w.sizeBytes,
		ContentType: w.spec.ContentType,
		CreatedAt:   now,
		ExpiresAt:   w.spec.ExpiresAt,
	}, nil
}

func (w *localObjectWriter) Abort(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !w.closed {
		w.closed = true
		if err := w.file.Close(); err != nil {
			_ = os.Remove(w.tempPath)
			return err
		}
	}
	if err := os.Remove(w.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r *localObjectReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *localObjectReader) Close() error {
	return r.file.Close()
}

func (r *localObjectReader) Size() int64 {
	return r.sizeBytes
}

func (r *localObjectReader) ContentType() string {
	return r.contentType
}

func (r *localReaderAt) Size() int64 {
	return r.sizeBytes
}

type resolvedLocalPath struct {
	finalPath string
	relPath   string
}

func (s *LocalStore) resolveSpec(spec ObjectSpec, createParents bool) (resolvedLocalPath, error) {
	base, label, err := s.baseForKind(spec.Kind)
	if err != nil {
		return resolvedLocalPath{}, err
	}
	if err := validateUUID("user_id", spec.UserID); err != nil {
		return resolvedLocalPath{}, err
	}
	if err := validateUUID("job_id", spec.JobID); err != nil {
		return resolvedLocalPath{}, err
	}
	nameParts, cleanName, err := validateObjectName(spec.Name)
	if err != nil {
		return resolvedLocalPath{}, err
	}
	parentParts := append([]string{spec.UserID, spec.JobID}, nameParts[:len(nameParts)-1]...)
	if createParents {
		if err := ensureDirChainNoSymlink(base, parentParts); err != nil {
			return resolvedLocalPath{}, err
		}
	} else if err := checkDirChainNoSymlink(base, parentParts); err != nil {
		return resolvedLocalPath{}, err
	}
	return resolvedLocalPath{
		finalPath: filepath.Join(append([]string{base, spec.UserID, spec.JobID}, nameParts...)...),
		relPath:   filepath.ToSlash(filepath.Join(label, spec.UserID, spec.JobID, cleanName)),
	}, nil
}

func (s *LocalStore) resolveRef(ref ObjectRef) (resolvedLocalPath, error) {
	if ref.Backend != "" && ref.Backend != localBackend {
		return resolvedLocalPath{}, fmt.Errorf("unsupported artifact backend %q", ref.Backend)
	}
	spec := ObjectSpec{
		Kind:   ref.Kind,
		UserID: ref.UserID,
		JobID:  ref.JobID,
		Role:   ref.Role,
		Name:   ref.Name,
	}
	resolved, err := s.resolveSpec(spec, false)
	if err != nil {
		return resolvedLocalPath{}, err
	}
	if ref.Path != "" && filepath.IsAbs(ref.Path) {
		return resolvedLocalPath{}, fmt.Errorf("artifact path must be relative")
	}
	return resolved, nil
}

func (s *LocalStore) baseForKind(kind string) (string, string, error) {
	switch kind {
	case "export":
		return s.exportsBase, "exports", nil
	case "import":
		return s.importsBase, "imports", nil
	default:
		return "", "", fmt.Errorf("unsupported artifact kind %q", kind)
	}
}

func validateUUID(field, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("%s must be a UUID", field)
	}
	return nil
}

func validateObjectName(name string) ([]string, string, error) {
	if name == "" {
		return nil, "", errors.New("object name is required")
	}
	if filepath.IsAbs(name) || path.IsAbs(name) {
		return nil, "", fmt.Errorf("object name must be relative: %q", name)
	}
	if strings.Contains(name, "\\") {
		return nil, "", fmt.Errorf("object name contains invalid separator: %q", name)
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, "", fmt.Errorf("object name contains unsafe component: %q", name)
		}
	}
	cleanName := path.Clean(name)
	if cleanName == "." || cleanName != name {
		return nil, "", fmt.Errorf("object name is not canonical: %q", name)
	}
	return parts, cleanName, nil
}

func ensurePrivateBaseDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		return os.Chmod(dir, 0o700)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("base directory must not be a symlink: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("base path is not a directory: %s", dir)
	}
	return os.Chmod(dir, 0o700)
}

func ensureDirChainNoSymlink(base string, parts []string) error {
	if err := ensurePrivateBaseDir(base); err != nil {
		return err
	}
	current := base
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component must not be a symlink: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("path component is not a directory: %s", current)
		}
		if err := os.Chmod(current, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func checkDirChainNoSymlink(base string, parts []string) error {
	info, err := os.Lstat(base)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("base directory must not be a symlink: %s", base)
	}
	if !info.IsDir() {
		return fmt.Errorf("base path is not a directory: %s", base)
	}
	current := base
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component must not be a symlink: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("path component is not a directory: %s", current)
		}
	}
	return nil
}

func openRegularNoFollow(filename string) (*os.File, os.FileInfo, error) {
	fd, err := unix.Open(filename, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if err == unix.ENOENT {
			return nil, nil, os.ErrNotExist
		}
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), filename)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("artifact is not a regular file: %s", filename)
	}
	return file, info, nil
}

func normalizeRange(rangeSpec *RangeSpec, size int64) (int64, int64, error) {
	if rangeSpec == nil {
		return 0, size, nil
	}
	start := int64(0)
	end := size
	if rangeSpec.Start != nil {
		start = *rangeSpec.Start
	}
	if rangeSpec.End != nil {
		end = *rangeSpec.End + 1
	}
	if start < 0 || end < start {
		return 0, 0, fmt.Errorf("invalid byte range start=%d end=%d", start, end)
	}
	if end > size {
		end = size
	}
	return start, end, nil
}

func tempPathFor(finalPath string) (string, error) {
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(finalPath), filepath.Base(finalPath)+".tmp-"+hex.EncodeToString(suffix[:])), nil
}

func fsyncDir(dir string) error {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}
