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

	adminPw, _ := auth.HashPassword("adminpass")
	adminUser := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true, Password: adminPw}
	store.CreateUser(ctx, adminUser)

	regularPw, _ := auth.HashPassword("userpass")
	regularUser := &db.User{Email: "user@example.com", Name: "Regular", IsAdmin: false, Password: regularPw}
	store.CreateUser(ctx, regularUser)

	authn := auth.New(store)
	adminSess, _ := authn.NewSession(ctx, adminUser.ID)
	regularSess, _ := authn.NewSession(ctx, regularUser.ID)

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
	var users []map[string]interface{}
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
