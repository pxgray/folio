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
	"github.com/pxgray/folio/internal/db"
)

func TestDashboardSettings_GET(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/dashboard/settings", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(newSessionCookie(t, store, user.ID))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/dashboard/settings: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	for _, want := range []string{`name="display_name"`, `name="current_password"`} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, bodyStr)
		}
	}
}

func TestDashboardSettings_POST_UpdateName(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)
	ctx := context.Background()

	form := url.Values{
		"display_name": {"Alice"},
	}

	req := newCSRFPost(t, ts.URL+"/-/dashboard/settings", form, store, user.ID)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/settings: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 303, got %d: %s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if loc != "/-/dashboard/settings" {
		t.Errorf("want redirect to /-/dashboard/settings, got %q", loc)
	}

	// Verify DB was updated.
	updated, err := store.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if updated.Name != "Alice" {
		t.Errorf("expected Name to be %q, got %q", "Alice", updated.Name)
	}
}

func TestDashboardSettings_POST_WrongCurrentPassword(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	form := url.Values{
		"current_password": {"wrong"},
		"new_password":     {"newpassword123"},
	}

	req := newCSRFPost(t, ts.URL+"/-/dashboard/settings", form, store, user.ID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/settings: %v", err)
	}
	defer resp.Body.Close()

	// Should re-render, not redirect.
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		t.Fatalf("want non-redirect response for wrong password, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "incorrect") && !strings.Contains(bodyStr, "password") {
		t.Errorf("expected error message about incorrect password in body, got:\n%s", bodyStr)
	}
}

func TestDashboardSettings_POST_ChangePassword_Valid(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)
	ctx := context.Background()

	form := url.Values{
		"current_password": {"testpass"},
		"new_password":     {"newpassword123"},
	}

	req := newCSRFPost(t, ts.URL+"/-/dashboard/settings", form, store, user.ID)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/settings: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 303, got %d: %s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if loc != "/-/dashboard/settings" {
		t.Errorf("want redirect to /-/dashboard/settings, got %q", loc)
	}

	// Re-fetch user and verify new password works.
	updated, err := store.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !auth.CheckPassword(updated.Password, "newpassword123") {
		t.Errorf("expected new password to be valid, but CheckPassword returned false")
	}
}

func TestDashboardSettings_UnlinkOAuth(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)
	ctx := context.Background()

	// Create an OAuth account row for the user.
	if err := store.CreateOAuthAccount(ctx, &db.OAuthAccount{
		UserID:     user.ID,
		Provider:   "github",
		ProviderID: "gh123",
	}); err != nil {
		t.Fatalf("CreateOAuthAccount: %v", err)
	}

	req := newCSRFPost(t, ts.URL+"/-/dashboard/settings/unlink/github", url.Values{}, store, user.ID)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/settings/unlink/github: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 303, got %d: %s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if loc != "/-/dashboard/settings" {
		t.Errorf("want redirect to /-/dashboard/settings, got %q", loc)
	}

	// Verify the OAuth account is gone.
	accounts, err := store.ListOAuthAccounts(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListOAuthAccounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 OAuth accounts after unlink, got %d", len(accounts))
	}
}

func TestDashboardSettings_POST_ConcurrentNameUpdates_NoRace(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)
	ctx := context.Background()

	// Create a single session shared across concurrent requests.
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
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
			form := url.Values{"display_name": {fmt.Sprintf("User%d", idx)}, "_csrf": {sess.CSRFToken}}
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/dashboard/settings",
				strings.NewReader(form.Encode()))
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			req.AddCookie(&http.Cookie{Name: "_csrf", Value: sess.CSRFToken})

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

	// Verify DB was updated at least once (at least one name change persisted).
	updated, err := store.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if updated.Name == "Test User" {
		t.Errorf("expected name to be updated by at least one goroutine, got %q", updated.Name)
	}
}
