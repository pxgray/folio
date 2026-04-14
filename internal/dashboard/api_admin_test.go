package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

// adminTestServer returns a test server pre-seeded with one admin user and one
// regular user. Returns server, admin session token, regular user session token.
func adminTestServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	adminPw, err := auth.HashPassword("adminpass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	adminUser := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true, Password: adminPw}
	if err := store.CreateUser(ctx, adminUser); err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}

	regularPw, err := auth.HashPassword("userpass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	regularUser := &db.User{Email: "user@example.com", Name: "Regular", IsAdmin: false, Password: regularPw}
	if err := store.CreateUser(ctx, regularUser); err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}

	authn := auth.New(store)
	adminSess, err := authn.NewSession(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("NewSession admin: %v", err)
	}
	regularSess, err := authn.NewSession(ctx, regularUser.ID)
	if err != nil {
		t.Fatalf("NewSession regular: %v", err)
	}

	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, adminSess.Token, regularSess.Token
}

func TestAdminListUsers_AsAdmin(t *testing.T) {
	ts, adminTok, _ := adminTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/-/api/v1/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var users []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestAdminListUsers_NonAdmin(t *testing.T) {
	ts, _, regularTok := adminTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/-/api/v1/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: regularTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
