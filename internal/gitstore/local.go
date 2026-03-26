package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// LocalRepo serves a live working tree from disk. The hash parameter in all
// methods is ignored; reads go directly to the filesystem.
type LocalRepo struct {
	root string
}

func newLocalRepo(root string) *LocalRepo {
	return &LocalRepo{root: root}
}

// ResolveRef always returns ZeroHash. The ref parameter is ignored.
func (r *LocalRepo) ResolveRef(_ context.Context, _ string) (plumbing.Hash, error) {
	return plumbing.ZeroHash, nil
}

// ReadBlob reads the file at path relative to the repo root.
func (r *LocalRepo) ReadBlob(_ plumbing.Hash, path string) ([]byte, error) {
	abs := filepath.Join(r.root, path)
	if !strings.HasPrefix(abs, r.root+string(filepath.Separator)) && abs != r.root {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		// Directories are not blobs (os.ReadFile returns EISDIR on Linux).
		if fi, statErr := os.Stat(abs); statErr == nil && fi.IsDir() {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// ReadTree lists the directory at path relative to the repo root.
// If path is empty, the root directory is listed.
func (r *LocalRepo) ReadTree(_ plumbing.Hash, path string) ([]TreeEntry, error) {
	abs := filepath.Join(r.root, path)
	if !strings.HasPrefix(abs, r.root+string(filepath.Separator)) && abs != r.root {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readdir %s: %w", path, err)
	}
	result := make([]TreeEntry, 0, len(entries))
	for _, e := range entries {
		te := TreeEntry{Name: e.Name(), IsDir: e.IsDir()}
		if !te.IsDir {
			if info, err := e.Info(); err == nil {
				te.Size = info.Size()
			}
		}
		result = append(result, te)
	}
	return result, nil
}

// FetchNow is a no-op for local repos.
func (r *LocalRepo) FetchNow(_ context.Context) error {
	return nil
}
