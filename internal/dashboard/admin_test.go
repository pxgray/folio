package dashboard_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/pxgray/folio/internal/auth"
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

func TestAdminSettingsPage_GET(t *testing.T) {
	srv, adminTok, _, store := adminTestServerWithStore(t)
	ctx := context.Background()
	store.UpsertSetting(ctx, "addr", ":8080")

	req, _ := http.NewRequest("GET", srv.URL+"/-/dashboard/admin/settings", nil)
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
	if !strings.Contains(body.String(), ":8080") {
		t.Error("expected addr value in settings form")
	}
	if !strings.Contains(body.String(), "requires a server restart") {
		t.Error("expected restart warning banner in settings page")
	}
}

func TestAdminSettingsPage_POST(t *testing.T) {
	srv, _, _, store := adminTestServerWithStore(t)
	ctx := context.Background()

	// Create a fresh session with CSRF token for the POST request.
	authn := auth.New(store)
	adminUser, _ := store.GetUserByEmail(ctx, "admin@example.com")
	adminSess, err := authn.NewSession(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	form := url.Values{
		"addr":                       {":9090"},
		"cache_dir":                  {"~/.cache/folio"},
		"stale_ttl":                  {"10m"},
		"base_url":                   {""},
		"oauth_github_client_id":     {""},
		"oauth_github_client_secret": {""},
		"oauth_google_client_id":     {""},
		"oauth_google_client_secret": {""},
		"_csrf":                      {adminSess.CSRFToken},
	}
	req, _ := http.NewRequest("POST", srv.URL+"/-/dashboard/admin/settings",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminSess.Token})
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: adminSess.CSRFToken})

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

	val, _ := store.GetSetting(ctx, "addr")
	if val != ":9090" {
		t.Fatalf("expected addr ':9090', got %q", val)
	}
}

func TestAdminUserEditPage_POST(t *testing.T) {
	srv, _, _, store := adminTestServerWithStore(t)
	ctx := context.Background()

	users, _ := store.ListUsers(ctx)
	var regularID int64
	for _, u := range users {
		if u.Email == "user@example.com" {
			regularID = u.ID
		}
	}

	// Create a fresh session with CSRF token for the POST request.
	authn := auth.New(store)
	adminUser, _ := store.GetUserByEmail(ctx, "admin@example.com")
	adminSess, err := authn.NewSession(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	form := url.Values{"name": {"Updated Name"}, "email": {"user@example.com"}, "_csrf": {adminSess.CSRFToken}}
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/-/dashboard/admin/users/%d", srv.URL, regularID),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminSess.Token})
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: adminSess.CSRFToken})

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

func TestAdminUserEditPost_ConcurrentUpdates_NoRace(t *testing.T) {
	srv, _, _, store := adminTestServerWithStore(t)
	ctx := context.Background()

	users, _ := store.ListUsers(ctx)
	var regularID int64
	for _, u := range users {
		if u.Email == "user@example.com" {
			regularID = u.ID
		}
	}

	authn := auth.New(store)
	adminUser, _ := store.GetUserByEmail(ctx, "admin@example.com")
	adminSess, err := authn.NewSession(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			form := url.Values{
				"name":  {fmt.Sprintf("ConcurrentUser%d", idx)},
				"email": {"user@example.com"},
				"_csrf": {adminSess.CSRFToken},
			}
			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/-/dashboard/admin/users/%d", srv.URL, regularID),
				strings.NewReader(form.Encode()))
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "session", Value: adminSess.Token})
			req.AddCookie(&http.Cookie{Name: "_csrf", Value: adminSess.CSRFToken})

			resp, err := client.Do(req)
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusSeeOther {
				mu.Lock()
				errors = append(errors,
					fmt.Errorf("goroutine %d: want 303, got %d", idx, resp.StatusCode))
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(errors) > 0 {
		for _, e := range errors {
			t.Error(e)
		}
	}

	// Verify DB was updated (at least one name change persisted).
	updated, err := store.GetUserByID(ctx, regularID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if updated.Name == "Regular" {
		t.Errorf("expected name to be updated by at least one goroutine, got %q", updated.Name)
	}
	// Verify email was not corrupted by concurrent writes.
	if updated.Email != "user@example.com" {
		t.Errorf("expected email to remain 'user@example.com', got %q", updated.Email)
	}
}
