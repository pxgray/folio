package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

// newRepoAPITestServer creates a dashboard test server with two users.
// Returns the server, the store, userA, and userB.
func newRepoAPITestServer(t *testing.T) (*httptest.Server, db.Store, *db.User, *db.User) {
	t.Helper()

	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	hashA, err := auth.HashPassword("secretA")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userA := &db.User{Email: "userA@example.com", Password: hashA, Name: "User A"}
	if err := store.CreateUser(ctx, userA); err != nil {
		t.Fatalf("CreateUser A: %v", err)
	}

	hashB, err := auth.HashPassword("secretB")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userB := &db.User{Email: "userB@example.com", Password: hashB, Name: "User B"}
	if err := store.CreateUser(ctx, userB); err != nil {
		t.Fatalf("CreateUser B: %v", err)
	}

	authn := auth.New(store)
	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, store, userA, userB
}

func TestAPICreateRepo(t *testing.T) {
	ts, store, userA, _ := newRepoAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"host":  "github.com",
		"owner": "acme",
		"repo":  "docs",
	})

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/api/v1/repos", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /-/api/v1/repos: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 202, got %d: %s", resp.StatusCode, bodyBytes)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "pending_clone") {
		t.Errorf("expected body to contain 'pending_clone', got: %s", bodyStr)
	}

	// Verify DB row exists.
	row, err := store.GetRepoByKey(ctx, "github.com", "acme", "docs")
	if err != nil {
		t.Fatalf("GetRepoByKey: %v", err)
	}
	if row.Status != db.RepoStatusPending {
		t.Errorf("want status %q, got %q", db.RepoStatusPending, row.Status)
	}
	if row.OwnerID != userA.ID {
		t.Errorf("want ownerID %d, got %d", userA.ID, row.OwnerID)
	}
}

func TestAPIListRepos(t *testing.T) {
	ts, store, userA, userB := newRepoAPITestServer(t)

	ctx := context.Background()

	// Seed two repos for userA.
	repoA1 := &db.Repo{OwnerID: userA.ID, Host: "github.com", RepoOwner: "acme", RepoName: "docs", Status: db.RepoStatusReady}
	if err := store.CreateRepo(ctx, repoA1); err != nil {
		t.Fatalf("CreateRepo A1: %v", err)
	}
	repoA2 := &db.Repo{OwnerID: userA.ID, Host: "github.com", RepoOwner: "acme", RepoName: "wiki", Status: db.RepoStatusReady}
	if err := store.CreateRepo(ctx, repoA2); err != nil {
		t.Fatalf("CreateRepo A2: %v", err)
	}

	// Seed one repo for userB.
	repoB1 := &db.Repo{OwnerID: userB.ID, Host: "github.com", RepoOwner: "beta", RepoName: "stuff", Status: db.RepoStatusReady}
	if err := store.CreateRepo(ctx, repoB1); err != nil {
		t.Fatalf("CreateRepo B1: %v", err)
	}

	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/api/v1/repos", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/api/v1/repos: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var repos []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	if len(repos) != 2 {
		t.Errorf("want 2 repos for userA, got %d", len(repos))
	}

	// Verify none of the returned repos belong to userB.
	for _, r := range repos {
		ownerID, ok := r["owner_id"].(float64)
		if !ok {
			t.Errorf("owner_id missing or wrong type in %v", r)
			continue
		}
		if int64(ownerID) != userA.ID {
			t.Errorf("expected owner_id %d, got %d", userA.ID, int64(ownerID))
		}
	}
}

func TestAPICreateRepoMissingFields(t *testing.T) {
	ts, store, userA, _ := newRepoAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	body, _ := json.Marshal(map[string]string{})

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/api/v1/repos", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /-/api/v1/repos: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 422, got %d: %s", resp.StatusCode, bodyBytes)
	}
}

func TestAPIDeleteRepoOwnership(t *testing.T) {
	ts, store, userA, userB := newRepoAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)

	// Seed a repo owned by userA.
	repo := &db.Repo{
		OwnerID:   userA.ID,
		Host:      "github.com",
		RepoOwner: "acme",
		RepoName:  "private",
		Status:    db.RepoStatusReady,
	}
	if err := store.CreateRepo(ctx, repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	url := ts.URL + fmt.Sprintf("/-/api/v1/repos/%d", repo.ID)

	// UserB tries to delete — expect 403.
	sessB, err := authn.NewSession(ctx, userB.ID)
	if err != nil {
		t.Fatalf("NewSession B: %v", err)
	}

	reqB, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("NewRequest B: %v", err)
	}
	reqB.AddCookie(&http.Cookie{Name: "session", Value: sessB.Token})

	respB, err := http.DefaultClient.Do(reqB)
	if err != nil {
		t.Fatalf("DELETE as userB: %v", err)
	}
	respB.Body.Close()

	if respB.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 from userB, got %d", respB.StatusCode)
	}

	// Verify repo still exists after failed delete.
	if _, err := store.GetRepo(ctx, repo.ID); err != nil {
		t.Fatalf("GetRepo after 403: %v", err)
	}

	// UserA deletes — expect 204.
	sessA, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession A: %v", err)
	}

	reqA, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("NewRequest A: %v", err)
	}
	reqA.AddCookie(&http.Cookie{Name: "session", Value: sessA.Token})

	respA, err := http.DefaultClient.Do(reqA)
	if err != nil {
		t.Fatalf("DELETE as userA: %v", err)
	}
	respA.Body.Close()

	if respA.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204 from userA, got %d", respA.StatusCode)
	}

	// Verify row is gone from DB.
	if _, err := store.GetRepo(ctx, repo.ID); err == nil {
		t.Fatal("expected error from GetRepo after delete, got nil")
	}
}

func TestAPIPatchRepo(t *testing.T) {
	ts, store, userA, _ := newRepoAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)

	// Seed a repo for userA.
	repo := &db.Repo{
		OwnerID:       userA.ID,
		Host:          "github.com",
		RepoOwner:     "acme",
		RepoName:      "patchable",
		WebhookSecret: "old-secret",
		Status:        db.RepoStatusReady,
	}
	if err := store.CreateRepo(ctx, repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	sess, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	newSecret := "new-secret"
	body, _ := json.Marshal(map[string]string{"webhook_secret": newSecret})

	url := ts.URL + fmt.Sprintf("/-/api/v1/repos/%d", repo.ID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /-/api/v1/repos/{id}: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	if got["webhook_secret"] != newSecret {
		t.Errorf("want webhook_secret %q, got %v", newSecret, got["webhook_secret"])
	}
	if got["repo_name"] != "patchable" {
		t.Errorf("want repo_name %q, got %v", "patchable", got["repo_name"])
	}
}

func TestAPIGetRepoNotFound(t *testing.T) {
	ts, store, userA, _ := newRepoAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)

	sess, err := authn.NewSession(ctx, userA.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/api/v1/repos/99999", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /-/api/v1/repos/99999: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 404, got %d: %s", resp.StatusCode, bodyBytes)
	}
}
