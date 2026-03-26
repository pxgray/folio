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

// Store manages all registered bare-clone repositories.
type Store struct {
	cfg   *config.Config
	repos map[string]*Repo // keyed by RepoConfig.Key()
}

// New creates a Store from the given config. Call EnsureCloned before serving.
func New(cfg *config.Config) *Store {
	return &Store{
		cfg:   cfg,
		repos: make(map[string]*Repo, len(cfg.Repos)),
	}
}

// EnsureCloned initialises all repos: bare-clones those missing from disk,
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
		}

		s.repos[rc.Key()] = repo
	}
	return nil
}

// Get returns the Repo for the given host/owner/repo triple.
// Returns ErrNotRegistered if the repo is not in the config.
func (s *Store) Get(host, owner, repo string) (*Repo, error) {
	key := host + "/" + owner + "/" + repo
	r, ok := s.repos[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, key)
	}
	return r, nil
}

// Repos returns all registered repos (used by the index page).
func (s *Store) Repos() []*config.RepoConfig {
	out := make([]*config.RepoConfig, 0, len(s.cfg.Repos))
	for i := range s.cfg.Repos {
		out = append(out, &s.cfg.Repos[i])
	}
	return out
}

