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
