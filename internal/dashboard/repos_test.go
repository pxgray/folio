package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

func newDashboardTestServer(t *testing.T) (*httptest.Server, db.Store, *db.User) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	hash, err := auth.HashPassword("testpass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := &db.User{Email: "test@example.com", Password: hash, Name: "Test User"}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authn := auth.New(store)
	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store, user
}

func TestDashboardRepoList_Unauthenticated(t *testing.T) {
	ts, _, _ := newDashboardTestServer(t)

	// Use a client that does NOT follow redirects so we can inspect the 302.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(ts.URL + "/-/dashboard/")
	if err != nil {
		t.Fatalf("GET /-/dashboard/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/-/auth/login" {
		t.Errorf("want redirect to /-/auth/login, got %q", loc)
	}
}

func TestDashboardRepoList_Empty(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	ctx := context.Background()
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/dashboard/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/dashboard/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "No repos yet") {
		t.Errorf("expected body to contain 'No repos yet', got:\n%s", body)
	}
}

func TestDashboardRepoList_WithRepos(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	ctx := context.Background()

	repo1 := &db.Repo{
		OwnerID:   user.ID,
		Host:      "github.com",
		RepoOwner: "acme",
		RepoName:  "docs",
		Status:    db.RepoStatusReady,
	}
	if err := store.CreateRepo(ctx, repo1); err != nil {
		t.Fatalf("CreateRepo 1: %v", err)
	}

	repo2 := &db.Repo{
		OwnerID:   user.ID,
		Host:      "github.com",
		RepoOwner: "acme",
		RepoName:  "wiki",
		Status:    db.RepoStatusPending,
	}
	if err := store.CreateRepo(ctx, repo2); err != nil {
		t.Fatalf("CreateRepo 2: %v", err)
	}

	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/dashboard/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/dashboard/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "docs") {
		t.Errorf("expected body to contain repo name 'docs', got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "wiki") {
		t.Errorf("expected body to contain repo name 'wiki', got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, db.RepoStatusReady) {
		t.Errorf("expected body to contain status badge %q, got:\n%s", db.RepoStatusReady, bodyStr)
	}
	if !strings.Contains(bodyStr, db.RepoStatusPending) {
		t.Errorf("expected body to contain status badge %q, got:\n%s", db.RepoStatusPending, bodyStr)
	}
}

// helper: create an authenticated session cookie for a user.
func newSessionCookie(t *testing.T, store db.Store, userID int64) *http.Cookie {
	t.Helper()
	authn := auth.New(store)
	sess, err := authn.NewSession(context.Background(), userID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func TestDashboardRepoNew_GET(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/dashboard/repos/new", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(newSessionCookie(t, store, user.ID))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/dashboard/repos/new: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	for _, field := range []string{`name="host"`, `name="owner"`, `name="repo_name"`} {
		if !strings.Contains(bodyStr, field) {
			t.Errorf("expected form field %q in body, got:\n%s", field, bodyStr)
		}
	}
}

func TestDashboardRepoNew_POST_Valid(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	form := url.Values{
		"host":      {"github.com"},
		"owner":     {"acme"},
		"repo_name": {"docs"},
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/dashboard/repos/new",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(newSessionCookie(t, store, user.ID))

	// Do not follow redirect.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/repos/new: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 303, got %d: %s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if loc != "/-/dashboard/" {
		t.Errorf("want redirect to /-/dashboard/, got %q", loc)
	}

	// Verify repo row was created in DB.
	repos, err := store.ListReposByOwner(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListReposByOwner: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo in DB, got %d", len(repos))
	}
	r := repos[0]
	if r.Host != "github.com" || r.RepoOwner != "acme" || r.RepoName != "docs" {
		t.Errorf("unexpected repo fields: %+v", r)
	}
}

func TestDashboardRepoNew_POST_MissingField(t *testing.T) {
	ts, store, user := newDashboardTestServer(t)

	// host is empty — should fail validation.
	form := url.Values{
		"host":      {""},
		"owner":     {"acme"},
		"repo_name": {"docs"},
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/dashboard/repos/new",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(newSessionCookie(t, store, user.ID))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /-/dashboard/repos/new: %v", err)
	}
	defer resp.Body.Close()

	// Should re-render the form, not redirect.
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		t.Fatalf("want non-redirect response, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "required") && !strings.Contains(bodyStr, "required") {
		// Accept any error message mentioning the validation failure.
		if !strings.Contains(bodyStr, "Host") && !strings.Contains(bodyStr, "error") {
			t.Errorf("expected error message in body, got:\n%s", bodyStr)
		}
	}
}
