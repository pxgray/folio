package db_test

import (
	"context"
	"testing"

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
