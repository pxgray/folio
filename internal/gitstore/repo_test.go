package gitstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// makeTestRepo creates a bare git repo in a temp dir with a few files and
// returns the path plus the commit hash.
func makeTestRepo(t *testing.T) (bareDir string) {
	t.Helper()

	// Create a non-bare repo to work with, then push to bare.
	workDir := t.TempDir()
	bareDir = t.TempDir()

	// Init working repo.
	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init work repo: %v", err)
	}
	wt, _ := work.Worktree()
	fs := wt.Filesystem

	// Write files.
	writeFile(t, fs.Root(), "README.md", "# Hello\n\nThis is a test repo.\n")
	writeFile(t, fs.Root(), "docs/index.md", "# Docs\n\n[Setup](setup.md)\n")
	writeFile(t, fs.Root(), "docs/setup.md", "# Setup\n\nInstall with `go install`.\n")
	writeFile(t, fs.Root(), "static/logo.png", "\x89PNG\r\n\x1a\n") // fake PNG header
	_ = wt.AddGlob(".")

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Init bare repo and push.
	_, err = git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatalf("init bare repo: %v", err)
	}

	_, err = work.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	})
	if err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}

	return bareDir
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openTestRepo(t *testing.T, bareDir string) *Repo {
	t.Helper()
	r := newRepo("file://"+bareDir, bareDir, 5*time.Minute)
	if err := r.open(); err != nil {
		t.Fatalf("open repo: %v", err)
	}
	return r
}

func TestResolveRef(t *testing.T) {
	bareDir := makeTestRepo(t)
	r := openTestRepo(t, bareDir)

	hash, err := r.ResolveRef(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveRef HEAD: %v", err)
	}
	if hash.IsZero() {
		t.Error("ResolveRef returned zero hash")
	}
}

func TestReadBlob(t *testing.T) {
	bareDir := makeTestRepo(t)
	r := openTestRepo(t, bareDir)

	hash, err := r.ResolveRef(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}

	content, err := r.ReadBlob(hash, "README.md")
	if err != nil {
		t.Fatalf("ReadBlob README.md: %v", err)
	}
	if string(content) != "# Hello\n\nThis is a test repo.\n" {
		t.Errorf("unexpected README.md content: %q", content)
	}

	// Nested file.
	content, err = r.ReadBlob(hash, "docs/setup.md")
	if err != nil {
		t.Fatalf("ReadBlob docs/setup.md: %v", err)
	}
	if string(content) != "# Setup\n\nInstall with `go install`.\n" {
		t.Errorf("unexpected docs/setup.md content: %q", content)
	}

	// Non-existent file.
	_, err = r.ReadBlob(hash, "nonexistent.md")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent.md, got %v", err)
	}
}

func TestReadTree(t *testing.T) {
	bareDir := makeTestRepo(t)
	r := openTestRepo(t, bareDir)

	hash, err := r.ResolveRef(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}

	// Root listing.
	entries, err := r.ReadTree(hash, "")
	if err != nil {
		t.Fatalf("ReadTree root: %v", err)
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["README.md"] {
		t.Error("expected README.md in root listing")
	}
	if !names["docs"] {
		t.Error("expected docs/ in root listing")
	}

	// Sub-directory.
	entries, err = r.ReadTree(hash, "docs")
	if err != nil {
		t.Fatalf("ReadTree docs: %v", err)
	}
	names = make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["index.md"] || !names["setup.md"] {
		t.Errorf("docs entries: %v", entries)
	}

	// Non-existent dir.
	_, err = r.ReadTree(hash, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent dir, got %v", err)
	}
}

func TestHeadCaching(t *testing.T) {
	bareDir := makeTestRepo(t)
	r := openTestRepo(t, bareDir)

	ctx := context.Background()
	h1, err := r.ResolveRef(ctx, "")
	if err != nil {
		t.Fatalf("first ResolveRef: %v", err)
	}
	h2, err := r.ResolveRef(ctx, "")
	if err != nil {
		t.Fatalf("second ResolveRef: %v", err)
	}
	if h1 != h2 {
		t.Error("cache miss: got different hashes on consecutive calls")
	}
}
