package gitstore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store manages all registered repositories (remote bare clones and local working trees).
type Store struct {
	cacheDir        string
	defaultStaleTTL time.Duration
	mu              sync.RWMutex // protects repos map
	repos           map[string]*Repo
	locals          map[string]*LocalRepo
	perRepoStaleTTL map[string]time.Duration
}

// New creates a Store. Call EnsureRepos (or AddRepo) before serving.
func New(cacheDir string, defaultStaleTTL time.Duration) *Store {
	return &Store{
		cacheDir:        cacheDir,
		defaultStaleTTL: defaultStaleTTL,
		repos:           make(map[string]*Repo),
		locals:          make(map[string]*LocalRepo),
		perRepoStaleTTL: make(map[string]time.Duration),
	}
}

// RepoEntry describes a remote repository to register with the Store.
type RepoEntry struct {
	Host      string        // e.g. "github.com"
	Owner     string        // e.g. "acme"
	Name      string        // e.g. "docs"
	RemoteURL string        // empty = infer from Host/Owner/Name
	StaleTTL  time.Duration // 0 = use store default
}

func (e RepoEntry) key() string { return e.Host + "/" + e.Owner + "/" + e.Name }

func (e RepoEntry) cloneURL() string {
	if e.RemoteURL != "" {
		return e.RemoteURL
	}
	return "https://" + e.Host + "/" + e.Owner + "/" + e.Name + ".git"
}

// AddRepo registers a repo; clones if not on disk, opens if already cloned.
// No-op (returns nil) if the key is already registered. Thread-safe.
func (s *Store) AddRepo(ctx context.Context, e RepoEntry) error {
	key := e.key()

	// Fast path: check if already registered (read lock only).
	s.mu.RLock()
	if repo, ok := s.repos[key]; ok {
		s.mu.RUnlock()
		repo.triggerBackgroundFetch(ctx)
		return nil
	}
	s.mu.RUnlock()

	// Slow path: clone/open outside the lock.
	staleTTL := e.StaleTTL
	if staleTTL == 0 {
		staleTTL = s.defaultStaleTTL
	}

	localDir := filepath.Join(s.cacheDir, e.Host, e.Owner, e.Name)
	repo := newRepo(e.cloneURL(), localDir, staleTTL)

	if _, err := os.Stat(localDir); errors.Is(err, os.ErrNotExist) {
		log.Printf("folio: cloning %s into %s", e.cloneURL(), localDir)
		if err := os.MkdirAll(filepath.Dir(localDir), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(localDir), err)
		}
		if err := repo.clone(ctx); err != nil {
			return fmt.Errorf("clone %s: %w", key, err)
		}
		log.Printf("folio: cloned %s", key)
	} else {
		log.Printf("folio: opening %s from %s", key, localDir)
		if err := repo.open(); err != nil {
			return fmt.Errorf("open %s: %w", key, err)
		}
	}

	// Insert into map before spawning goroutine (double-check after acquiring write lock).
	s.mu.Lock()
	if existing, ok := s.repos[key]; ok {
		s.mu.Unlock()
		repo.Close()
		existing.triggerBackgroundFetch(ctx)
		return nil
	}
	s.repos[key] = repo
	s.mu.Unlock()

	// Now safe to spawn: repo is in the map.
	if _, err := os.Stat(localDir); errors.Is(err, os.ErrNotExist) {
		// Fresh clone — no background fetch needed.
		return nil
	}
	repo.triggerBackgroundFetch(ctx)
	return nil
}

// RepoEntries returns a snapshot of all registered remote repo entries.
// Intended for testing and diagnostics.
func (s *Store) RepoEntries() []RepoEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RepoEntry, 0, len(s.repos))
	for key, repo := range s.repos {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		out = append(out, RepoEntry{
			Host:      parts[0],
			Owner:     parts[1],
			Name:      parts[2],
			RemoteURL: repo.CloneURL(),
		})
	}
	return out
}

// RemoveRepo unregisters a repo. The cache directory is left on disk.
// Thread-safe. No-op if the key is not registered.
func (s *Store) RemoveRepo(host, owner, name string) {
	key := host + "/" + owner + "/" + name
	s.mu.Lock()
	delete(s.repos, key)
	s.mu.Unlock()
}

// EnsureRepos registers all entries, cloning or opening as needed.
// Replaces the old EnsureCloned. Thread-safe.
func (s *Store) EnsureRepos(ctx context.Context, entries []RepoEntry) error {
	for _, e := range entries {
		if err := s.AddRepo(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// Get returns the Repository for the given host/owner/repo triple.
// Returns ErrNotRegistered if the repo is not registered.
func (s *Store) Get(host, owner, repo string) (Repository, error) {
	key := host + "/" + owner + "/" + repo
	s.mu.RLock()
	r, ok := s.repos[key]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, key)
	}
	return r, nil
}

// GetLocal returns the Repository for the given local label.
// Returns ErrNotRegistered if the label is not registered.
func (s *Store) GetLocal(label string) (Repository, error) {
	s.mu.RLock()
	r, ok := s.locals[label]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: local/%s", ErrNotRegistered, label)
	}
	return r, nil
}

// RegisterLocal registers a single local filesystem repo with the Store.
// Used when loading local repos from the DB at startup.
func (s *Store) RegisterLocal(label, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.locals[label]; exists {
		return
	}
	if _, err := os.Stat(path); err != nil {
		log.Printf("folio: registerLocal %q: path %q not accessible: %v", label, path, err)
		return
	}
	s.locals[label] = newLocalRepo(path)
	log.Printf("folio: registered local repo %q at %s", label, path)
}

// RemoveLocal unregisters a local repo by label.
// Thread-safe. No-op if the label is not registered.
func (s *Store) RemoveLocal(label string) {
	s.mu.Lock()
	delete(s.locals, label)
	s.mu.Unlock()
}

// RepoKeys returns the registered host/owner/name keys (used by the index page).
func (s *Store) RepoKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.repos))
	for k := range s.repos {
		out = append(out, k)
	}
	return out
}

// LocalLabels returns the registered local repo labels (used by the index page).
func (s *Store) LocalLabels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.locals))
	for k := range s.locals {
		out = append(out, k)
	}
	return out
}

// SetRepoStaleTTL sets a per-repo stale TTL override and applies it
// to the registered repo (if any). Pass zero to remove the override
// and fall back to the store default.
func (s *Store) SetRepoStaleTTL(host, owner, repo string, ttl time.Duration) {
	key := host + "/" + owner + "/" + repo
	s.mu.Lock()
	if ttl == 0 {
		delete(s.perRepoStaleTTL, key)
	} else {
		s.perRepoStaleTTL[key] = ttl
	}
	if r, ok := s.repos[key]; ok {
		r.SetStaleTTL(ttl)
	}
	s.mu.Unlock()
}

// resolveStaleTTL returns the effective stale TTL for a repo key,
// checking per-repo overrides first, then falling back to the store default.
func (s *Store) resolveStaleTTL(key string) time.Duration {
	s.mu.RLock()
	ttl, ok := s.perRepoStaleTTL[key]
	s.mu.RUnlock()
	if ok {
		return ttl
	}
	return s.defaultStaleTTL
}
