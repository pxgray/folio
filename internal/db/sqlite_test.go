package db_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	u := &db.User{Email: "alice@example.com", Name: "Alice", Password: "existing-hashed-password", IsAdmin: true}
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

	// UpdateUser without password preserves existing password
	u.Password = "existing-hashed-password"
	u.Name = "Alice Updated"
	if err := s.UpdateUser(ctx, u, nil); err != nil {
		t.Fatalf("UpdateUser nil password: %v", err)
	}
	got3, _ := s.GetUserByID(ctx, u.ID)
	if got3.Name != "Alice Updated" {
		t.Errorf("UpdateUser did not persist name: %q", got3.Name)
	}
	if got3.Password != "existing-hashed-password" {
		t.Errorf("UpdateUser(nil) changed password: expected %q, got %q", "existing-hashed-password", got3.Password)
	}

	// UpdateUser with password updates it
	pw := "newhashedpassword"
	if err := s.UpdateUser(ctx, u, &pw); err != nil {
		t.Fatalf("UpdateUser with password: %v", err)
	}
	got4, _ := s.GetUserByID(ctx, u.ID)
	if got4.Password != pw {
		t.Errorf("UpdateUser with password: expected %q, got %q", pw, got4.Password)
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

func TestRepoCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	owner := &db.User{Email: "erin@example.com", Name: "Erin"}
	_ = s.CreateUser(ctx, owner)

	r := &db.Repo{
		OwnerID:   owner.ID,
		Host:      "github.com",
		RepoOwner: "acme",
		RepoName:  "docs",
		Status:    db.RepoStatusPending,
	}
	if err := s.CreateRepo(ctx, r); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if r.ID == 0 {
		t.Fatal("expected ID to be set")
	}

	// GetRepo
	got, err := s.GetRepo(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if got.RepoName != "docs" || got.Status != db.RepoStatusPending {
		t.Errorf("GetRepo mismatch: %+v", got)
	}

	// GetRepoByKey
	got2, err := s.GetRepoByKey(ctx, "github.com", "acme", "docs")
	if err != nil {
		t.Fatalf("GetRepoByKey: %v", err)
	}
	if got2.ID != r.ID {
		t.Errorf("GetRepoByKey returned wrong ID")
	}

	// Key() helper
	if r.Key() != "github.com/acme/docs" {
		t.Errorf("Key() = %q", r.Key())
	}

	// ListReposByOwner
	repos, err := s.ListReposByOwner(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListReposByOwner: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}

	// ListAllRepos
	all, err := s.ListAllRepos(ctx)
	if err != nil {
		t.Fatalf("ListAllRepos: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 repo in ListAllRepos, got %d", len(all))
	}

	// UpdateRepoStatus
	if err := s.UpdateRepoStatus(ctx, r.ID, db.RepoStatusReady, ""); err != nil {
		t.Fatalf("UpdateRepoStatus: %v", err)
	}
	got3, _ := s.GetRepo(ctx, r.ID)
	if got3.Status != db.RepoStatusReady {
		t.Errorf("UpdateRepoStatus did not persist")
	}

	// UpdateRepo
	r.TrustedHTML = true
	r.StaleTTLSecs = 300
	if err := s.UpdateRepo(ctx, r); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}
	got4, _ := s.GetRepo(ctx, r.ID)
	if !got4.TrustedHTML || got4.StaleTTLSecs != 300 {
		t.Errorf("UpdateRepo did not persist: %+v", got4)
	}

	// DeleteRepo
	if err := s.DeleteRepo(ctx, r.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	_, err = s.GetRepo(ctx, r.ID)
	if err == nil {
		t.Fatal("expected error after DeleteRepo")
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// GetSetting on missing key returns ErrSettingNotFound
	_, err := s.GetSetting(ctx, "setup_complete")
	if !errors.Is(err, db.ErrSettingNotFound) {
		t.Fatalf("expected ErrSettingNotFound, got %v", err)
	}

	// UpsertSetting inserts
	if err := s.UpsertSetting(ctx, "setup_complete", "false"); err != nil {
		t.Fatalf("UpsertSetting insert: %v", err)
	}
	val, err := s.GetSetting(ctx, "setup_complete")
	if err != nil || val != "false" {
		t.Fatalf("GetSetting: err=%v val=%q", err, val)
	}

	// IsSetupComplete — false
	done, err := s.IsSetupComplete(ctx)
	if err != nil || done {
		t.Fatalf("IsSetupComplete expected false, got %v / %v", done, err)
	}

	// UpsertSetting updates
	if err := s.UpsertSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatalf("UpsertSetting update: %v", err)
	}
	done2, _ := s.IsSetupComplete(ctx)
	if !done2 {
		t.Fatal("IsSetupComplete expected true after upsert")
	}
}

func TestArtifacts(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	owner := &db.User{Email: "frank@example.com", Name: "Frank"}
	_ = s.CreateUser(ctx, owner)
	repo := &db.Repo{OwnerID: owner.ID, Host: "github.com", RepoOwner: "x", RepoName: "y"}
	_ = s.CreateRepo(ctx, repo)

	arts := map[string]string{"app": "/app", "docs": "/docs"}
	if err := s.SetRepoArtifacts(ctx, repo.ID, arts); err != nil {
		t.Fatalf("SetRepoArtifacts: %v", err)
	}

	got, err := s.GetRepoArtifacts(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepoArtifacts: %v", err)
	}
	if got["app"] != "/app" || got["docs"] != "/docs" {
		t.Errorf("unexpected artifacts: %v", got)
	}

	// Replace artifacts (SetRepoArtifacts is replace-all)
	arts2 := map[string]string{"new": "/new"}
	_ = s.SetRepoArtifacts(ctx, repo.ID, arts2)
	got2, _ := s.GetRepoArtifacts(ctx, repo.ID)
	if len(got2) != 1 || got2["new"] != "/new" {
		t.Errorf("expected replace-all, got: %v", got2)
	}
}

func TestDemoteAdmin(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// Create 3 admins
	admin1 := &db.User{Email: "a1@example.com", Name: "Admin1", IsAdmin: true}
	_ = s.CreateUser(ctx, admin1)
	admin2 := &db.User{Email: "a2@example.com", Name: "Admin2", IsAdmin: true}
	_ = s.CreateUser(ctx, admin2)
	admin3 := &db.User{Email: "a3@example.com", Name: "Admin3", IsAdmin: true}
	_ = s.CreateUser(ctx, admin3)

	// Demoting one admin should succeed (3 admins → 2)
	affected, err := s.DemoteAdmin(ctx, admin1.ID)
	if err != nil {
		t.Fatalf("DemoteAdmin: %v", err)
	}
	if affected != 1 {
		t.Errorf("expected affected=1, got %d", affected)
	}
	got1, _ := s.GetUserByID(ctx, admin1.ID)
	if got1.IsAdmin {
		t.Error("expected admin1 to be non-admin after demote")
	}

	// Demoting another should succeed (2 admins → 1)
	affected, err = s.DemoteAdmin(ctx, admin2.ID)
	if err != nil {
		t.Fatalf("DemoteAdmin: %v", err)
	}
	if affected != 1 {
		t.Errorf("expected affected=1, got %d", affected)
	}
	got2, _ := s.GetUserByID(ctx, admin2.ID)
	if got2.IsAdmin {
		t.Error("expected admin2 to be non-admin after demote")
	}

	// Demoting the last admin should fail with ErrLastAdmin
	affected, err = s.DemoteAdmin(ctx, admin3.ID)
	if !errors.Is(err, db.ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin, got %v (affected=%d)", err, affected)
	}
	got3, _ := s.GetUserByID(ctx, admin3.ID)
	if !got3.IsAdmin {
		t.Error("expected admin3 to still be admin (last admin protection)")
	}
}

func TestDemoteAdmin_NonAdmin(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// Create one admin and one regular user
	admin := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true}
	_ = s.CreateUser(ctx, admin)
	regular := &db.User{Email: "user@example.com", Name: "User"}
	_ = s.CreateUser(ctx, regular)

	// Demoting a non-admin should do nothing (no rows affected)
	affected, err := s.DemoteAdmin(ctx, regular.ID)
	if err != nil {
		t.Fatalf("DemoteAdmin: %v", err)
	}
	if affected != 0 {
		t.Errorf("expected affected=0 for non-admin, got %d", affected)
	}
}

func TestDemoteAdmin_LastAdmin(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// Create exactly one admin
	admin := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true}
	_ = s.CreateUser(ctx, admin)

	affected, err := s.DemoteAdmin(ctx, admin.ID)
	if !errors.Is(err, db.ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin, got %v (affected=%d)", err, affected)
	}
}

func TestDemoteAdmin_Concurrent(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)

	// Create 3 admins
	for i := 0; i < 3; i++ {
		u := &db.User{
			Email:   fmt.Sprintf("admin%d@example.com", i),
			Name:    fmt.Sprintf("Admin%d", i),
			IsAdmin: true,
		}
		_ = s.CreateUser(ctx, u)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			affected, err := s.DemoteAdmin(ctx, int64(idx%3+1))
			if err != nil {
				if errors.Is(err, db.ErrLastAdmin) {
					return // expected
				}
				mu.Lock()
				errs = append(errs, fmt.Errorf("goroutine %d: %w", idx, err))
				mu.Unlock()
				return
			}
			// affected=1 means we demoted this user; affected=0 means another
			// goroutine already demoted them — both are valid outcomes.
			if affected != 1 && affected != 0 {
				mu.Lock()
				errs = append(errs, fmt.Errorf("goroutine %d: expected affected=0 or 1, got %d", idx, affected))
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		for _, e := range errs {
			t.Error(e)
		}
	}

	// Exactly 1 admin should remain
	users, _ := s.ListUsers(ctx)
	adminCount := 0
	for _, u := range users {
		if u.IsAdmin {
			adminCount++
		}
	}
	if adminCount != 1 {
		t.Errorf("expected exactly 1 admin remaining, got %d", adminCount)
	}
}
