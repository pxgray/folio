package gitstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func makeLocalTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "docs", "index.md"), []byte("# Docs\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "logo.png"), []byte("\x89PNG"), 0o644)
	return dir
}

func TestLocalRepo_ResolveRef(t *testing.T) {
	r := newLocalRepo(t.TempDir())
	hash, err := r.ResolveRef(context.Background(), "main")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if hash != plumbing.ZeroHash {
		t.Errorf("expected ZeroHash, got %s", hash)
	}
}

func TestLocalRepo_ReadBlob(t *testing.T) {
	dir := makeLocalTestDir(t)
	r := newLocalRepo(dir)

	data, err := r.ReadBlob(plumbing.ZeroHash, "README.md")
	if err != nil {
		t.Fatalf("ReadBlob: %v", err)
	}
	if string(data) != "# Hello\n" {
		t.Errorf("data = %q, want \"# Hello\\n\"", data)
	}
}

func TestLocalRepo_ReadBlob_NotFound(t *testing.T) {
	r := newLocalRepo(t.TempDir())
	_, err := r.ReadBlob(plumbing.ZeroHash, "nonexistent.md")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalRepo_ReadTree(t *testing.T) {
	dir := makeLocalTestDir(t)
	r := newLocalRepo(dir)

	entries, err := r.ReadTree(plumbing.ZeroHash, "")
	if err != nil {
		t.Fatalf("ReadTree: %v", err)
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["README.md"] {
		t.Error("expected README.md in root entries")
	}
	if !names["docs"] {
		t.Error("expected docs/ in root entries")
	}
}

func TestLocalRepo_ReadTree_NotFound(t *testing.T) {
	r := newLocalRepo(t.TempDir())
	_, err := r.ReadTree(plumbing.ZeroHash, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalRepo_FetchNow(t *testing.T) {
	r := newLocalRepo(t.TempDir())
	if err := r.FetchNow(context.Background()); err != nil {
		t.Errorf("FetchNow: %v", err)
	}
}
