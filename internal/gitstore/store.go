package gitstore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pxgray/folio/internal/config"
)

// Store manages all registered repositories (remote bare clones and local working trees).
type Store struct {
	cfg    *config.Config
	repos  map[string]*Repo
	locals map[string]*LocalRepo
}

// New creates a Store from the given config. Call EnsureCloned and OpenLocals before serving.
func New(cfg *config.Config) *Store {
	return &Store{
		cfg:    cfg,
		repos:  make(map[string]*Repo, len(cfg.Repos)),
		locals: make(map[string]*LocalRepo, len(cfg.Locals)),
	}
}

// EnsureCloned initialises all remote repos: bare-clones those missing from disk,
// and opens existing ones. Should be called once at startup.
func (s *Store) EnsureCloned(ctx context.Context) error {
	for _, rc := range s.cfg.Repos {
		localDir := filepath.Join(s.cfg.Cache.Dir, rc.Host, rc.Owner, rc.Repo)
		repo := newRepo(rc.CloneURL(), localDir, s.cfg.Cache.StaleTTL)

		if _, err := os.Stat(localDir); errors.Is(err, os.ErrNotExist) {
			log.Printf("folio: cloning %s into %s", rc.CloneURL(), localDir)
			if err := os.MkdirAll(filepath.Dir(localDir), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(localDir), err)
			}
			if err := repo.clone(ctx); err != nil {
				return fmt.Errorf("clone %s: %w", rc.Key(), err)
			}
			log.Printf("folio: cloned %s", rc.Key())
		} else {
			log.Printf("folio: opening %s from %s", rc.Key(), localDir)
			if err := repo.open(); err != nil {
				return fmt.Errorf("open %s: %w", rc.Key(), err)
			}
			// Fetch in the background so stale caches from before the server
			// restarted are refreshed without blocking startup.
			go repo.triggerBackgroundFetch(context.Background())
		}

		s.repos[rc.Key()] = repo
	}
	return nil
}

// OpenLocals validates and registers all local repos from config.
// Should be called once at startup.
func (s *Store) OpenLocals() error {
	for _, lc := range s.cfg.Locals {
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

// Get returns the Repository for the given host/owner/repo triple.
// Returns ErrNotRegistered if the repo is not in the config.
func (s *Store) Get(host, owner, repo string) (Repository, error) {
	key := host + "/" + owner + "/" + repo
	r, ok := s.repos[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, key)
	}
	return r, nil
}

// GetLocal returns the Repository for the given local label.
// Returns ErrNotRegistered if the label is not in the config.
func (s *Store) GetLocal(label string) (Repository, error) {
	r, ok := s.locals[label]
	if !ok {
		return nil, fmt.Errorf("%w: local/%s", ErrNotRegistered, label)
	}
	return r, nil
}

// Repos returns all registered remote repo configs (used by the index page).
func (s *Store) Repos() []*config.RepoConfig {
	out := make([]*config.RepoConfig, 0, len(s.cfg.Repos))
	for i := range s.cfg.Repos {
		out = append(out, &s.cfg.Repos[i])
	}
	return out
}

// Locals returns all registered local repo configs (used by the index page).
func (s *Store) Locals() []*config.LocalConfig {
	out := make([]*config.LocalConfig, 0, len(s.cfg.Locals))
	for i := range s.cfg.Locals {
		out = append(out, &s.cfg.Locals[i])
	}
	return out
}
