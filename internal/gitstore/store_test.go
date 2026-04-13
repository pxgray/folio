package gitstore_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	_ "github.com/go-git/go-git/v5/plumbing/transport/file"
	"github.com/pxgray/folio/internal/gitstore"
)

func makeTestBareRepo(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()

	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := work.Worktree()

	path := filepath.Join(workDir, "README.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("# Test\n"), 0o644)

	_ = wt.AddGlob(".")
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	git.PlainInit(bareDir, true)
	work.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{bareDir}})
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	return bareDir
}

func TestNew_EmptyStore(t *testing.T) {
	s := gitstore.New(t.TempDir(), 5*time.Minute)
	if s == nil {
		t.Fatal("New returned nil")
	}
	// An empty store returns ErrNotRegistered for any key.
	_, err := s.Get("example.com", "owner", "repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAddRepo_RegistersKey(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	s := gitstore.New(t.TempDir(), 5*time.Minute)

	err := s.AddRepo(t.Context(), gitstore.RepoEntry{
		Host:      "example.com",
		Owner:     "testuser",
		Name:      "docs",
		RemoteURL: "file://" + bareDir,
	})
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	repo, err := s.Get("example.com", "testuser", "docs")
	if err != nil {
		t.Fatalf("Get after AddRepo: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestAddRepo_NoOpOnDuplicate(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	s := gitstore.New(t.TempDir(), 5*time.Minute)
	entry := gitstore.RepoEntry{
		Host: "example.com", Owner: "testuser", Name: "docs",
		RemoteURL: "file://" + bareDir,
	}

	if err := s.AddRepo(t.Context(), entry); err != nil {
		t.Fatalf("first AddRepo: %v", err)
	}
	// Second call must be a no-op — no error, no panic.
	if err := s.AddRepo(t.Context(), entry); err != nil {
		t.Fatalf("second AddRepo (no-op): %v", err)
	}

	// Still exactly one registration.
	repos := s.RepoEntries()
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestRemoveRepo_RemovesKey(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	s := gitstore.New(t.TempDir(), 5*time.Minute)

	err := s.AddRepo(t.Context(), gitstore.RepoEntry{
		Host:      "example.com",
		Owner:     "testuser",
		Name:      "docs",
		RemoteURL: "file://" + bareDir,
	})
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	s.RemoveRepo("example.com", "testuser", "docs")

	_, err = s.Get("example.com", "testuser", "docs")
	if !errors.Is(err, gitstore.ErrNotRegistered) {
		t.Errorf("expected ErrNotRegistered after RemoveRepo, got %v", err)
	}
}

func TestStore_ConcurrentAddRemoveGet(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	s := gitstore.New(t.TempDir(), 5*time.Minute)

	// Pre-populate via clone path (clone does NOT start a background fetch goroutine).
	if err := s.AddRepo(t.Context(), gitstore.RepoEntry{
		Host: "example.com", Owner: "u", Name: "r",
		RemoteURL: "file://" + bareDir,
	}); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_, _ = s.Get("example.com", "u", "r")
		}
	}()

	for i := 0; i < 50; i++ {
		s.RemoveRepo("example.com", "u", "r")
		// Use a unique name each iteration so AddRepo takes the clone path
		// (not the open+background-fetch path), avoiding goroutines that would
		// outlive the test and race with TempDir cleanup.
		_ = s.AddRepo(t.Context(), gitstore.RepoEntry{
			Host: "example.com", Owner: "u", Name: fmt.Sprintf("r%d", i),
			RemoteURL: "file://" + bareDir,
		})
	}

	<-done
}
