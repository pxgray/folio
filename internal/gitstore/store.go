package gitstore

import (
	"fmt"
	"log"
	"os"
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
}

// New creates a Store. Call EnsureRepos (or AddRepo) before serving.
func New(cacheDir string, defaultStaleTTL time.Duration) *Store {
	return &Store{
		cacheDir:        cacheDir,
		defaultStaleTTL: defaultStaleTTL,
		repos:           make(map[string]*Repo),
		locals:          make(map[string]*LocalRepo),
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

// LocalEntry describes a local filesystem repo to register with the Store.
type LocalEntry struct {
	Label       string
	Path        string
	TrustedHTML bool
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

// OpenLocals registers local filesystem repos. Should be called once at startup
// for any TOML-configured local repos (deprecated path; new repos use db.Store).
func (s *Store) OpenLocals(locals []LocalEntry) error {
	for _, lc := range locals {
		if _, exists := s.locals[lc.Label]; exists {
			return fmt.Errorf("local repo: duplicate label %q", lc.Label)
		}
		if _, err := os.Stat(lc.Path); err != nil {
			return fmt.Errorf("local repo %q: %w", lc.Label, err)
		}
		s.locals[lc.Label] = newLocalRepo(lc.Path)
		log.Printf("folio: registered local repo %q at %s", lc.Label, lc.Path)
	}
	return nil
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
