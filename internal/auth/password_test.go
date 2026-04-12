package auth_test

import (
	"testing"

	"github.com/pxgray/folio/internal/auth"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == "hunter2" {
		t.Fatal("hash must not equal plaintext")
	}

	if !auth.CheckPassword(hash, "hunter2") {
		t.Error("CheckPassword should return true for correct password")
	}
	if auth.CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should return false for wrong password")
	}
}
