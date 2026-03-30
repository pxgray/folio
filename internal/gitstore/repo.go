package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TreeEntry describes one entry in a directory listing.
type TreeEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// headEntry caches a resolved ref → commit hash.
type headEntry struct {
	hash       plumbing.Hash
	resolvedAt time.Time
}

// Repo manages a single bare-cloned git repository.
type Repo struct {
	cloneURL string
	localDir string
	staleTTL time.Duration

	r *git.Repository // bare clone; opened once, safe for concurrent reads

	fetchMu   sync.Mutex // single in-flight fetch at a time
	lastFetch time.Time  // protected by fetchMu

	headMu    sync.RWMutex
	headCache map[string]headEntry // ref string → headEntry
}

func newRepo(cloneURL, localDir string, staleTTL time.Duration) *Repo {
	return &Repo{
		cloneURL:  cloneURL,
		localDir:  localDir,
		staleTTL:  staleTTL,
		headCache: make(map[string]headEntry),
	}
}

// open opens an already-cloned bare repo from disk.
func (r *Repo) open() error {
	repo, err := git.PlainOpen(r.localDir)
	if err != nil {
		return fmt.Errorf("open %s: %w", r.localDir, err)
	}
	r.r = repo
	return nil
}

// clone performs an initial bare clone from the remote.
func (r *Repo) clone(ctx context.Context) error {
	repo, err := git.PlainCloneContext(ctx, r.localDir, true, &git.CloneOptions{
		URL:          r.cloneURL,
		SingleBranch: false,
		Tags:         git.AllTags,
	})
	if err != nil {
		return fmt.Errorf("clone %s: %w", r.cloneURL, err)
	}
	r.r = repo
	return nil
}

// ResolveRef returns the commit hash for ref. If ref is empty, HEAD is used.
// Stale cached entries trigger a background fetch; the stale value is returned
// immediately (stale-while-revalidate). staleTTL == 0 means cache forever
// (webhooks are the only invalidation path).
func (r *Repo) ResolveRef(ctx context.Context, ref string) (plumbing.Hash, error) {
	key := ref
	if key == "" {
		key = "HEAD"
	}

	// Fast path: valid cached entry.
	r.headMu.RLock()
	entry, ok := r.headCache[key]
	r.headMu.RUnlock()

	if ok {
		if r.staleTTL == 0 || time.Since(entry.resolvedAt) < r.staleTTL {
			return entry.hash, nil
		}
		// Stale: return cached value and refresh in background.
		go r.triggerBackgroundFetch(context.Background())
		return entry.hash, nil
	}

	// Cache miss: resolve synchronously.
	hash, err := r.resolveViaGit(ref)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	r.headMu.Lock()
	r.headCache[key] = headEntry{hash: hash, resolvedAt: time.Now()}
	r.headMu.Unlock()

	return hash, nil
}

func (r *Repo) resolveViaGit(ref string) (plumbing.Hash, error) {
	if ref == "" {
		return r.resolveLocalHEAD()
	}

	hash, err := r.r.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	return *hash, nil
}

func (r *Repo) resolveLocalHEAD() (plumbing.Hash, error) {
	// Try remote-tracking branches first (normal bare clone from remote),
	// then local branches (bare repo that was pushed-to directly).
	candidates := []string{
		"refs/remotes/origin/HEAD",
		"refs/remotes/origin/main",
		"refs/remotes/origin/master",
		"refs/heads/main",
		"refs/heads/master",
	}
	for _, candidate := range candidates {
		hash, err := r.r.ResolveRevision(plumbing.Revision(candidate))
		if err == nil {
			return *hash, nil
		}
	}
	// Last resort: pick any branch.
	refs, err := r.r.References()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("list refs: %w", err)
	}
	var found plumbing.Hash
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		if (ref.Name().IsRemote() || ref.Name().IsBranch()) && found.IsZero() {
			found = ref.Hash()
		}
		return nil
	})
	if found.IsZero() {
		return plumbing.ZeroHash, fmt.Errorf("%w: HEAD", ErrNotFound)
	}
	return found, nil
}

// ReadBlob reads the raw bytes of the file at path in the commit identified by hash.
func (r *Repo) ReadBlob(hash plumbing.Hash, path string) ([]byte, error) {
	commit, err := r.r.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	f, err := tree.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, plumbing.ErrObjectNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("file %s: %w", path, err)
	}
	contents, err := f.Contents()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return []byte(contents), nil
}

// ReadTree returns the entries in the directory at path in the commit identified by hash.
// If path is empty or "/", it returns the root entries.
func (r *Repo) ReadTree(hash plumbing.Hash, path string) ([]TreeEntry, error) {
	commit, err := r.r.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}

	var target *object.Tree
	if path == "" || path == "/" {
		target = tree
	} else {
		target, err = tree.Tree(path)
		if err != nil {
			if errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, plumbing.ErrObjectNotFound) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("subtree %s: %w", path, err)
		}
	}

	entries := make([]TreeEntry, 0, len(target.Entries))
	for _, e := range target.Entries {
		te := TreeEntry{Name: e.Name, IsDir: e.Mode.IsFile() == false}
		if !te.IsDir {
			blob, err := r.r.BlobObject(e.Hash)
			if err == nil {
				te.Size = blob.Size
			}
		}
		entries = append(entries, te)
	}
	return entries, nil
}

// FetchNow performs a synchronous fetch and clears the head cache.
// Used by the webhook handler for immediate invalidation.
func (r *Repo) FetchNow(ctx context.Context) error {
	r.fetchMu.Lock()
	defer r.fetchMu.Unlock()
	if err := r.doFetch(ctx); err != nil {
		return err
	}
	r.invalidateHeadCache()
	return nil
}

// triggerBackgroundFetch fires a fetch in a goroutine if one isn't already running
// and the last fetch wasn't too recent.
func (r *Repo) triggerBackgroundFetch(ctx context.Context) {
	r.fetchMu.Lock()
	defer r.fetchMu.Unlock()
	if r.staleTTL > 0 && time.Since(r.lastFetch) < r.staleTTL/2 {
		return // another goroutine just fetched
	}
	if err := r.doFetch(ctx); err != nil {
		// Non-fatal: log only.
		fmt.Fprintf(os.Stderr, "folio: fetch %s: %v\n", r.cloneURL, err)
		return
	}
	r.invalidateHeadCache()
}

func (r *Repo) doFetch(ctx context.Context) error {
	remote, err := r.r.Remote("origin")
	if err != nil {
		return fmt.Errorf("get remote: %w", err)
	}
	// go-git bare clones with the default refspec (+refs/heads/*:refs/remotes/origin/*)
	// only update refs/remotes/origin/* on fetch, leaving refs/heads/* frozen at the
	// clone-time values. Explicit ?ref=<branch> queries resolve via refs/heads/*, so
	// they would never see new commits. Specifying both mappings keeps both in sync.
	err = remote.FetchContext(ctx, &git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
		RefSpecs: []gitconfig.RefSpec{
			"+refs/heads/*:refs/heads/*",
			"+refs/heads/*:refs/remotes/origin/*",
		},
		Force: true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch: %w", err)
	}
	r.lastFetch = time.Now()
	return nil
}

func (r *Repo) invalidateHeadCache() {
	r.headMu.Lock()
	r.headCache = make(map[string]headEntry)
	r.headMu.Unlock()
}
