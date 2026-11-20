package shadowfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// LocalBackend implements StorageBackend for the local filesystem.
type LocalBackend struct {
	basePath string
}

// NewLocalBackend creates a LocalBackend rooted at basePath.
func NewLocalBackend(basePath string) *LocalBackend {
	return &LocalBackend{basePath: basePath}
}

// BasePath returns the root directory.
func (b *LocalBackend) BasePath() string { return b.basePath }

func (b *LocalBackend) Read(ctx context.Context, path string, opts *BackendOpts) (io.ReadCloser, error) {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return nil, fmt.Errorf("path traversal blocked: %w", err)
	}
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Clean(path))
}

func (b *LocalBackend) Write(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, opts *BackendOpts) (int64, string, error) {
	return b.WriteStream(ctx, path, data, contentType, exclusive, "", opts)
}

func (b *LocalBackend) WriteStream(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, expectedHash string, opts *BackendOpts) (int64, string, error) {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return 0, "", fmt.Errorf("path traversal blocked: %w", err)
	}
	if err := os.MkdirAll(b.basePath, 0o755); err != nil {
		return 0, "", fmt.Errorf("mkdir base: %w", err)
	}
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		return 0, "", fmt.Errorf("open root: %w", err)
	}
	defer root.Close()

	rel := filepath.Clean(path)
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return 0, "", fmt.Errorf("mkdir: %w", err)
		}
	}
	if exclusive {
		if _, err := root.Stat(rel); err == nil {
			return 0, "", os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, "", err
		}
	}

	tmpRel := filepath.Join(filepath.Dir(rel), "."+filepath.Base(rel)+".tmp-"+uuid.NewString())
	tmp, err := root.OpenFile(tmpRel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, "", err
	}
	removeTmp := true
	defer func() {
		if removeTmp {
			if rmErr := root.Remove(tmpRel); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				slog.Warn("shadowfs: cleanup temp file failed", "path", tmpRel, "err", rmErr)
			}
		}
	}()

	h := sha256.New()
	mw := io.MultiWriter(tmp, h)
	written, copyErr := io.Copy(mw, data)
	if closeErr := tmp.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return 0, "", copyErr
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if expectedHash != "" && !strings.EqualFold(expectedHash, hash) {
		return 0, "", fmt.Errorf("sha256 mismatch: expected %s got %s", expectedHash, hash)
	}
	if err := root.Chmod(tmpRel, 0o644); err != nil {
		return 0, "", fmt.Errorf("chmod temp: %w", err)
	}
	if ctx.Err() != nil {
		return 0, "", ctx.Err()
	}
	if exclusive {
		if err := root.Link(tmpRel, rel); err != nil {
			return 0, "", err
		}
	} else if err := root.Rename(tmpRel, rel); err != nil {
		return 0, "", err
	}
	removeTmp = true
	return written, hash, nil
}

func (b *LocalBackend) Delete(ctx context.Context, path string, opts *BackendOpts) error {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return fmt.Errorf("path traversal blocked: %w", err)
	}
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer root.Close()
	err = root.Remove(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (b *LocalBackend) Exists(ctx context.Context, path string, opts *BackendOpts) (bool, error) {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return false, fmt.Errorf("path traversal blocked: %w", err)
	}
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer root.Close()
	_, err = root.Stat(filepath.Clean(path))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (b *LocalBackend) DeletePrefix(ctx context.Context, prefix string, opts *BackendOpts) error {
	if err := ValidateRelPath(b.basePath, prefix); err != nil {
		return fmt.Errorf("path traversal blocked: %w", err)
	}
	cleaned := filepath.Clean(prefix)
	// Refuse to delete the entire basePath
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("refusing to delete basePath")
	}
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer root.Close()
	return root.RemoveAll(cleaned)
}

func (b *LocalBackend) ListFunc(ctx context.Context, prefix string, opts *BackendOpts, fn func(string) error) error {
	if err := ValidateRelPath(b.basePath, prefix); err != nil {
		return fmt.Errorf("path traversal blocked: %w", err)
	}
	root := filepath.Join(b.basePath, filepath.Clean(prefix))
	var count int
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if d.IsDir() {
			return nil
		}
		// Check context cancellation every 1000 files
		count++
		if count%1000 == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		rel, _ := filepath.Rel(b.basePath, path)
		return fn(rel)
	})
}

var _ StorageBackend = (*LocalBackend)(nil)
var _ StreamingBackend = (*LocalBackend)(nil)
