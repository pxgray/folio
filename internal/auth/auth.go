package auth

import "github.com/pxgray/folio/internal/db"

// Auth provides session management, password hashing, and OAuth helpers.
// It wraps a db.Store for persistence.
type Auth struct {
	store db.Store
}

// New creates an Auth backed by the given store.
func New(store db.Store) *Auth {
	return &Auth{store: store}
}
