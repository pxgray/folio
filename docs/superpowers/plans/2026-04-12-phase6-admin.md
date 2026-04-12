# Phase 6: Admin Panel

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the admin panel: JSON API and SSR pages for managing all users (promote/demote, delete with repo cascade) and server settings. After this phase the dashboard feature is complete.

**Architecture:** All admin routes are protected by RequireAdmin middleware. User delete cascades via DB ON DELETE CASCADE + explicit gitStore.RemoveRepo calls for each owned repo. Server settings that require restart are flagged in the API response but still persisted immediately.

**Tech Stack:** No new dependencies. Uses all packages from Phases 1-5.

---

## Task 1: Admin user list API + test

**Files touched:** `internal/dashboard/api_admin.go`, `internal/dashboard/api_admin_test.go`

**Goal:** `GET /-/api/v1/admin/users` returns a JSON array of all users. Non-admins get 403.

### Steps

- [ ] Create `internal/dashboard/api_admin.go` with package declaration and the `HandleAdminListUsers` handler:

```go
package dashboard

import (
    "encoding/json"
    "net/http"

    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/db"
)

// HandleAdminListUsers handles GET /-/api/v1/admin/users.
// Returns a JSON array of all users. Requires admin role.
func (h *Handler) HandleAdminListUsers(w http.ResponseWriter, r *http.Request) {
    users, err := h.store.ListUsers(r.Context())
    if err != nil {
        jsonError(w, "failed to list users", http.StatusInternalServerError)
        return
    }
    type userRow struct {
        ID        int64  `json:"id"`
        Email     string `json:"email"`
        Name      string `json:"name"`
        IsAdmin   bool   `json:"is_admin"`
        CreatedAt string `json:"created_at"`
    }
    rows := make([]userRow, len(users))
    for i, u := range users {
        rows[i] = userRow{
            ID:        u.ID,
            Email:     u.Email,
            Name:      u.Name,
            IsAdmin:   u.IsAdmin,
            CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
        }
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(rows)
}
```

- [ ] Also add the `jsonError` helper at the bottom of `api_admin.go` if not already defined elsewhere in the package:

```go
func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

  (If `jsonError` already exists in another file in the `dashboard` package from a prior phase, skip this step to avoid a duplicate-declaration compile error.)

- [ ] Create `internal/dashboard/api_admin_test.go`. Use the same in-process test server pattern established in earlier phases (construct a `Handler` with a `db.NewMemStore()` and stub `gitStore` / `docSrv`):

```go
package dashboard_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/dashboard"
    "github.com/pxgray/folio/internal/db"
)

// adminTestServer returns a test server pre-seeded with one admin user and one
// regular user. It returns the server, the admin session token, and the regular
// user session token.
func adminTestServer(t *testing.T) (*httptest.Server, string, string) {
    t.Helper()
    store := db.NewMemStore()
    ctx := context.Background()

    adminUser := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true}
    // hash a known password
    adminUser.Password, _ = auth.HashPassword("adminpass")
    store.CreateUser(ctx, adminUser)

    regularUser := &db.User{Email: "user@example.com", Name: "Regular", IsAdmin: false}
    regularUser.Password, _ = auth.HashPassword("userpass")
    store.CreateUser(ctx, regularUser)

    authn := auth.New(store)
    adminSess, _ := authn.CreateSession(ctx, adminUser.ID)
    regularSess, _ := authn.CreateSession(ctx, regularUser.ID)

    h := dashboard.New(store, nil, nil, authn)
    srv := httptest.NewServer(h.Handler())
    t.Cleanup(srv.Close)
    return srv, adminSess.Token, regularSess.Token
}

func TestAdminListUsers_AsAdmin(t *testing.T) {
    srv, adminTok, _ := adminTestServer(t)

    req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
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
    srv, _, regularTok := adminTestServer(t)

    req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
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
```

- [ ] Wire the route temporarily so tests can compile — add to `Handler()` in `internal/dashboard/handler.go` (or wherever routes are defined):
  ```go
  r.With(authn.RequireAdmin).Get("/-/api/v1/admin/users", h.HandleAdminListUsers)
  ```
  (This will be consolidated in Task 8; the temporary wiring is needed for the test to run.)

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminListUsers -timeout 60s
  ```
  Expected: both subtests pass.

---

## Task 2: Admin user update API + tests

**Files touched:** `internal/dashboard/api_admin.go`, `internal/dashboard/api_admin_test.go`

**Goal:** `PATCH /-/api/v1/admin/users/{id}` allows partial update of name, is_admin, and password. Enforces: cannot demote the last admin; cannot change your own admin status.

### Steps

- [ ] Add `HandleAdminUpdateUser` to `internal/dashboard/api_admin.go`:

```go
// HandleAdminUpdateUser handles PATCH /-/api/v1/admin/users/{id}.
func (h *Handler) HandleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx) // injected by RequireAuth middleware

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        jsonError(w, "invalid user id", http.StatusBadRequest)
        return
    }

    var body struct {
        Name     *string `json:"name"`
        IsAdmin  *bool   `json:"is_admin"`
        Password *string `json:"password"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        jsonError(w, "invalid JSON", http.StatusBadRequest)
        return
    }

    target, err := h.store.GetUserByID(ctx, targetID)
    if err != nil {
        jsonError(w, "user not found", http.StatusNotFound)
        return
    }

    // Guard: cannot change your own admin status.
    if body.IsAdmin != nil && currentUser.ID == targetID {
        jsonError(w, "cannot modify your own admin status", http.StatusUnprocessableEntity)
        return
    }

    // Guard: cannot demote the last admin.
    if body.IsAdmin != nil && !*body.IsAdmin && target.IsAdmin {
        users, err := h.store.ListUsers(ctx)
        if err != nil {
            jsonError(w, "internal error", http.StatusInternalServerError)
            return
        }
        adminCount := 0
        for _, u := range users {
            if u.IsAdmin {
                adminCount++
            }
        }
        if adminCount <= 1 {
            jsonError(w, "cannot demote the last admin", http.StatusUnprocessableEntity)
            return
        }
    }

    if body.Name != nil {
        target.Name = *body.Name
    }
    if body.IsAdmin != nil {
        target.IsAdmin = *body.IsAdmin
    }
    if body.Password != nil && *body.Password != "" {
        hashed, err := auth.HashPassword(*body.Password)
        if err != nil {
            jsonError(w, "failed to hash password", http.StatusInternalServerError)
            return
        }
        target.Password = hashed
    }

    if err := h.store.UpdateUser(ctx, target); err != nil {
        jsonError(w, "failed to update user", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
```

- [ ] Add imports needed in `api_admin.go`: `"strconv"`, `"github.com/go-chi/chi/v5"`.

- [ ] Wire route temporarily:
  ```go
  r.With(authn.RequireAdmin).Patch("/-/api/v1/admin/users/{id}", h.HandleAdminUpdateUser)
  ```

- [ ] Append to `internal/dashboard/api_admin_test.go`:

```go
func TestAdminUpdateUser_Promote(t *testing.T) {
    srv, adminTok, _ := adminTestServer(t)
    // get user list to find regular user id
    req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, _ := http.DefaultClient.Do(req)
    var users []map[string]interface{}
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
    resp2, _ := http.DefaultClient.Do(req2)
    defer resp2.Body.Close()
    if resp2.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp2.StatusCode)
    }
}

func TestAdminUpdateUser_DemoteLastAdmin(t *testing.T) {
    srv, adminTok, _ := adminTestServer(t)
    // find admin user id
    req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/users", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, _ := http.DefaultClient.Do(req)
    var users []map[string]interface{}
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
    resp2, _ := http.DefaultClient.Do(req2)
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
    var users []map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&users)
    resp.Body.Close()

    var adminID float64
    for _, u := range users {
        if u["email"] == "admin@example.com" {
            adminID = u["id"].(float64)
        }
    }

    // Promote the regular user first so we're not demoting the last admin
    // (so the only-admin guard doesn't fire before the self guard)
    var regularID float64
    for _, u := range users {
        if u["email"] == "user@example.com" {
            regularID = u["id"].(float64)
        }
    }
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
    resp2, _ := http.DefaultClient.Do(req2)
    defer resp2.Body.Close()
    if resp2.StatusCode != http.StatusUnprocessableEntity {
        t.Fatalf("expected 422, got %d", resp2.StatusCode)
    }
}
```

- [ ] Add `"fmt"` and `"strings"` to the test file imports.

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminUpdateUser -timeout 60s
  ```
  Expected: all three subtests pass.

---

## Task 3: Admin user delete API + tests

**Files touched:** `internal/dashboard/api_admin.go`, `internal/dashboard/api_admin_test.go`

**Goal:** `DELETE /-/api/v1/admin/users/{id}` removes user and all their repos from gitStore. Prevents self-deletion.

### Steps

- [ ] Add `HandleAdminDeleteUser` to `internal/dashboard/api_admin.go`:

```go
// HandleAdminDeleteUser handles DELETE /-/api/v1/admin/users/{id}.
func (h *Handler) HandleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        jsonError(w, "invalid user id", http.StatusBadRequest)
        return
    }

    if targetID == currentUser.ID {
        jsonError(w, "cannot delete your own account", http.StatusUnprocessableEntity)
        return
    }

    // Remove all repos owned by this user from the live gitStore before deleting from DB.
    repos, err := h.store.ListReposByOwner(ctx, targetID)
    if err != nil {
        jsonError(w, "failed to list repos", http.StatusInternalServerError)
        return
    }
    for _, repo := range repos {
        h.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
    }

    if err := h.store.DeleteUser(ctx, targetID); err != nil {
        jsonError(w, "failed to delete user", http.StatusInternalServerError)
        return
    }

    if err := h.docSrv.Reload(ctx); err != nil {
        // Non-fatal: log and continue; the delete already succeeded.
        // Use the logger from Handler if available; otherwise just ignore.
        _ = err
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
```

- [ ] Wire route temporarily:
  ```go
  r.With(authn.RequireAdmin).Delete("/-/api/v1/admin/users/{id}", h.HandleAdminDeleteUser)
  ```

- [ ] Define a stub `gitStore` interface and `docSrv` interface at the top of `api_admin.go` (or in a shared `interfaces.go` within the package) if they don't already exist from prior phases. They need at minimum:

```go
// GitStoreRemover is the gitStore capability needed by admin handlers.
type GitStoreRemover interface {
    RemoveRepo(host, owner, name string)
}

// DocReloader is the docSrv capability needed by admin handlers.
type DocReloader interface {
    Reload(ctx context.Context) error
}
```

  If these interfaces already exist in the package from earlier phases, skip this sub-step.

- [ ] Add `"context"` import to `api_admin.go` if not already present.

- [ ] Add a `stubGitStore` test helper to `api_admin_test.go` to record which repos were removed:

```go
type stubGitStore struct {
    removed []string // "host/owner/name"
}

func (s *stubGitStore) RemoveRepo(host, owner, name string) {
    s.removed = append(s.removed, host+"/"+owner+"/"+name)
}

type stubDocSrv struct{}

func (s *stubDocSrv) Reload(_ context.Context) error { return nil }
```

- [ ] Update `adminTestServer` to accept optional `gitStore` and `docSrv` parameters, or create a new variant `adminTestServerWithStubs` that wires them in. Pass the stubs when constructing the `Handler`:

```go
func adminTestServerWithStubs(t *testing.T) (*httptest.Server, string, string, *stubGitStore, db.Store) {
    t.Helper()
    store := db.NewMemStore()
    ctx := context.Background()

    adminUser := &db.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true}
    adminUser.Password, _ = auth.HashPassword("adminpass")
    store.CreateUser(ctx, adminUser)

    regularUser := &db.User{Email: "user@example.com", Name: "Regular", IsAdmin: false}
    regularUser.Password, _ = auth.HashPassword("userpass")
    store.CreateUser(ctx, regularUser)

    authn := auth.New(store)
    adminSess, _ := authn.CreateSession(ctx, adminUser.ID)
    regularSess, _ := authn.CreateSession(ctx, regularUser.ID)

    git := &stubGitStore{}
    doc := &stubDocSrv{}
    h := dashboard.New(store, git, doc, authn)
    srv := httptest.NewServer(h.Handler())
    t.Cleanup(srv.Close)
    return srv, adminSess.Token, regularSess.Token, git, store
}
```

- [ ] Add test cases to `api_admin_test.go`:

```go
func TestAdminDeleteUser_CascadesRepos(t *testing.T) {
    srv, adminTok, _, git, store := adminTestServerWithStubs(t)
    ctx := context.Background()

    // Seed a repo for the regular user.
    users, _ := store.ListUsers(ctx)
    var regularID int64
    for _, u := range users {
        if u.Email == "user@example.com" {
            regularID = u.ID
        }
    }
    store.CreateRepo(ctx, &db.Repo{
        OwnerID:   regularID,
        Host:      "github.com",
        RepoOwner: "acme",
        RepoName:  "docs",
        Status:    "ready",
    })

    req, _ := http.NewRequest("DELETE",
        fmt.Sprintf("%s/-/api/v1/admin/users/%d", srv.URL, regularID), nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
    if len(git.removed) != 1 || git.removed[0] != "github.com/acme/docs" {
        t.Fatalf("expected repo removal, got %v", git.removed)
    }
}

func TestAdminDeleteUser_SelfDelete(t *testing.T) {
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
    ctx := context.Background()

    users, _ := store.ListUsers(ctx)
    var adminID int64
    for _, u := range users {
        if u.Email == "admin@example.com" {
            adminID = u.ID
        }
    }

    req, _ := http.NewRequest("DELETE",
        fmt.Sprintf("%s/-/api/v1/admin/users/%d", srv.URL, adminID), nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusUnprocessableEntity {
        t.Fatalf("expected 422, got %d", resp.StatusCode)
    }
}
```

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminDeleteUser -timeout 60s
  ```
  Expected: both subtests pass.

---

## Task 4: Admin settings API + tests

**Files touched:** `internal/dashboard/api_admin.go`, `internal/dashboard/api_admin_test.go`

**Goal:** `GET /-/api/v1/admin/settings` returns all server_settings as a JSON object. `PATCH /-/api/v1/admin/settings` updates any provided keys and returns `restart_required:true` if `addr` or `cache_dir` was among them.

### Steps

- [ ] Define the canonical list of known settings keys as a package-level slice in `api_admin.go`:

```go
// knownSettings is the ordered list of all server_settings keys served by the API.
var knownSettings = []string{
    "addr",
    "cache_dir",
    "stale_ttl",
    "base_url",
    "oauth_github_client_id",
    "oauth_github_client_secret",
    "oauth_google_client_id",
    "oauth_google_client_secret",
}

// restartRequiredSettings are settings whose change requires a server restart.
var restartRequiredSettings = map[string]bool{
    "addr":      true,
    "cache_dir": true,
}
```

- [ ] Add `HandleAdminGetSettings` to `api_admin.go`:

```go
// HandleAdminGetSettings handles GET /-/api/v1/admin/settings.
func (h *Handler) HandleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    result := make(map[string]string, len(knownSettings))
    for _, key := range knownSettings {
        val, err := h.store.GetSetting(ctx, key)
        if err != nil {
            val = "" // missing keys default to empty string
        }
        result[key] = val
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}
```

- [ ] Add `HandleAdminPatchSettings` to `api_admin.go`:

```go
// HandleAdminPatchSettings handles PATCH /-/api/v1/admin/settings.
func (h *Handler) HandleAdminPatchSettings(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    var body map[string]string
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        jsonError(w, "invalid JSON", http.StatusBadRequest)
        return
    }

    restartNeeded := false
    for key, val := range body {
        if err := h.store.UpsertSetting(ctx, key, val); err != nil {
            jsonError(w, "failed to save setting: "+key, http.StatusInternalServerError)
            return
        }
        if restartRequiredSettings[key] {
            restartNeeded = true
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "ok":               true,
        "restart_required": restartNeeded,
    })
}
```

- [ ] Wire routes temporarily:
  ```go
  r.With(authn.RequireAdmin).Get("/-/api/v1/admin/settings", h.HandleAdminGetSettings)
  r.With(authn.RequireAdmin).Patch("/-/api/v1/admin/settings", h.HandleAdminPatchSettings)
  ```

- [ ] Add tests to `api_admin_test.go`:

```go
func TestAdminGetSettings(t *testing.T) {
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
    ctx := context.Background()
    store.UpsertSetting(ctx, "addr", ":8080")
    store.UpsertSetting(ctx, "cache_dir", "~/.cache/folio")

    req, _ := http.NewRequest("GET", srv.URL+"/-/api/v1/admin/settings", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
    var settings map[string]string
    json.NewDecoder(resp.Body).Decode(&settings)
    if settings["addr"] != ":8080" {
        t.Fatalf("expected :8080, got %q", settings["addr"])
    }
    // All known keys must be present (empty string if not set).
    for _, key := range []string{"addr", "cache_dir", "stale_ttl", "base_url",
        "oauth_github_client_id", "oauth_github_client_secret",
        "oauth_google_client_id", "oauth_google_client_secret"} {
        if _, ok := settings[key]; !ok {
            t.Errorf("missing key %q in settings response", key)
        }
    }
}

func TestAdminPatchSettings_RestartRequired(t *testing.T) {
    srv, adminTok, _, _, _ := adminTestServerWithStubs(t)

    body := strings.NewReader(`{"addr":":9090"}`)
    req, _ := http.NewRequest("PATCH", srv.URL+"/-/api/v1/admin/settings", body)
    req.Header.Set("Content-Type", "application/json")
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    if result["restart_required"] != true {
        t.Fatalf("expected restart_required=true, got %v", result["restart_required"])
    }
}

func TestAdminPatchSettings_NoRestart(t *testing.T) {
    srv, adminTok, _, _, _ := adminTestServerWithStubs(t)

    body := strings.NewReader(`{"base_url":"https://docs.example.com"}`)
    req, _ := http.NewRequest("PATCH", srv.URL+"/-/api/v1/admin/settings", body)
    req.Header.Set("Content-Type", "application/json")
    req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    if result["restart_required"] == true {
        t.Fatal("expected restart_required=false for base_url change")
    }
}
```

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminGetSettings -timeout 60s
  go test ./internal/dashboard/... -run TestAdminPatchSettings -timeout 60s
  ```
  Expected: all subtests pass.

---

## Task 5: Admin user list SSR page + template + test

**Files touched:** `internal/dashboard/admin.go`, `internal/assets/templates/dashboard_admin_users.html`, `internal/dashboard/admin_test.go`

**Goal:** `GET /-/dashboard/admin/` renders an HTML page listing all users. Requires admin role. Non-admins get 403.

### Steps

- [ ] Create `internal/assets/templates/dashboard_admin_users.html`:

```html
{{template "dashboard_base.html" .}}

{{define "content"}}
<div class="dashboard-content">
  <h1>Users</h1>

  {{if .Flash}}
  <div class="flash flash-success">{{.Flash}}</div>
  {{end}}

  <a href="/-/dashboard/admin/users/new" class="button">Add User</a>

  <table class="users-table">
    <thead>
      <tr>
        <th>Name</th>
        <th>Email</th>
        <th>Role</th>
        <th>Created</th>
        <th>Actions</th>
      </tr>
    </thead>
    <tbody>
      {{range .Users}}
      <tr>
        <td>{{.Name}}{{if eq .ID $.User.ID}} <span class="badge badge-self">You</span>{{end}}</td>
        <td>{{.Email}}</td>
        <td>
          {{if .IsAdmin}}
          <span class="badge badge-admin">Admin</span>
          {{else}}
          <span class="badge badge-user">User</span>
          {{end}}
        </td>
        <td>{{.CreatedAt.Format "2006-01-02"}}</td>
        <td>
          <a href="/-/dashboard/admin/users/{{.ID}}">Edit</a>
          &nbsp;
          <form method="POST" action="/-/dashboard/admin/users/{{.ID}}/toggle-admin" style="display:inline">
            {{if .IsAdmin}}
            <button type="submit">Demote</button>
            {{else}}
            <button type="submit">Promote</button>
            {{end}}
          </form>
          &nbsp;
          {{if eq .ID $.User.ID}}
          <button disabled title="Cannot delete your own account">Delete</button>
          {{else}}
          <form method="POST" action="/-/dashboard/admin/users/{{.ID}}/delete" style="display:inline"
                onsubmit="return confirm('Delete user and all their repos?')">
            <button type="submit">Delete</button>
          </form>
          {{end}}
        </td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] Create `internal/dashboard/admin.go` with the `adminUsersData`, `adminUserEditData`, and `adminSettingsData` structs, plus `HandleAdminUsersPage`:

```go
package dashboard

import (
    "net/http"

    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/db"
)

type adminUsersData struct {
    Flash string
    User  *db.User   // current admin (for self-identification in the template)
    Users []*db.User // all users
}

type adminUserEditData struct {
    Flash  string
    User   *db.User // current admin (for self-edit check)
    Target *db.User // user being edited
    Error  string
}

type adminSettingsData struct {
    Flash    string
    User     *db.User
    Settings map[string]string
}

// HandleAdminUsersPage handles GET /-/dashboard/admin/.
func (h *Handler) HandleAdminUsersPage(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    users, err := h.store.ListUsers(ctx)
    if err != nil {
        http.Error(w, "failed to list users", http.StatusInternalServerError)
        return
    }

    flash := flashGet(w, r) // reads and clears the _flash cookie
    data := adminUsersData{
        Flash: flash,
        User:  currentUser,
        Users: users,
    }
    h.renderTemplate(w, "dashboard_admin_users.html", data)
}
```

  Note: `flashGet` and `renderTemplate` are assumed to be defined in the `dashboard` package from earlier phases. Adjust names to match actual helpers from Phase 3/4 if they differ.

- [ ] Create `internal/dashboard/admin_test.go`:

```go
package dashboard_test

import (
    "context"
    "net/http"
    "strings"
    "testing"
)

func TestAdminUsersPage_AsAdmin(t *testing.T) {
    srv, adminTok, _, _, _ := adminTestServerWithStubs(t)

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
    srv, _, regularTok, _, _ := adminTestServerWithStubs(t)

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
```

- [ ] Add `"io"` and `"strings"` to the imports in `admin_test.go`.

- [ ] Wire route temporarily:
  ```go
  r.With(authn.RequireAdmin).Get("/-/dashboard/admin/", h.HandleAdminUsersPage)
  ```

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminUsersPage -timeout 60s
  ```
  Expected: both subtests pass.

---

## Task 6: Admin user edit SSR page + tests

**Files touched:** `internal/dashboard/admin.go`, `internal/assets/templates/dashboard_admin_users.html` (may add separate edit template), `internal/dashboard/admin_test.go`

**Goal:** `GET /-/dashboard/admin/users/{id}` shows an edit form pre-populated with user data. `POST /-/dashboard/admin/users/{id}` processes the update. `POST /-/dashboard/admin/users/{id}/delete` deletes the user (SSR equivalent of the DELETE API). `POST /-/dashboard/admin/users/{id}/toggle-admin` toggles the admin flag.

### Steps

- [ ] Create `internal/assets/templates/dashboard_admin_user_edit.html`:

```html
{{template "dashboard_base.html" .}}

{{define "content"}}
<div class="dashboard-content">
  <h1>Edit User</h1>

  {{if .Error}}
  <div class="flash flash-error">{{.Error}}</div>
  {{end}}
  {{if .Flash}}
  <div class="flash flash-success">{{.Flash}}</div>
  {{end}}

  <form method="POST" action="/-/dashboard/admin/users/{{.Target.ID}}">
    <label>Name
      <input type="text" name="name" value="{{.Target.Name}}" required>
    </label>
    <label>Email
      <input type="email" name="email" value="{{.Target.Email}}" required>
    </label>
    <label>New Password <small>(leave blank to keep current)</small>
      <input type="password" name="password" autocomplete="new-password">
    </label>
    <label>
      {{if eq .Target.ID .User.ID}}
      <input type="checkbox" name="is_admin" disabled {{if .Target.IsAdmin}}checked{{end}}>
      Admin <small>(cannot change your own admin status)</small>
      {{else}}
      <input type="checkbox" name="is_admin" value="true" {{if .Target.IsAdmin}}checked{{end}}>
      Admin
      {{end}}
    </label>
    <button type="submit">Save</button>
    <a href="/-/dashboard/admin/">Cancel</a>
  </form>
</div>
{{end}}
```

- [ ] Add `HandleAdminUserEditPage` (GET) and `HandleAdminUserEditPost` (POST) to `internal/dashboard/admin.go`:

```go
// HandleAdminUserEditPage handles GET /-/dashboard/admin/users/{id}.
func (h *Handler) HandleAdminUserEditPage(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "invalid user id", http.StatusBadRequest)
        return
    }
    target, err := h.store.GetUserByID(ctx, targetID)
    if err != nil {
        http.Error(w, "user not found", http.StatusNotFound)
        return
    }
    data := adminUserEditData{
        Flash:  flashGet(w, r),
        User:   currentUser,
        Target: target,
    }
    h.renderTemplate(w, "dashboard_admin_user_edit.html", data)
}

// HandleAdminUserEditPost handles POST /-/dashboard/admin/users/{id}.
func (h *Handler) HandleAdminUserEditPost(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "invalid user id", http.StatusBadRequest)
        return
    }
    target, err := h.store.GetUserByID(ctx, targetID)
    if err != nil {
        http.Error(w, "user not found", http.StatusNotFound)
        return
    }

    if err := r.ParseForm(); err != nil {
        http.Error(w, "bad form", http.StatusBadRequest)
        return
    }

    target.Name = r.FormValue("name")
    target.Email = r.FormValue("email")

    // is_admin: only update if not editing own record
    if currentUser.ID != targetID {
        target.IsAdmin = r.FormValue("is_admin") == "true"
    }

    if pw := r.FormValue("password"); pw != "" {
        hashed, err := auth.HashPassword(pw)
        if err != nil {
            http.Error(w, "failed to hash password", http.StatusInternalServerError)
            return
        }
        target.Password = hashed
    }

    if err := h.store.UpdateUser(ctx, target); err != nil {
        data := adminUserEditData{User: currentUser, Target: target, Error: "Failed to save: " + err.Error()}
        h.renderTemplate(w, "dashboard_admin_user_edit.html", data)
        return
    }

    flashSet(w, "User updated.")
    http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
}
```

- [ ] Add `HandleAdminUserDeletePost` and `HandleAdminToggleAdmin` to `internal/dashboard/admin.go`:

```go
// HandleAdminUserDeletePost handles POST /-/dashboard/admin/users/{id}/delete.
func (h *Handler) HandleAdminUserDeletePost(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "invalid user id", http.StatusBadRequest)
        return
    }
    if targetID == currentUser.ID {
        flashSet(w, "Cannot delete your own account.")
        http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
        return
    }

    repos, _ := h.store.ListReposByOwner(ctx, targetID)
    for _, repo := range repos {
        h.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
    }
    h.store.DeleteUser(ctx, targetID)
    h.docSrv.Reload(ctx)

    flashSet(w, "User deleted.")
    http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
}

// HandleAdminToggleAdmin handles POST /-/dashboard/admin/users/{id}/toggle-admin.
func (h *Handler) HandleAdminToggleAdmin(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    idStr := chi.URLParam(r, "id")
    targetID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "invalid user id", http.StatusBadRequest)
        return
    }
    if targetID == currentUser.ID {
        flashSet(w, "Cannot change your own admin status.")
        http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
        return
    }

    target, err := h.store.GetUserByID(ctx, targetID)
    if err != nil {
        http.Error(w, "user not found", http.StatusNotFound)
        return
    }

    // Guard: demoting last admin.
    if target.IsAdmin {
        users, _ := h.store.ListUsers(ctx)
        adminCount := 0
        for _, u := range users {
            if u.IsAdmin {
                adminCount++
            }
        }
        if adminCount <= 1 {
            flashSet(w, "Cannot demote the last admin.")
            http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
            return
        }
    }

    target.IsAdmin = !target.IsAdmin
    h.store.UpdateUser(ctx, target)

    flashSet(w, "Admin status updated.")
    http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
}
```

- [ ] Add `"strconv"` and `"github.com/go-chi/chi/v5"` imports to `admin.go`.

- [ ] Wire routes temporarily:
  ```go
  r.With(authn.RequireAdmin).Get("/-/dashboard/admin/users/{id}", h.HandleAdminUserEditPage)
  r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}", h.HandleAdminUserEditPost)
  r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}/delete", h.HandleAdminUserDeletePost)
  r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}/toggle-admin", h.HandleAdminToggleAdmin)
  ```

- [ ] Add tests to `internal/dashboard/admin_test.go`:

```go
func TestAdminUserEditPage_GET(t *testing.T) {
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
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
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
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
```

- [ ] Add `"net/url"` to the test file imports.

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminUserEdit -timeout 60s
  ```
  Expected: both subtests pass.

---

## Task 7: Admin settings SSR page + template + tests

**Files touched:** `internal/dashboard/admin.go`, `internal/assets/templates/dashboard_admin_settings.html`, `internal/dashboard/admin_test.go`

**Goal:** `GET /-/dashboard/admin/settings` renders a settings form with warning banners for restart-required fields. `POST /-/dashboard/admin/settings` persists each setting via `store.UpsertSetting` and redirects.

### Steps

- [ ] Create `internal/assets/templates/dashboard_admin_settings.html`:

```html
{{template "dashboard_base.html" .}}

{{define "content"}}
<div class="dashboard-content">
  <h1>Server Settings</h1>

  {{if .Flash}}
  <div class="flash flash-success">{{.Flash}}</div>
  {{end}}

  <form method="POST" action="/-/dashboard/admin/settings">

    <section>
      <h2>Server</h2>
      <div class="field-warning">
        Changing <strong>Server Address</strong> or <strong>Cache Directory</strong> requires a server restart.
      </div>
      <label>Server Address
        <input type="text" name="addr" value="{{index .Settings "addr"}}">
      </label>
      <label>Cache Directory
        <input type="text" name="cache_dir" value="{{index .Settings "cache_dir"}}">
      </label>
      <label>Stale TTL <small>(e.g. 5m, 0 for webhook-only)</small>
        <input type="text" name="stale_ttl" value="{{index .Settings "stale_ttl"}}">
      </label>
      <label>Base URL <small>(used for webhook URLs; leave blank to auto-detect)</small>
        <input type="text" name="base_url" value="{{index .Settings "base_url"}}">
      </label>
    </section>

    <section>
      <h2>OAuth — GitHub</h2>
      <label>Client ID
        <input type="text" name="oauth_github_client_id" value="{{index .Settings "oauth_github_client_id"}}">
      </label>
      <label>Client Secret
        <input type="password" name="oauth_github_client_secret" value="{{index .Settings "oauth_github_client_secret"}}">
      </label>
    </section>

    <section>
      <h2>OAuth — Google</h2>
      <label>Client ID
        <input type="text" name="oauth_google_client_id" value="{{index .Settings "oauth_google_client_id"}}">
      </label>
      <label>Client Secret
        <input type="password" name="oauth_google_client_secret" value="{{index .Settings "oauth_google_client_secret"}}">
      </label>
    </section>

    <button type="submit">Save Settings</button>
  </form>
</div>
{{end}}
```

- [ ] Add `HandleAdminSettingsPage` (GET) and `HandleAdminSettingsPost` (POST) to `internal/dashboard/admin.go`:

```go
// HandleAdminSettingsPage handles GET /-/dashboard/admin/settings.
func (h *Handler) HandleAdminSettingsPage(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    currentUser := auth.UserFromContext(ctx)

    settings := make(map[string]string, len(knownSettings))
    for _, key := range knownSettings {
        val, err := h.store.GetSetting(ctx, key)
        if err != nil {
            val = ""
        }
        settings[key] = val
    }
    data := adminSettingsData{
        Flash:    flashGet(w, r),
        User:     currentUser,
        Settings: settings,
    }
    h.renderTemplate(w, "dashboard_admin_settings.html", data)
}

// HandleAdminSettingsPost handles POST /-/dashboard/admin/settings.
func (h *Handler) HandleAdminSettingsPost(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    if err := r.ParseForm(); err != nil {
        http.Error(w, "bad form", http.StatusBadRequest)
        return
    }
    for _, key := range knownSettings {
        val := r.FormValue(key)
        h.store.UpsertSetting(ctx, key, val)
    }
    flashSet(w, "Settings saved.")
    http.Redirect(w, r, "/-/dashboard/admin/settings", http.StatusSeeOther)
}
```

  Note: `knownSettings` is defined in `api_admin.go` in the same package, so it's accessible here.

- [ ] Wire routes temporarily:
  ```go
  r.With(authn.RequireAdmin).Get("/-/dashboard/admin/settings", h.HandleAdminSettingsPage)
  r.With(authn.RequireAdmin).Post("/-/dashboard/admin/settings", h.HandleAdminSettingsPost)
  ```

- [ ] Add tests to `internal/dashboard/admin_test.go`:

```go
func TestAdminSettingsPage_GET(t *testing.T) {
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
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
    srv, adminTok, _, _, store := adminTestServerWithStubs(t)
    ctx := context.Background()

    form := url.Values{
        "addr":      {":9090"},
        "cache_dir": {"~/.cache/folio"},
        "stale_ttl": {"10m"},
        "base_url":  {""},
        "oauth_github_client_id":     {""},
        "oauth_github_client_secret": {""},
        "oauth_google_client_id":     {""},
        "oauth_google_client_secret": {""},
    }
    req, _ := http.NewRequest("POST", srv.URL+"/-/dashboard/admin/settings",
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

    val, _ := store.GetSetting(ctx, "addr")
    if val != ":9090" {
        t.Fatalf("expected addr ':9090', got %q", val)
    }
}
```

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestAdminSettingsPage -timeout 60s
  ```
  Expected: both subtests pass.

---

## Task 8: Wire all routes into Handler(); run `task check`

**Files touched:** wherever `Handler()` is defined in `internal/dashboard/` (e.g. `handler.go`)

**Goal:** Consolidate all temporary route registrations from Tasks 1-7 into the canonical `Handler()` function, then verify the entire project passes fmt + vet + tests.

### Steps

- [ ] Open `internal/dashboard/handler.go` (or wherever the chi router is assembled). Remove any duplicate route registrations added during Tasks 1-7 and ensure the following routes are present exactly once, all under a `r.Group` or with `r.With(authn.RequireAdmin)`:

```go
// Admin API routes — all require admin role
r.With(authn.RequireAdmin).Get("/-/api/v1/admin/users", h.HandleAdminListUsers)
r.With(authn.RequireAdmin).Patch("/-/api/v1/admin/users/{id}", h.HandleAdminUpdateUser)
r.With(authn.RequireAdmin).Delete("/-/api/v1/admin/users/{id}", h.HandleAdminDeleteUser)
r.With(authn.RequireAdmin).Get("/-/api/v1/admin/settings", h.HandleAdminGetSettings)
r.With(authn.RequireAdmin).Patch("/-/api/v1/admin/settings", h.HandleAdminPatchSettings)

// Admin SSR routes — all require admin role
r.With(authn.RequireAdmin).Get("/-/dashboard/admin/", h.HandleAdminUsersPage)
r.With(authn.RequireAdmin).Get("/-/dashboard/admin/users/{id}", h.HandleAdminUserEditPage)
r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}", h.HandleAdminUserEditPost)
r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}/delete", h.HandleAdminUserDeletePost)
r.With(authn.RequireAdmin).Post("/-/dashboard/admin/users/{id}/toggle-admin", h.HandleAdminToggleAdmin)
r.With(authn.RequireAdmin).Get("/-/dashboard/admin/settings", h.HandleAdminSettingsPage)
r.With(authn.RequireAdmin).Post("/-/dashboard/admin/settings", h.HandleAdminSettingsPost)
```

  If the router uses a sub-router pattern for admin routes, prefer a route group:
  ```go
  r.Route("/-/api/v1/admin", func(r chi.Router) {
      r.Use(authn.RequireAdmin)
      r.Get("/users", h.HandleAdminListUsers)
      r.Patch("/users/{id}", h.HandleAdminUpdateUser)
      r.Delete("/users/{id}", h.HandleAdminDeleteUser)
      r.Get("/settings", h.HandleAdminGetSettings)
      r.Patch("/settings", h.HandleAdminPatchSettings)
  })
  r.Route("/-/dashboard/admin", func(r chi.Router) {
      r.Use(authn.RequireAdmin)
      r.Get("/", h.HandleAdminUsersPage)
      r.Get("/users/{id}", h.HandleAdminUserEditPage)
      r.Post("/users/{id}", h.HandleAdminUserEditPost)
      r.Post("/users/{id}/delete", h.HandleAdminUserDeletePost)
      r.Post("/users/{id}/toggle-admin", h.HandleAdminToggleAdmin)
      r.Get("/settings", h.HandleAdminSettingsPage)
      r.Post("/settings", h.HandleAdminSettingsPost)
  })
  ```

- [ ] Verify compilation:
  ```bash
  cd /home/pxgray/src/g3doc-clone && go build ./...
  ```
  Fix any compiler errors before proceeding.

- [ ] Run the full admin test suite:
  ```bash
  go test ./internal/dashboard/... -run TestAdmin -timeout 60s
  ```
  Expected: all tests pass.

- [ ] Run the complete project check:
  ```bash
  cd /home/pxgray/src/g3doc-clone && task check
  ```
  Expected: `fmt` reports no changes, `vet` reports no issues, all tests pass across the entire project (including existing doc-serving tests in `internal/render`, `internal/nav`, `internal/web`, etc.).

  If `task check` fails:
  - Formatting issues: run `task fmt` then re-check.
  - Vet issues: fix the reported issue (typically unused imports, incorrect types, or unreachable code) and re-run.
  - Test failures in non-dashboard packages: investigate whether any dashboard package changes inadvertently broke an interface contract from earlier phases and fix accordingly.
