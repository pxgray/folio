package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

func newTestAuth(t *testing.T) (*auth.Auth, db.Store) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return auth.New(store), store
}

func TestNewAndValidateSession(t *testing.T) {
	ctx := context.Background()
	a, store := newTestAuth(t)

	u := &db.User{Email: "grace@example.com", Name: "Grace"}
	if err := store.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sess, err := a.NewSession(ctx, u.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if len(sess.Token) == 0 {
		t.Fatal("expected non-empty token")
	}
	if sess.ExpiresAt.Before(time.Now().Add(29 * 24 * time.Hour)) {
		t.Fatal("expected 30-day expiry")
	}

	// ValidateSession returns the user
	got, err := a.ValidateSession(ctx, sess.Token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ValidateSession returned wrong user ID: %d", got.ID)
	}

	// Unknown token returns error
	_, err = a.ValidateSession(ctx, "bad-token")
	if err == nil {
		t.Fatal("expected error for unknown token")
	}

	// Expired session returns error
	expSess := &db.Session{
		Token:     "expired-tok",
		UserID:    u.ID,
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
		CreatedAt: time.Now().UTC(),
	}
	_ = store.CreateSession(ctx, expSess)
	_, err = a.ValidateSession(ctx, "expired-tok")
	if err == nil {
		t.Fatal("expected error for expired session")
	}
}
