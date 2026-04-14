package dashboard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAdminUpdateUser_Promote(t *testing.T) {
	srv, adminTok, _ := adminTestServer(t)
	// get user list to find regular user id
	req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, _ := http.DefaultClient.Do(req)
	var users []map[string]any
	json.NewDecoder(resp.Body).Decode(&users)
	resp.Body.Close()

	var regularID float64
	for _, u := range users {
		if u["email"] == "user@example.com" {
			regularID = u["id"].(float64)
		}
	}

	body := strings.NewReader(`{"is_admin":true}`)
	req2, _ := http.NewRequest("PATCH",
		fmt.Sprintf("%s/-/api/v1/admin/users/%.0f", srv.URL, regularID), body)
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestAdminUpdateUser_DemoteLastAdmin(t *testing.T) {
	srv, adminTok, _ := adminTestServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, _ := http.DefaultClient.Do(req)
	var users []map[string]any
	json.NewDecoder(resp.Body).Decode(&users)
	resp.Body.Close()

	var adminID float64
	for _, u := range users {
		if u["is_admin"].(bool) {
			adminID = u["id"].(float64)
		}
	}

	body := strings.NewReader(`{"is_admin":false}`)
	req2, _ := http.NewRequest("PATCH",
		fmt.Sprintf("%s/-/api/v1/admin/users/%.0f", srv.URL, adminID), body)
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp2.StatusCode)
	}
}

func TestAdminUpdateUser_SelfDemote(t *testing.T) {
	srv, adminTok, _ := adminTestServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, _ := http.DefaultClient.Do(req)
	var users []map[string]any
	json.NewDecoder(resp.Body).Decode(&users)
	resp.Body.Close()

	var adminID float64
	var regularID float64
	for _, u := range users {
		if u["email"] == "admin@example.com" {
			adminID = u["id"].(float64)
		}
		if u["email"] == "user@example.com" {
			regularID = u["id"].(float64)
		}
	}

	// Promote the regular user first so we're not demoting the last admin
	promoteBody := strings.NewReader(`{"is_admin":true}`)
	req3, _ := http.NewRequest("PATCH",
		fmt.Sprintf("%s/-/api/v1/admin/users/%.0f", srv.URL, regularID), promoteBody)
	req3.Header.Set("Content-Type", "application/json")
	req3.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	http.DefaultClient.Do(req3)

	body := strings.NewReader(`{"is_admin":false}`)
	req2, _ := http.NewRequest("PATCH",
		fmt.Sprintf("%s/-/api/v1/admin/users/%.0f", srv.URL, adminID), body)
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp2.StatusCode)
	}
}
