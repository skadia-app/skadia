package shadowfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if err := ValidateRelPath(b.basePath, path); err != nil {
		return 0, "", fmt.Errorf("path traversal blocked: %w", err)
	}
	full := filepath.Join(b.basePath, filepath.Clean(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, "", fmt.Errorf("mkdir: %w", err)
	}

	flags := os.O_WRONLY | os.O_CREATE
	if exclusive {
		flags |= os.O_EXCL // fail if file already exists
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(full, flags, 0o644)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	h := sha256.New()
	mw := io.MultiWriter(f, h)
	written, err := io.Copy(mw, data)
	if err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(h.Sum(nil)), nil
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
