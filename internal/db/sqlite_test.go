package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/pxgray/folio/internal/db"
)

func openTestDB(t *testing.T) db.Store {
	t.Helper()
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	s := openTestDB(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestUserCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// CreateUser sets ID
	u := &db.User{Email: "alice@example.com", Name: "Alice", IsAdmin: true}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected ID to be set after CreateUser")
	}

	// GetUserByID
	got, err := s.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Email != u.Email || got.Name != u.Name || !got.IsAdmin {
		t.Errorf("GetUserByID mismatch: %+v", got)
	}

	// GetUserByEmail
	got2, err := s.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got2.ID != u.ID {
		t.Errorf("GetUserByEmail ID mismatch")
	}

	// UpdateUser
	u.Name = "Alice Updated"
	if err := s.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	got3, _ := s.GetUserByID(ctx, u.ID)
	if got3.Name != "Alice Updated" {
		t.Errorf("UpdateUser did not persist: %q", got3.Name)
	}

	// ListUsers
	_ = s.CreateUser(ctx, &db.User{Email: "bob@example.com", Name: "Bob"})
	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	// DeleteUser cascades (no foreign-key violation)
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	_, err = s.GetUserByID(ctx, u.ID)
	if err == nil {
		t.Fatal("expected error after DeleteUser, got nil")
	}
}

func TestOAuthAccountCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	u := &db.User{Email: "carol@example.com", Name: "Carol"}
	_ = s.CreateUser(ctx, u)

	a := &db.OAuthAccount{UserID: u.ID, Provider: "github", ProviderID: "gh-123"}
	if err := s.CreateOAuthAccount(ctx, a); err != nil {
		t.Fatalf("CreateOAuthAccount: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected ID to be set")
	}

	// GetUserByOAuth
	got, err := s.GetUserByOAuth(ctx, "github", "gh-123")
	if err != nil {
		t.Fatalf("GetUserByOAuth: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("GetUserByOAuth returned wrong user")
	}

	// ListOAuthAccounts
	accounts, err := s.ListOAuthAccounts(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListOAuthAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Provider != "github" {
		t.Errorf("unexpected accounts: %v", accounts)
	}

	// DeleteOAuthAccount
	if err := s.DeleteOAuthAccount(ctx, u.ID, "github"); err != nil {
		t.Fatalf("DeleteOAuthAccount: %v", err)
	}
	accounts2, _ := s.ListOAuthAccounts(ctx, u.ID)
	if len(accounts2) != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", len(accounts2))
	}
}

func TestSessionCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	u := &db.User{Email: "dave@example.com", Name: "Dave"}
	_ = s.CreateUser(ctx, u)

	sess := &db.Session{
		Token:     "tok-abc123",
		UserID:    u.ID,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// GetSession
	got, err := s.GetSession(ctx, "tok-abc123")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserID != u.ID {
		t.Errorf("wrong UserID: %d", got.UserID)
	}

	// TouchSession (extend expiry)
	newExpiry := time.Now().UTC().Add(60 * 24 * time.Hour)
	if err := s.TouchSession(ctx, "tok-abc123", newExpiry); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	got2, _ := s.GetSession(ctx, "tok-abc123")
	if got2.ExpiresAt.Before(newExpiry.Add(-time.Second)) {
		t.Errorf("TouchSession did not update expiry")
	}

	// DeleteExpiredSessions — create an expired one first
	expired := &db.Session{
		Token:     "tok-expired",
		UserID:    u.ID,
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	_ = s.CreateSession(ctx, expired)
	if err := s.DeleteExpiredSessions(ctx); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	_, err = s.GetSession(ctx, "tok-expired")
	if err == nil {
		t.Fatal("expected error for expired/deleted session")
	}

	// DeleteUserSessions
	if err := s.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUserSessions: %v", err)
	}
	_, err = s.GetSession(ctx, "tok-abc123")
	if err == nil {
		t.Fatal("expected session gone after DeleteUserSessions")
	}

	// DeleteSession (individual)
	sess2 := &db.Session{Token: "tok-xyz", UserID: u.ID,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC()}
	_ = s.CreateSession(ctx, sess2)
	if err := s.DeleteSession(ctx, "tok-xyz"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = s.GetSession(ctx, "tok-xyz")
	if err == nil {
		t.Fatal("expected error for deleted session")
	}
}
