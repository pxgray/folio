package db_test

import (
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
