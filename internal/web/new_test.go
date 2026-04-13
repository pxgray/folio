package web_test

import (
	"io/fs"
	"testing"
	"time"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/web"
)

func TestNew_WithEmptyDB(t *testing.T) {
	dbStore, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	gitStore := gitstore.New(t.TempDir(), 5*time.Minute)
	staticFS, _ := fs.Sub(assets.StaticFS, "static")

	srv, err := web.New(dbStore, gitStore, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestReload_UpdatesMaps(t *testing.T) {
	dbStore, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	gitStore := gitstore.New(t.TempDir(), 5*time.Minute)
	staticFS, _ := fs.Sub(assets.StaticFS, "static")

	srv, err := web.New(dbStore, gitStore, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	// Insert a user and repo into the DB after construction.
	user := &db.User{Email: "u@example.com", Name: "u", IsAdmin: true}
	if err := dbStore.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	repo := &db.Repo{
		OwnerID:       user.ID,
		Host:          "github.com",
		RepoOwner:     "acme",
		RepoName:      "docs",
		TrustedHTML:   true,
		WebhookSecret: "s3cr3t",
		Status:        db.RepoStatusPending,
	}
	if err := dbStore.CreateRepo(t.Context(), repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Reload should pick up the new repo without error.
	if err := srv.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
}
