package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestGitHubOAuthRedirect verifies that GET /-/auth/github with credentials
// configured redirects to the GitHub OAuth authorization URL and sets the
// oauth_state cookie.
func TestGitHubOAuthRedirect(t *testing.T) {
	ts, store := newTestDashboard(t)
	ctx := context.Background()

	if err := store.UpsertSetting(ctx, "oauth_github_client_id", "test-client-id"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSetting(ctx, "oauth_github_client_secret", "test-client-secret"); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(ts.URL + "/-/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize") {
		t.Errorf("want Location to start with https://github.com/login/oauth/authorize, got %q", loc)
	}

	// Check that the oauth_state cookie was set.
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "oauth_state" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Set-Cookie with oauth_state, not found in response")
	}
}

// TestGitHubOAuthNotConfigured verifies that GET /-/auth/github with no
// credentials configured redirects to the login page with an error parameter.
func TestGitHubOAuthNotConfigured(t *testing.T) {
	ts, _ := newTestDashboard(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(ts.URL + "/-/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/-/auth/login?error=github_not_configured" {
		t.Errorf("want Location /-/auth/login?error=github_not_configured, got %q", loc)
	}
}

func TestLoginPageGet(t *testing.T) {
	ts, _ := newTestDashboard(t)

	resp, err := http.Get(ts.URL + "/-/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "<form") {
		t.Error("expected response body to contain <form")
	}
	if !strings.Contains(bodyStr, "/-/api/v1/auth/login") {
		t.Error("expected response body to contain /-/api/v1/auth/login")
	}
}
