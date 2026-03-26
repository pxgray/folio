package gitstore

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing"
)

// Repository is the read interface implemented by both Repo (remote bare clone)
// and LocalRepo (local working tree). The hash parameter is used by Repo to
// address git objects; LocalRepo ignores it and reads directly from disk.
type Repository interface {
	ResolveRef(ctx context.Context, ref string) (plumbing.Hash, error)
	ReadBlob(hash plumbing.Hash, path string) ([]byte, error)
	ReadTree(hash plumbing.Hash, path string) ([]TreeEntry, error)
	FetchNow(ctx context.Context) error
}
