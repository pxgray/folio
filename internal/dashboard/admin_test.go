package dashboard_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAdminUsersPage_AsAdmin(t *testing.T) {
	srv, adminTok, _, _ := adminTestServerWithStore(t)

	req, _ := http.NewRequest("GET", srv.URL+"/-/dashboard/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := new(strings.Builder)
	io.Copy(body, resp.Body)
	if !strings.Contains(body.String(), "admin@example.com") {
		t.Error("expected admin email in response body")
	}
	if !strings.Contains(body.String(), "user@example.com") {
		t.Error("expected regular user email in response body")
	}
}

func TestAdminUsersPage_NonAdmin(t *testing.T) {
	srv, _, regularTok, _ := adminTestServerWithStore(t)

	req, _ := http.NewRequest("GET", srv.URL+"/-/dashboard/admin/", nil)
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

func TestAdminUserEditPage_GET(t *testing.T) {
	srv, adminTok, _, store := adminTestServerWithStore(t)
	ctx := context.Background()

	users, _ := store.ListUsers(ctx)
	var regularID int64
	for _, u := range users {
		if u.Email == "user@example.com" {
			regularID = u.ID
		}
	}

	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/-/dashboard/admin/users/%d", srv.URL, regularID), nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := new(strings.Builder)
	io.Copy(body, resp.Body)
	if !strings.Contains(body.String(), "user@example.com") {
		t.Error("expected user email in edit form")
	}
}

func TestAdminUserEditPage_POST(t *testing.T) {
	srv, adminTok, _, store := adminTestServerWithStore(t)
	ctx := context.Background()

	users, _ := store.ListUsers(ctx)
	var regularID int64
	for _, u := range users {
		if u.Email == "user@example.com" {
			regularID = u.ID
		}
	}

	form := url.Values{"name": {"Updated Name"}, "email": {"user@example.com"}}
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/-/dashboard/admin/users/%d", srv.URL, regularID),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	updated, _ := store.GetUserByID(ctx, regularID)
	if updated.Name != "Updated Name" {
		t.Fatalf("expected name 'Updated Name', got %q", updated.Name)
	}
}
