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
	full := filepath.Join(b.basePath, filepath.Clean(path))
	return os.Open(full)
}

func (b *LocalBackend) Write(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, opts *BackendOpts) (int64, string, error) {
	return b.WriteStream(ctx, path, data, contentType, exclusive, "", opts)
}

func (b *LocalBackend) WriteStream(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, expectedHash string, opts *BackendOpts) (int64, string, error) {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return 0, "", fmt.Errorf("path traversal blocked: %w", err)
	}
	full := filepath.Join(b.basePath, filepath.Clean(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, "", fmt.Errorf("mkdir: %w", err)
	}
	if exclusive {
		if _, err := os.Stat(full); err == nil {
			return 0, "", os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, "", err
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), "."+filepath.Base(full)+".tmp-*")
	if err != nil {
		return 0, "", err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			if rmErr := os.Remove(tmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				slog.Warn("shadowfs: cleanup temp file failed", "path", tmpPath, "err", rmErr)
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
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return 0, "", fmt.Errorf("chmod temp: %w", err)
	}
	if ctx.Err() != nil {
		return 0, "", ctx.Err()
	}
	if exclusive {
		if err := os.Link(tmpPath, full); err != nil {
			return 0, "", err
		}
	} else if err := os.Rename(tmpPath, full); err != nil {
		return 0, "", err
	}
	removeTmp = true
	return written, hash, nil
}

func (b *LocalBackend) Delete(ctx context.Context, path string, opts *BackendOpts) error {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return fmt.Errorf("path traversal blocked: %w", err)
	}
	full := filepath.Join(b.basePath, filepath.Clean(path))
	err := os.Remove(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (b *LocalBackend) Exists(ctx context.Context, path string, opts *BackendOpts) (bool, error) {
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return false, fmt.Errorf("path traversal blocked: %w", err)
	}
	full := filepath.Join(b.basePath, filepath.Clean(path))
	_, err := os.Stat(full)
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
	root := filepath.Join(b.basePath, filepath.Clean(prefix))
	// Refuse to delete the entire basePath
	if root == b.basePath {
		return fmt.Errorf("refusing to delete basePath")
	}
	return os.RemoveAll(root)
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
