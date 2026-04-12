# Phase 3: Entrypoint + Setup Wizard

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite cmd/folio/main.go with the DB-backed startup model and implement the first-run setup wizard at /-/setup. After this phase, `folio serve` starts; on first run the setup wizard collects server config and creates the admin account.

**Architecture:** main.go opens the SQLite DB, checks setup state, and dispatches to either setup-only mode or full-serve mode. dashboard.Server is a new package that will accumulate handlers across phases 3-6. The master HTTP handler is a simple path-prefix dispatch in main.go.

**Tech Stack:** chi router (already in go.mod), html/template, existing assets embed.

---

## Context

**Module path:** `github.com/pxgray/folio`  
**Go version:** 1.26  
**Working directory:** `/home/pxgray/src/g3doc-clone`

### Current state of main.go

`cmd/folio/main.go` currently accepts a positional TOML config file path, calls `config.Load`, creates a `gitstore.Store` directly, and wires up `web.New`. Phase 3 replaces this entirely with a `folio serve [--db path]` subcommand that opens a SQLite DB.

### APIs available from Phase 1 and 2

```go
// internal/db
db.Open(path string) (*SQLiteStore, error)
store.IsSetupComplete(ctx) (bool, error)
store.GetSetting(ctx, key) (string, error)
store.UpsertSetting(ctx, key, value) error
store.CreateUser(ctx, *User) error  // sets user.ID
store.ListAllRepos(ctx) ([]*Repo, error)
db.RepoStatusPending, db.RepoStatusReady, db.RepoStatusError

// internal/auth
auth.New(store db.Store) *Auth
authn.HashPassword(password string) (string, error)
auth.UserFromContext(ctx) *db.User

// internal/gitstore (updated in Phase 2)
gitstore.New(cacheDir string, defaultStaleTTL time.Duration) *Store
gitstore.RepoEntry{Host, Owner, Name, RemoteURL string, StaleTTL time.Duration}
gitStore.EnsureRepos(ctx, []RepoEntry) error
gitStore.AddRepo(ctx, RepoEntry) error

// internal/web (updated in Phase 2)
web.New(dbStore db.Store, gitStore *gitstore.Store, tmplFS embed.FS, staticFS fs.FS) (*Server, error)
docSrv.Handler() http.Handler
```

### assets package

`internal/assets/assets.go` exposes `assets.TemplateFS` (embed.FS) and `assets.StaticFS` (embed.FS). Templates live in `internal/assets/templates/`. The `static/` subdirectory is obtained via `fs.Sub(assets.StaticFS, "static")`. `setup.html` will be placed in `internal/assets/templates/setup.html` and picked up automatically by the embed glob.

---

## Files Affected

| File | Action |
|---|---|
| `cmd/folio/main.go` | Complete rewrite |
| `internal/dashboard/server.go` | Create — Server struct, New, Handler |
| `internal/dashboard/setup.go` | Create — GET/POST /-/setup handlers |
| `internal/assets/templates/setup.html` | Create — standalone setup wizard form |
| `internal/dashboard/dashboard_test.go` | Create — integration tests |

---

## Task 1: dashboard.Server skeleton

**Goal:** Create the `internal/dashboard` package with a `Server` struct and a `Handler()` method that returns a non-nil `http.Handler`. No real routes yet — everything returns 404. Write one test confirming `Handler()` is non-nil and the server starts without panicking.

### Step 1.1 — Create `internal/dashboard/server.go`

```go
package dashboard

import (
    "embed"
    "html/template"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/db"
    "github.com/pxgray/folio/internal/gitstore"
    "github.com/pxgray/folio/internal/web"
)

// Server accumulates dashboard HTTP handlers across phases 3-6.
type Server struct {
    dbStore       db.Store
    gitStore      *gitstore.Store // nil when setup not yet complete
    authn         *auth.Auth
    docSrv        *web.Server     // nil when setup not yet complete
    tmplFS        embed.FS
    setupComplete bool

    setupTmpl *template.Template
    // additional templates added in later phases
}

// New creates a dashboard Server. gitStore and docSrv may be nil when
// setupComplete is false (setup-only mode).
func New(
    dbStore db.Store,
    gitStore *gitstore.Store,
    authn *auth.Auth,
    docSrv *web.Server,
    tmplFS embed.FS,
    setupComplete bool,
) *Server {
    s := &Server{
        dbStore:       dbStore,
        gitStore:      gitStore,
        authn:         authn,
        docSrv:        docSrv,
        tmplFS:        tmplFS,
        setupComplete: setupComplete,
    }
    s.setupTmpl = template.Must(
        template.ParseFS(tmplFS, "templates/setup.html"),
    )
    return s
}

// Handler returns a chi router mounting all active dashboard routes.
// When setupComplete is false only /-/setup routes are registered.
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()
    r.Route("/-/setup", func(r chi.Router) {
        r.Get("/", s.handleSetupGet)
        r.Post("/", s.handleSetupPost)
    })
    // /-/auth, /-/dashboard, /-/api routes are added in later phases
    return r
}
```

**Notes:**
- `web.Server` must be importable; if `web.New` signature changed in Phase 2 ensure the import matches.
- `template.ParseFS` will fail fast at startup if `setup.html` is missing — catch the panic in tests.

### Step 1.2 — Create `internal/dashboard/dashboard_test.go` (partial — skeleton test only)

```go
package dashboard_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/pxgray/folio/internal/assets"
    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/dashboard"
    "github.com/pxgray/folio/internal/db"
)

// newTestDashboard creates a dashboard server backed by an in-memory DB.
// gitStore and docSrv are nil (setup-only mode).
func newTestDashboard(t *testing.T) (*httptest.Server, db.Store) {
    t.Helper()
    store, err := db.Open(":memory:")
    if err != nil {
        t.Fatalf("db.Open: %v", err)
    }
    t.Cleanup(func() { store.Close() })

    authn := auth.New(store)
    srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
    ts := httptest.NewServer(srv.Handler())
    t.Cleanup(ts.Close)
    return ts, store
}

func TestHandlerNonNil(t *testing.T) {
    store, err := db.Open(":memory:")
    if err != nil {
        t.Fatalf("db.Open: %v", err)
    }
    defer store.Close()

    authn := auth.New(store)
    srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
    if srv.Handler() == nil {
        t.Fatal("Handler() returned nil")
    }
}
```

**Verification:** `go test ./internal/dashboard/... -run TestHandlerNonNil -timeout 60s` must pass.

- [ ] Create `internal/dashboard/server.go` with the struct, `New`, and `Handler` as specified above (setup route stubs may return `http.NotFound` until Task 2)
- [ ] Create placeholder `internal/assets/templates/setup.html` (minimal valid HTML so `template.ParseFS` does not panic — full template added in Task 2)
- [ ] Create `internal/dashboard/dashboard_test.go` with `newTestDashboard` helper and `TestHandlerNonNil`
- [ ] Run `go build ./internal/dashboard/...` — must compile
- [ ] Run `go test ./internal/dashboard/... -run TestHandlerNonNil -timeout 60s` — must pass

---

## Task 2: GET /-/setup + setup.html template

**Goal:** Implement `GET /-/setup`. When setup is not complete it renders `setup.html` with a 200. When setup IS complete it redirects to `/`. Write two tests.

### Step 2.1 — Create `internal/assets/templates/setup.html`

Standalone page (no `base.html` inheritance). Uses pico.min.css from `/-/static/pico.min.css`. No JavaScript. Centered single-column form.

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Folio Setup</title>
  <link rel="stylesheet" href="/-/static/pico.min.css">
  <style>
    body { max-width: 480px; margin: 4rem auto; padding: 0 1rem; }
    h1   { margin-bottom: 1.5rem; }
  </style>
</head>
<body>
  <main>
    <h1>Folio Setup</h1>
    <p>Configure your server and create the admin account.</p>

    {{if .Error}}
    <p role="alert" style="color:var(--pico-color-red-500)">{{.Error}}</p>
    {{end}}

    <form method="POST" action="/-/setup">
      <label for="addr">Server Address</label>
      <input id="addr" name="addr" type="text"
             value="{{or .Addr ":8080"}}"
             placeholder=":8080" required>

      <label for="cache_dir">Cache Directory</label>
      <input id="cache_dir" name="cache_dir" type="text"
             value="{{or .CacheDir "~/.cache/folio"}}"
             placeholder="~/.cache/folio" required>

      <label for="name">Admin Name</label>
      <input id="name" name="name" type="text"
             value="{{.Name}}" required>

      <label for="email">Admin Email</label>
      <input id="email" name="email" type="email"
             value="{{.Email}}" required>

      <label for="password">Admin Password</label>
      <input id="password" name="password" type="password"
             autocomplete="new-password" required minlength="8">

      <button type="submit">Create Admin &amp; Start</button>
    </form>
  </main>
</body>
</html>
```

**Template data struct** (unexported, defined in `setup.go`):

```go
type setupPageData struct {
    Error    string
    Addr     string
    CacheDir string
    Name     string
    Email    string
}
```

### Step 2.2 — Implement `GET /-/setup` in `internal/dashboard/setup.go`

```go
package dashboard

import (
    "net/http"
)

func (s *Server) handleSetupGet(w http.ResponseWriter, r *http.Request) {
    complete, err := s.dbStore.IsSetupComplete(r.Context())
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    if complete {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }
    s.renderSetup(w, setupPageData{
        Addr:     ":8080",
        CacheDir: "~/.cache/folio",
    })
}

func (s *Server) renderSetup(w http.ResponseWriter, data setupPageData) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    if err := s.setupTmpl.Execute(w, data); err != nil {
        http.Error(w, "template error", http.StatusInternalServerError)
    }
}
```

### Step 2.3 — Add tests to `dashboard_test.go`

```go
func TestSetupGet_NotComplete_Returns200(t *testing.T) {
    ts, _ := newTestDashboard(t)
    resp, err := http.Get(ts.URL + "/-/setup")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("want 200, got %d", resp.StatusCode)
    }
}

func TestSetupGet_AlreadyComplete_RedirectsToRoot(t *testing.T) {
    ts, store := newTestDashboard(t)
    ctx := context.Background()
    if err := store.UpsertSetting(ctx, "setup_complete", "true"); err != nil {
        t.Fatal(err)
    }

    // disable redirect following so we can inspect the 303
    client := &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
    resp, err := client.Get(ts.URL + "/-/setup")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusSeeOther {
        t.Fatalf("want 303, got %d", resp.StatusCode)
    }
    if loc := resp.Header.Get("Location"); loc != "/" {
        t.Fatalf("want Location /, got %q", loc)
    }
}
```

**Note:** `context` must be imported in the test file.

**Verification:** `go test ./internal/dashboard/... -run TestSetupGet -timeout 60s` must pass.

- [ ] Write `internal/assets/templates/setup.html` with the full form as specified
- [ ] Create `internal/dashboard/setup.go` with `handleSetupGet`, `renderSetup`, and the `setupPageData` struct
- [ ] Add `TestSetupGet_NotComplete_Returns200` and `TestSetupGet_AlreadyComplete_RedirectsToRoot` to `dashboard_test.go`
- [ ] Run `go test ./internal/dashboard/... -run TestSetupGet -timeout 60s` — both must pass

---

## Task 3: POST /-/setup handler

**Goal:** Implement `POST /-/setup`. On valid input: hash password, create admin user, upsert server settings, redirect to `/-/auth/login`. On missing/invalid fields: re-render the form with an error message. Write two tests.

### Step 3.1 — Implement `POST /-/setup` in `internal/dashboard/setup.go`

```go
func (s *Server) handleSetupPost(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Guard: reject if already complete
    complete, err := s.dbStore.IsSetupComplete(ctx)
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    if complete {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    if err := r.ParseForm(); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    name     := strings.TrimSpace(r.FormValue("name"))
    email    := strings.TrimSpace(r.FormValue("email"))
    password := r.FormValue("password")
    addr     := strings.TrimSpace(r.FormValue("addr"))
    cacheDir := strings.TrimSpace(r.FormValue("cache_dir"))

    // Apply defaults
    if addr == "" {
        addr = ":8080"
    }
    if cacheDir == "" {
        cacheDir = "~/.cache/folio"
    }

    // Validation
    pageData := setupPageData{Addr: addr, CacheDir: cacheDir, Name: name, Email: email}
    switch {
    case name == "":
        pageData.Error = "Admin name is required."
        s.renderSetup(w, pageData)
        return
    case email == "":
        pageData.Error = "Admin email is required."
        s.renderSetup(w, pageData)
        return
    case len(password) < 8:
        pageData.Error = "Password must be at least 8 characters."
        s.renderSetup(w, pageData)
        return
    }

    // Hash password
    hash, err := s.authn.HashPassword(password)
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    // Create admin user
    user := &db.User{
        Email:   email,
        Name:    name,
        Password: hash,
        IsAdmin: true,
    }
    if err := s.dbStore.CreateUser(ctx, user); err != nil {
        pageData.Error = "Failed to create admin account: " + err.Error()
        s.renderSetup(w, pageData)
        return
    }

    // Persist server settings
    settings := [][2]string{
        {"addr", addr},
        {"cache_dir", cacheDir},
        {"stale_ttl", "5m"},
        {"setup_complete", "true"},
    }
    for _, kv := range settings {
        if err := s.dbStore.UpsertSetting(ctx, kv[0], kv[1]); err != nil {
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
    }

    http.Redirect(w, r, "/-/auth/login", http.StatusSeeOther)
}
```

**Import additions for `setup.go`:** `strings`, `github.com/pxgray/folio/internal/db`.

**Note on `auth.HashPassword`:** The spec shows `authn.HashPassword(password)`. Check whether this is a method on `*auth.Auth` or a package-level function. If it is a method on the struct, the call is `s.authn.HashPassword(password)`. If it is a package-level function (`auth.HashPassword`), import accordingly and adjust the call site.

### Step 3.2 — Add tests to `dashboard_test.go`

```go
func TestSetupPost_ValidInput_CreatesUserAndRedirects(t *testing.T) {
    ts, store := newTestDashboard(t)
    ctx := context.Background()

    resp, err := http.PostForm(ts.URL+"/-/setup", url.Values{
        "name":      {"Alice Admin"},
        "email":     {"alice@example.com"},
        "password":  {"securepass1"},
        "addr":      {":9090"},
        "cache_dir": {"/tmp/folio-test"},
    })
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    // PostForm follows redirects; final status should be 200 or 404 depending
    // on whether /-/auth/login exists yet. Just verify the setup_complete flag.
    complete, err := store.IsSetupComplete(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !complete {
        t.Fatal("setup_complete should be true after valid POST")
    }

    // Verify admin user exists
    // (use store.GetUserByEmail if available, or ListAllUsers; adjust to actual API)
    addr, err := store.GetSetting(ctx, "addr")
    if err != nil {
        t.Fatal(err)
    }
    if addr != ":9090" {
        t.Fatalf("want addr :9090, got %q", addr)
    }
}

func TestSetupPost_MissingName_ReturnsFormWithError(t *testing.T) {
    ts, _ := newTestDashboard(t)

    // Use a client that does NOT follow redirects so we see the re-rendered form
    client := &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
    resp, err := client.PostForm(ts.URL+"/-/setup", url.Values{
        "name":     {""},      // empty — should fail validation
        "email":    {"alice@example.com"},
        "password": {"securepass1"},
    })
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("want 200 (re-rendered form), got %d", resp.StatusCode)
    }
    body, _ := io.ReadAll(resp.Body)
    if !strings.Contains(string(body), "required") &&
       !strings.Contains(string(body), "required") {
        // The error message text from handleSetupPost contains "required"
        t.Error("expected error message in re-rendered form")
    }
}
```

**Additional imports for test file:** `io`, `net/url`, `strings` (if not already present).

**Verification:** `go test ./internal/dashboard/... -run TestSetupPost -timeout 60s` must pass.

- [ ] Implement `handleSetupPost` in `internal/dashboard/setup.go` with validation, password hashing, user creation, settings upsert, and redirect
- [ ] Add required imports to `setup.go` (`strings`, `db`)
- [ ] Confirm the `HashPassword` call matches the actual Phase 1 `auth` API (method vs. package function)
- [ ] Add `TestSetupPost_ValidInput_CreatesUserAndRedirects` and `TestSetupPost_MissingName_ReturnsFormWithError` to `dashboard_test.go`
- [ ] Add `io`, `net/url` imports to test file if not already present
- [ ] Run `go test ./internal/dashboard/... -run TestSetupPost -timeout 60s` — both must pass

---

## Task 4: Rewrite cmd/folio/main.go

**Goal:** Replace the TOML-based entrypoint with `folio serve [--db path]`. The binary must compile and start. Add a smoke test that verifies `/` redirects to `/-/setup` when the DB has no setup_complete entry.

### Step 4.1 — Rewrite `cmd/folio/main.go`

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "io/fs"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "github.com/pxgray/folio/internal/assets"
    "github.com/pxgray/folio/internal/auth"
    "github.com/pxgray/folio/internal/dashboard"
    "github.com/pxgray/folio/internal/db"
    "github.com/pxgray/folio/internal/gitstore"
    "github.com/pxgray/folio/internal/web"
)

func main() {
    if len(os.Args) < 2 || os.Args[1] != "serve" {
        fmt.Fprintf(os.Stderr, "usage: folio serve [--db path]\n")
        os.Exit(1)
    }

    serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
    defaultDB := os.Getenv("FOLIO_DB")
    if defaultDB == "" {
        defaultDB = "folio.db"
    }
    dbPath := serveCmd.String("db", defaultDB, "path to SQLite database file")
    if err := serveCmd.Parse(os.Args[2:]); err != nil {
        log.Fatalf("folio: %v", err)
    }

    store, err := db.Open(*dbPath)
    if err != nil {
        log.Fatalf("folio: open db: %v", err)
    }
    defer store.Close()

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // Background session cleanup
    go func() {
        ticker := time.NewTicker(24 * time.Hour)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                if err := store.DeleteExpiredSessions(ctx); err != nil {
                    log.Printf("folio: session cleanup: %v", err)
                }
            case <-ctx.Done():
                return
            }
        }
    }()

    authn := auth.New(store)

    staticFS, err := fs.Sub(assets.StaticFS, "static")
    if err != nil {
        log.Fatalf("folio: static fs: %v", err)
    }

    setupComplete, err := store.IsSetupComplete(ctx)
    if err != nil {
        log.Fatalf("folio: check setup: %v", err)
    }

    var docHandler http.Handler
    var docSrv *web.Server
    var gitStore *gitstore.Store
    addr := ":8080"

    if setupComplete {
        // Load settings
        if v, err := store.GetSetting(ctx, "addr"); err == nil && v != "" {
            addr = v
        }
        cacheDir := "~/.cache/folio"
        if v, err := store.GetSetting(ctx, "cache_dir"); err == nil && v != "" {
            cacheDir = v
        }
        staleTTL := 5 * time.Minute
        if v, err := store.GetSetting(ctx, "stale_ttl"); err == nil && v != "" {
            if d, err := time.ParseDuration(v); err == nil {
                staleTTL = d
            }
        }

        gitStore = gitstore.New(cacheDir, staleTTL)

        // Hydrate gitstore from DB
        repos, err := store.ListAllRepos(ctx)
        if err != nil {
            log.Fatalf("folio: list repos: %v", err)
        }
        entries := make([]gitstore.RepoEntry, 0, len(repos))
        for _, r := range repos {
            if r.Status == db.RepoStatusReady || r.Status == db.RepoStatusPending {
                entries = append(entries, gitstore.RepoEntry{
                    Host:      r.Host,
                    Owner:     r.RepoOwner,
                    Name:      r.RepoName,
                    RemoteURL: r.RemoteURL,
                })
            }
        }
        if err := gitStore.EnsureRepos(ctx, entries); err != nil {
            log.Printf("folio: EnsureRepos: %v", err)
        }

        docSrv, err = web.New(store, gitStore, assets.TemplateFS, staticFS)
        if err != nil {
            log.Fatalf("folio: web.New: %v", err)
        }
        docHandler = docSrv.Handler()
    }

    dashSrv := dashboard.New(store, gitStore, authn, docSrv, assets.TemplateFS, setupComplete)
    dashHandler := dashSrv.Handler()

    combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        p := r.URL.Path
        if strings.HasPrefix(p, "/-/setup") ||
            strings.HasPrefix(p, "/-/auth") ||
            strings.HasPrefix(p, "/-/dashboard") ||
            strings.HasPrefix(p, "/-/api") {
            dashHandler.ServeHTTP(w, r)
            return
        }
        if docHandler != nil {
            docHandler.ServeHTTP(w, r)
        } else {
            http.Redirect(w, r, "/-/setup", http.StatusSeeOther)
        }
    })

    httpSrv := &http.Server{
        Addr:         addr,
        Handler:      combined,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        <-ctx.Done()
        log.Printf("folio: shutting down...")
        shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer shutCancel()
        _ = httpSrv.Shutdown(shutCtx)
    }()

    log.Printf("folio: listening on %s", addr)
    if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("folio: listen: %v", err)
    }
}
```

**Notes on the rewrite:**
- The old `config`, `config.Load`, and `store.EnsureCloned` / `store.OpenLocals` calls are gone entirely.
- `db.RepoStatusPending` is used for the entries filter — adjust to match the exact constant name exported by Phase 1 (`db.RepoStatusPending` vs `db.RepoStatusPendingClone`). Check the Phase 1 spec if uncertain.
- `store.DeleteExpiredSessions` must exist in the Phase 1 DB API. If it is not present, omit the goroutine and add a TODO comment.
- If `web.New` signature has changed from Phase 2, update accordingly. The Phase 2 signature shown here is: `web.New(dbStore db.Store, gitStore *gitstore.Store, tmplFS embed.FS, staticFS fs.FS)`.
- The old `config` package is no longer imported.

### Step 4.2 — Add smoke test to `dashboard_test.go`

```go
// TestMainSmoke_RedirectsToSetupWhenNotConfigured uses newTestDashboard
// (which creates an incomplete DB) and verifies that hitting / on the
// master combined handler redirects to /-/setup.
func TestMainSmoke_RedirectsToSetupWhenNotConfigured(t *testing.T) {
    ts, _ := newTestDashboard(t)

    client := &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
    // The test server exposes only the dashboard handler, not the combined
    // handler from main.go. To test the combined dispatch logic, we wire it
    // directly here.
    store, err := db.Open(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()

    authn := auth.New(store)
    dashSrv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
    dashHandler := dashSrv.Handler()

    combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        p := r.URL.Path
        if strings.HasPrefix(p, "/-/setup") ||
            strings.HasPrefix(p, "/-/auth") ||
            strings.HasPrefix(p, "/-/dashboard") ||
            strings.HasPrefix(p, "/-/api") {
            dashHandler.ServeHTTP(w, r)
            return
        }
        // docHandler is nil (setup not complete)
        http.Redirect(w, r, "/-/setup", http.StatusSeeOther)
    })

    combinedTS := httptest.NewServer(combined)
    defer combinedTS.Close()
    _ = ts // suppress unused variable; ts is from newTestDashboard helper

    resp, err := client.Get(combinedTS.URL + "/")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusSeeOther {
        t.Fatalf("want 303, got %d", resp.StatusCode)
    }
    if loc := resp.Header.Get("Location"); loc != "/-/setup" {
        t.Fatalf("want Location /-/setup, got %q", loc)
    }
}
```

**Note:** `strings` must be imported in the test file.

**Verification commands:**
```bash
go build ./cmd/folio
go test ./internal/dashboard/... -timeout 60s
```

Both must succeed without errors.

- [ ] Rewrite `cmd/folio/main.go` as specified; remove all references to `internal/config`
- [ ] Verify that `db.RepoStatusPending` (or the correct constant name) is exported by Phase 1; adjust if needed
- [ ] Verify that `store.DeleteExpiredSessions` exists in Phase 1 DB API; if absent, add a TODO comment and remove the goroutine
- [ ] Add `TestMainSmoke_RedirectsToSetupWhenNotConfigured` to `dashboard_test.go`
- [ ] Run `go build ./cmd/folio` — must produce `bin/folio` or the binary at the default path without error
- [ ] Run `go test ./internal/dashboard/... -timeout 60s` — all tests must pass

---

## End-to-End Verification

After all four tasks are complete, run the full check:

```bash
go build ./cmd/folio && go test ./internal/dashboard/... -timeout 60s
```

Expected outcome:
- Binary compiles without errors or warnings.
- All dashboard tests pass (5+ tests: `TestHandlerNonNil`, `TestSetupGet_NotComplete_Returns200`, `TestSetupGet_AlreadyComplete_RedirectsToRoot`, `TestSetupPost_ValidInput_CreatesUserAndRedirects`, `TestSetupPost_MissingName_ReturnsFormWithError`, `TestMainSmoke_RedirectsToSetupWhenNotConfigured`).

---

## Common Pitfalls

1. **`template.ParseFS` glob path** — the embed FS in `assets.go` uses `//go:embed templates/*`. Calling `template.ParseFS(tmplFS, "templates/setup.html")` is the correct form. If it uses a different glob (e.g. `templates/**`), adjust the path argument.

2. **`auth.HashPassword` API** — the spec shows it as `authn.HashPassword(password)`. If Phase 1 implemented it as a package-level function `auth.HashPassword(password)` instead of a method, update the call in `handleSetupPost` accordingly.

3. **`db.User` field names** — the spec shows `Password` (string) and `IsAdmin` (bool). Verify these match the struct exported by Phase 1 exactly. Common variations: `PasswordHash` instead of `Password`.

4. **`web.New` signature drift** — if Phase 2's `web.New` kept the old `*config.Config` parameter rather than accepting `db.Store`, the call in `main.go` will not compile. Reconcile with the actual Phase 2 output before proceeding.

5. **`db.RepoStatusPending` vs `db.RepoStatusPendingClone`** — the spec uses both names in different places. Use whatever constant Phase 1 actually exported; grep `internal/db` to confirm.

6. **`store.Close()` method** — `db.Open` returns a `*SQLiteStore`; confirm it implements `io.Closer` or has a `.Close()` method before calling `defer store.Close()` in main.

7. **Static assets in tests** — `setup.html` references `/-/static/pico.min.css`. Tests use `httptest.NewServer` without the static file server, so CSS 404s are expected and harmless. Do not add static serving to the test helper.
