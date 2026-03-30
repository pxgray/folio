package gitstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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

// makeUpstreamAndClone sets up a two-tier git environment: an "upstream" bare
// repo (like GitHub) and a folio-style bare clone of it. Returns the Repo and
// a helper to push new commits to upstream.
func makeUpstreamAndClone(t *testing.T) (*Repo, func(content string)) {
	t.Helper()

	upstreamDir := t.TempDir()
	if _, err := git.PlainInit(upstreamDir, true); err != nil {
		t.Fatalf("init upstream: %v", err)
	}

	workDir := t.TempDir()
	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init work: %v", err)
	}
	wt, _ := work.Worktree()
	writeFile(t, wt.Filesystem.Root(), "README.md", "initial")
	_ = wt.AddGlob(".")
	if _, err := wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	if _, err := work.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin", URLs: []string{upstreamDir},
	}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("initial push: %v", err)
	}

	localDir := t.TempDir()
	if err := os.RemoveAll(localDir); err != nil {
		t.Fatalf("remove localDir: %v", err)
	}
	r := newRepo("file://"+upstreamDir, localDir, 5*time.Minute)
	if err := r.clone(context.Background()); err != nil {
		t.Fatalf("clone: %v", err)
	}

	push := func(content string) {
		t.Helper()
		writeFile(t, wt.Filesystem.Root(), "README.md", content)
		_ = wt.AddGlob(".")
		if _, err := wt.Commit(content, &git.CommitOptions{
			Author: &object.Signature{Name: "test", Email: "test@test.com"},
		}); err != nil {
			t.Fatalf("commit %q: %v", content, err)
		}
		if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
			t.Fatalf("push %q: %v", content, err)
		}
	}

	return r, push
}

// TestFetchNowSeesNewCommits verifies that FetchNow picks up new commits from the
// remote and that the default HEAD view (no ?ref=) reflects them.
func TestFetchNowSeesNewCommits(t *testing.T) {
	r, push := makeUpstreamAndClone(t)
	ctx := context.Background()

	h1, err := r.ResolveRef(ctx, "")
	if err != nil {
		t.Fatalf("ResolveRef initial: %v", err)
	}

	push("v2 content")

	if err := r.FetchNow(ctx); err != nil {
		t.Fatalf("FetchNow: %v", err)
	}

	h2, err := r.ResolveRef(ctx, "")
	if err != nil {
		t.Fatalf("ResolveRef after fetch: %v", err)
	}
	if h1 == h2 {
		t.Errorf("hash unchanged after push+FetchNow (default ref): %s", h1)
	}

	blob, err := r.ReadBlob(h2, "README.md")
	if err != nil {
		t.Fatalf("ReadBlob: %v", err)
	}
	if string(blob) != "v2 content" {
		t.Errorf("README.md: want %q got %q", "v2 content", blob)
	}
}

// TestFetchNowSeesNewCommitsExplicitRef verifies that FetchNow also updates
// refs visible via explicit ?ref=<branch> queries.
func TestFetchNowSeesNewCommitsExplicitRef(t *testing.T) {
	r, push := makeUpstreamAndClone(t)
	ctx := context.Background()

	// Discover the branch name by resolving HEAD.
	h1, err := r.ResolveRef(ctx, "")
	if err != nil {
		t.Fatalf("ResolveRef initial HEAD: %v", err)
	}

	// Find the branch name that this hash lives on.
	refs, err := r.r.References()
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	var branch string
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() && ref.Hash() == h1 {
			branch = ref.Name().Short()
		}
		return nil
	})
	if branch == "" {
		t.Skip("could not find branch name for HEAD, skipping explicit-ref test")
	}

	push("v2 content")

	if err := r.FetchNow(ctx); err != nil {
		t.Fatalf("FetchNow: %v", err)
	}

	h2, err := r.ResolveRef(ctx, branch)
	if err != nil {
		t.Fatalf("ResolveRef(%q) after fetch: %v", branch, err)
	}
	if h1 == h2 {
		t.Errorf("explicit ?ref=%s hash unchanged after push+FetchNow: %s — refs/heads/* not updated by fetch", branch, h1)
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
