# Phase 4: Auth UI + API Repo Endpoints

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add login/logout pages, OAuth redirect handlers, and the JSON API for auth and repo management. After this phase, users can log in (email/password or OAuth) and manage repos via the API.

**Architecture:** Login/logout are standard form-POST handlers. The OAuth flow stores state in a short-lived cookie. API endpoints return JSON and are protected by auth.RequireAuth middleware. Repo add triggers a background clone; status is tracked in db.Repo.Status.

**Tech Stack:** golang.org/x/oauth2, encoding/json, net/http, chi.

---

## File Map

| File | Purpose |
|---|---|
| `internal/dashboard/login.go` | GET/POST `/-/auth/login`, POST `/-/auth/logout`, GitHub/Google OAuth redirect + callback handlers |
| `internal/dashboard/api_auth.go` | `POST /-/api/v1/auth/login`, `POST /-/api/v1/auth/logout`, `GET /-/api/v1/auth/me` |
| `internal/dashboard/api_repos.go` | Full repo CRUD + sync (`/-/api/v1/repos`) |
| `internal/assets/templates/login.html` | Standalone login page (email/password form + OAuth buttons) |
| `internal/assets/templates/dashboard_base.html` | Shared dashboard layout (sidebar nav, flash banner, no topnav/TOC) |

Flash cookie helpers live in `internal/dashboard/server.go` (the existing dashboard server file from Phase 3).

---

## Shared Conventions

- `writeJSON(w, code, v)` — set `Content-Type: application/json`, write status code, JSON-encode `v`. Used by every API handler.
- Session cookie name: `session`. Attributes: `HttpOnly; Secure; SameSite=Lax; Path=/`.
- OAuth state cookie name: `oauth_state`. MaxAge: 600 (10 min). Same security attributes.
- Flash cookie name: `_flash`. MaxAge: 60 on set; MaxAge: -1 to clear.
- On validation failure API handlers return `422` with `{"error": "..."}`.
- On auth failure API handlers return `401`; dashboard handlers redirect to `/-/auth/login`.
- All repo API endpoints scope to `currentUser.ID` via `auth.UserFromContext(r.Context())`.

---

## Task 1: dashboard_base.html + flash cookie helpers

**Goal:** Lay the template and helper groundwork that every subsequent dashboard page builds on.

### Steps

- [ ] **1a. Write `internal/assets/templates/dashboard_base.html`**

  Minimal standalone HTML document (does not extend `base.html`):
  - `<head>`: charset, viewport, title `{{.Title}} — Folio`, link `/-/static/pico.min.css`, link `/-/static/style.css`.
  - `<body class="dashboard">`: a `<div class="dash-layout">` containing:
    - `<nav class="dash-sidebar">`:
      - `<a href="/-/dashboard/">Repos</a>`
      - `<a href="/-/dashboard/settings">Settings</a>`
      - `{{if .IsAdmin}}<a href="/-/dashboard/admin/">Admin</a>{{end}}`
    - `<main class="dash-main">`:
      - `{{if .Flash}}<div role="alert" class="flash">{{.Flash}}</div>{{end}}`
      - `{{block "content" .}}{{end}}`
  - Include `<script src="/-/static/folio.js"></script>` (theme toggle only; no other JS).
  - No topnav, no TOC, no breadcrumbs.

- [ ] **1b. Add flash helpers to `internal/dashboard/server.go`**

  ```go
  func setFlash(w http.ResponseWriter, msg string) {
      http.SetCookie(w, &http.Cookie{
          Name: "_flash", Value: url.QueryEscape(msg),
          Path: "/", MaxAge: 60,
      })
  }

  func getFlash(w http.ResponseWriter, r *http.Request) string {
      c, err := r.Cookie("_flash")
      if err != nil {
          return ""
      }
      http.SetCookie(w, &http.Cookie{Name: "_flash", Value: "", Path: "/", MaxAge: -1})
      v, _ := url.QueryUnescape(c.Value)
      return v
  }
  ```

  Add `"net/url"` import if not already present.

- [ ] **1c. Write test `TestFlashCookieRoundTrip` in `internal/dashboard/server_test.go`**

  Use `httptest.NewRecorder`. Call `setFlash(w, "hello world")`. Verify a `Set-Cookie` header for `_flash` is present. Then create a new recorder and a synthetic request carrying that cookie value, call `getFlash(w2, r2)`, assert the returned string equals `"hello world"` and that a clearing `Set-Cookie` (MaxAge=-1) was emitted.

- [ ] **1d. Run `go test ./internal/dashboard/... -run TestFlash -timeout 60s`** — must pass.

---

## Task 2: login.html + GET /-/auth/login

**Goal:** Render the login page; verify it returns HTTP 200 with the form present.

### Steps

- [ ] **2a. Write `internal/assets/templates/login.html`**

  Standalone page (does not use `dashboard_base.html`):
  - `<head>`: charset, viewport, title `Sign in — Folio`, CSS links same as dashboard_base.
  - `<body>`: centered `<main class="login-box">`:
    - `<h1>Sign in</h1>`
    - `{{if .Error}}<p class="error">{{.Error}}</p>{{end}}`
    - `<form method="POST" action="/-/api/v1/auth/login">`:
      - Email `<input type="email" name="email" required>`
      - Password `<input type="password" name="password" required>`
      - `<button type="submit">Sign in</button>`
    - `<div class="oauth-buttons">`:
      - `<a href="/-/auth/github" class="btn-github">Sign in with GitHub</a>`
      - `<a href="/-/auth/google" class="btn-google">Sign in with Google</a>`
  - No JavaScript.

- [ ] **2b. Add `Server` struct fields for login template**

  In `internal/dashboard/server.go`, add:
  ```go
  loginTmpl *template.Template
  ```
  Parse `dashboard_base.html` and `login.html` in the `New` constructor (similar to how `internal/web/server.go` parses `base.html` + `doc.html`). The login template is standalone, so parse it independently (no base template needed for login.html — it is its own root).

- [ ] **2c. Add `handleLoginGet` in `internal/dashboard/login.go`**

  ```go
  func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
      data := map[string]any{"Title": "Sign in", "Error": r.URL.Query().Get("error")}
      s.loginTmpl.Execute(w, data)
  }
  ```

  Register `GET /-/auth/login` in `Server.Handler()`.

- [ ] **2d. Write test `TestLoginPageGet` in `internal/dashboard/login_test.go`**

  Use `httptest.NewServer(srv.Handler())`. `GET /-/auth/login` → assert status 200 and response body contains `<form` and `/-/api/v1/auth/login`.

- [ ] **2e. Run `go test ./internal/dashboard/... -run TestLoginPage -timeout 60s`** — must pass.

---

## Task 3: POST /-/api/v1/auth/login + GET /-/api/v1/auth/me

**Goal:** Email/password login sets the session cookie and returns user JSON; `/me` returns the current user when authenticated.

### Steps

- [ ] **3a. Add `writeJSON` helper to `internal/dashboard/api_auth.go`**

  ```go
  func writeJSON(w http.ResponseWriter, code int, v any) {
      w.Header().Set("Content-Type", "application/json")
      w.WriteHeader(code)
      json.NewEncoder(w).Encode(v)
  }
  ```

- [ ] **3b. Implement `handleAPILogin` in `internal/dashboard/api_auth.go`**

  - Decode JSON body into `struct{ Email, Password string }`.
  - Call `store.GetUserByEmail(ctx, email)`. On not-found or error: `writeJSON(w, 422, map[string]string{"error":"invalid credentials"})`.
  - Call `authn.CheckPassword(user.Password, password)`. On false: same 422.
  - Call `authn.NewSession(ctx, user.ID)`. On error: 500.
  - Set cookie:
    ```go
    http.SetCookie(w, &http.Cookie{
        Name:     "session",
        Value:    session.Token,
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteLaxMode,
    })
    ```
  - `writeJSON(w, 200, userResponse(user))` where `userResponse` returns a small struct:
    ```go
    type userResp struct {
        ID      int64  `json:"id"`
        Email   string `json:"email"`
        Name    string `json:"name"`
        IsAdmin bool   `json:"is_admin"`
    }
    ```

- [ ] **3c. Implement `handleAPIMe` in `internal/dashboard/api_auth.go`**

  Protected by `authn.RequireAuth`. Extract user via `auth.UserFromContext(r.Context())`. Return `writeJSON(w, 200, userResponse(user))`.

- [ ] **3d. Register routes in `Server.Handler()`**

  ```
  POST /-/api/v1/auth/login  → handleAPILogin
  GET  /-/api/v1/auth/me     → authn.RequireAuth(handleAPIMe)
  ```

- [ ] **3e. Write tests in `internal/dashboard/api_auth_test.go`**

  Bootstrap: in-memory SQLite DB + call `auth.HashPassword("secret")`, create a user row with that hash via `store.CreateUser`.

  **`TestAPILoginValidCreds`**: POST JSON `{"email":"test@example.com","password":"secret"}` to `/-/api/v1/auth/login` → assert 200, response body contains `"email"`, and `Set-Cookie` header contains `session=`.

  **`TestAPILoginBadPassword`**: same but wrong password → assert 422, body contains `"error"`.

  **`TestAPILoginUnknownEmail`**: non-existent email → assert 422.

  **`TestAPIMeWithSession`**: obtain a session token via direct `authn.NewSession` call, craft request with `Cookie: session=<token>`, `GET /-/api/v1/auth/me` → assert 200 and body contains the user's email.

  **`TestAPIMeWithoutSession`**: `GET /-/api/v1/auth/me` with no cookie → assert 401.

- [ ] **3f. Run `go test ./internal/dashboard/... -run TestAPILogin -timeout 60s`** and `go test ./internal/dashboard/... -run TestAPIMe -timeout 60s` — both must pass.

---

## Task 4: POST /-/api/v1/auth/logout + POST /-/auth/logout

**Goal:** Both logout endpoints clear the session and redirect/respond appropriately.

### Steps

- [ ] **4a. Implement `handleAPILogout` in `internal/dashboard/api_auth.go`**

  - Read the `session` cookie. If absent: still return 200 `{"ok":true}`.
  - Call `store.DeleteSession(ctx, token)` (Phase 2 DB method).
  - Clear cookie: `http.SetCookie(w, &http.Cookie{Name:"session", Value:"", Path:"/", MaxAge:-1})`.
  - `writeJSON(w, 200, map[string]bool{"ok":true})`.

- [ ] **4b. Implement `handleFormLogout` in `internal/dashboard/login.go`**

  Same session-deletion logic as `handleAPILogout`, then `http.Redirect(w, r, "/-/auth/login", http.StatusSeeOther)`.

- [ ] **4c. Register routes**

  ```
  POST /-/api/v1/auth/logout → handleAPILogout
  POST /-/auth/logout        → handleFormLogout
  ```

- [ ] **4d. Write test `TestAPILogoutClearsCookie` in `internal/dashboard/api_auth_test.go`**

  Create a session via `authn.NewSession`. POST to `/-/api/v1/auth/logout` with that session cookie. Assert 200 and that `Set-Cookie` contains `session=` with `Max-Age=0` or `Max-Age=-1`.

- [ ] **4e. Run `go test ./internal/dashboard/... -run TestAPILogout -timeout 60s`** — must pass.

---

## Task 5: OAuth redirect handlers (GitHub + Google)

**Goal:** Browser-facing OAuth flows; state cookie prevents CSRF; callback creates or links user accounts.

### Steps

- [ ] **5a. Implement `handleGitHubOAuth` in `internal/dashboard/login.go`**

  - Read `oauth_github_client_id` and `oauth_github_client_secret` from `store.GetSetting`.
  - If either is empty: redirect to `/-/auth/login?error=github_not_configured`.
  - Generate 16-byte random state (`crypto/rand`), hex-encode it.
  - Set `oauth_state` cookie: `HttpOnly; Secure; SameSite=Lax; Path=/; MaxAge=600`.
  - Build OAuth config via `authn.GitHubOAuthConfig(auth.OAuthConfig{ClientID:..., ClientSecret:..., RedirectURL: baseURL+"/-/auth/github/callback"})`.
  - `http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)`.

- [ ] **5b. Implement `handleGitHubCallback` in `internal/dashboard/login.go`**

  - Read `oauth_state` cookie; compare with `r.URL.Query().Get("state")`. On mismatch: 400.
  - Clear `oauth_state` cookie immediately (MaxAge=-1).
  - Exchange code via `cfg.Exchange(ctx, code)`.
  - Call `auth.FetchGitHubProfile(ctx, token, cfg)` → `*auth.OAuthProfile{ID, Email, Name}`.
  - `store.GetUserByOAuth(ctx, "github", profile.ID)`:
    - Found: proceed to login.
    - `ErrNotFound`: call `store.CreateUser` + `store.CreateOAuthAccount`. Then login.
  - `authn.NewSession(ctx, user.ID)`, set `session` cookie, redirect to `/-/dashboard/`.

- [ ] **5c. Implement `handleGoogleOAuth` + `handleGoogleCallback`**

  Identical pattern to 5a/5b, using `authn.GoogleOAuthConfig` and the `google` provider string. Setting keys: `oauth_google_client_id`, `oauth_google_client_secret`. Redirect URL: `baseURL+"/-/auth/google/callback"`.

- [ ] **5d. Register routes**

  ```
  GET /-/auth/github           → handleGitHubOAuth
  GET /-/auth/github/callback  → handleGitHubCallback
  GET /-/auth/google           → handleGoogleOAuth
  GET /-/auth/google/callback  → handleGoogleCallback
  ```

- [ ] **5e. Write test `TestGitHubOAuthRedirect` in `internal/dashboard/login_test.go`**

  Seed `server_settings` with dummy GitHub client ID/secret. `GET /-/auth/github` → assert status 302 and `Location` header starts with `https://github.com/login/oauth/authorize`. Assert `Set-Cookie` contains `oauth_state=`.

- [ ] **5f. Write test `TestGitHubOAuthNotConfigured`**

  No settings seeded. `GET /-/auth/github` → assert redirect to `/-/auth/login?error=github_not_configured`.

- [ ] **5g. Run `go test ./internal/dashboard/... -run TestGitHub -timeout 60s`** — must pass.

---

## Task 6: GET + POST /-/api/v1/repos (list + create)

**Goal:** Users can list their repos and add new ones; creation is non-blocking (background clone).

### Steps

- [ ] **6a. Define request/response types in `internal/dashboard/api_repos.go`**

  ```go
  type createRepoReq struct {
      Host          string `json:"host"`
      Owner         string `json:"owner"`
      Repo          string `json:"repo"`
      RemoteURL     string `json:"remote_url"`
      WebhookSecret string `json:"webhook_secret"`
      TrustedHTML   bool   `json:"trusted_html"`
  }
  ```

  Return type is `db.Repo` marshaled as JSON (add `json` struct tags to `db.Repo` in the db package if not already present, or define a local `repoResp` struct mirroring the fields).

- [ ] **6b. Implement `handleAPIListRepos` in `internal/dashboard/api_repos.go`**

  - Extract user from context.
  - `store.ListReposByOwner(ctx, user.ID)`.
  - `writeJSON(w, 200, repos)`.

- [ ] **6c. Implement `handleAPICreateRepo` in `internal/dashboard/api_repos.go`**

  - Decode `createRepoReq`. Validate `Host`, `Owner`, `Repo` are non-empty; on failure `writeJSON(w, 422, ...)`.
  - Build `db.Repo{OwnerID: user.ID, Host: req.Host, RepoOwner: req.Owner, RepoName: req.Repo, RemoteURL: req.RemoteURL, WebhookSecret: req.WebhookSecret, TrustedHTML: req.TrustedHTML, Status: db.RepoStatusPending}`.
  - `store.CreateRepo(ctx, &repo)` — on error 500.
  - `writeJSON(w, 202, repo)` immediately.
  - Launch goroutine:
    ```go
    go func(id int64) {
        bgCtx := context.Background()
        entry := gitstore.RepoEntry{Host: repo.Host, Owner: repo.RepoOwner, Name: repo.RepoName, RemoteURL: remoteURL(repo)}
        if err := s.gitStore.AddRepo(bgCtx, entry); err != nil {
            s.store.UpdateRepoStatus(bgCtx, id, db.RepoStatusError, err.Error())
            return
        }
        s.store.UpdateRepoStatus(bgCtx, id, db.RepoStatusReady, "")
        s.docSrv.Reload(bgCtx)
    }(repo.ID)
    ```
  - `remoteURL` helper: returns `repo.RemoteURL` if non-empty, else constructs `"https://{host}/{owner}/{repo}.git"`.

- [ ] **6d. Register routes** (behind `authn.RequireAuth`)

  ```
  GET  /-/api/v1/repos → handleAPIListRepos
  POST /-/api/v1/repos → handleAPICreateRepo
  ```

- [ ] **6e. Write tests in `internal/dashboard/api_repos_test.go`**

  **`TestAPICreateRepo`**: POST JSON `{"host":"github.com","owner":"acme","repo":"docs"}` with a valid session cookie → assert 202, body contains `"status":"pending_clone"`, and the DB row exists.

  **`TestAPIListRepos`**: seed two repos for user A and one for user B. Authenticated as user A, `GET /-/api/v1/repos` → assert 200, body is a JSON array of length 2 containing only user A's repos.

  **`TestAPICreateRepoMissingFields`**: POST `{}` → assert 422.

- [ ] **6f. Run `go test ./internal/dashboard/... -run TestAPICreate -timeout 60s`** and `go test ./internal/dashboard/... -run TestAPIList -timeout 60s`** — must pass.

---

## Task 7: GET/PATCH/DELETE /-/api/v1/repos/{id} + sync

**Goal:** Individual repo read, update, delete, and sync with ownership enforcement.

### Steps

- [ ] **7a. Implement `handleAPIGetRepo` in `internal/dashboard/api_repos.go`**

  - Parse `chi.URLParam(r, "id")` as `int64`.
  - `store.GetRepo(ctx, id)`. On not-found: 404. Check `repo.OwnerID == user.ID || user.IsAdmin`; on false: 403.
  - `writeJSON(w, 200, repo)`.

- [ ] **7b. Implement `handleAPIUpdateRepo`**

  - Same ownership check as GET.
  - Decode partial update body:
    ```go
    type updateRepoReq struct {
        WebhookSecret *string `json:"webhook_secret"`
        TrustedHTML   *bool   `json:"trusted_html"`
        StaleTTLSecs  *int    `json:"stale_ttl_secs"`
        RemoteURL     *string `json:"remote_url"`
    }
    ```
    Pointer fields: only set `repo` fields when pointer is non-nil.
  - `store.UpdateRepo(ctx, &repo)`. On error: 500.
  - `s.docSrv.Reload(ctx)`.
  - `writeJSON(w, 200, repo)`.

- [ ] **7c. Implement `handleAPIDeleteRepo`**

  - Ownership check (same as GET; admins may delete any repo).
  - `store.DeleteRepo(ctx, id)`. On error: 500.
  - `s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)`.
  - `s.docSrv.Reload(ctx)`.
  - `w.WriteHeader(204)`.

- [ ] **7d. Implement `handleAPIRepoSync`**

  - Ownership check.
  - Obtain the live `gitstore.Repo` from `s.gitStore` and call `FetchNow(ctx)`. On error: 500 with `{"error":...}`.
  - `writeJSON(w, 200, map[string]bool{"ok":true})`.

- [ ] **7e. Register routes** (all behind `authn.RequireAuth`)

  ```
  GET    /-/api/v1/repos/{id}       → handleAPIGetRepo
  PATCH  /-/api/v1/repos/{id}       → handleAPIUpdateRepo
  DELETE /-/api/v1/repos/{id}       → handleAPIDeleteRepo
  POST   /-/api/v1/repos/{id}/sync  → handleAPIRepoSync
  ```

- [ ] **7f. Write tests in `internal/dashboard/api_repos_test.go`**

  **`TestAPIDeleteRepoOwnership`**: create repo owned by user A. Authenticate as user B, `DELETE /-/api/v1/repos/{id}` → assert 403. Then authenticate as user A → assert 204. Verify row is gone from DB.

  **`TestAPIDeleteRepoCascade`**: after successful delete, assert `gitStore.RemoveRepo` was called (use a test-double `*gitstore.Store` or verify the store's repo map no longer contains the key).

  **`TestAPIPatchRepo`**: PATCH `{"webhook_secret":"new-secret"}` → assert 200, body contains `"webhook_secret":"new-secret"`, other fields unchanged.

  **`TestAPIGetRepoNotFound`**: `GET /-/api/v1/repos/99999` → assert 404.

- [ ] **7g. Run `go test ./internal/dashboard/... -run TestAPIDelete -timeout 60s`** and `go test ./internal/dashboard/... -run TestAPIPatch -timeout 60s`** — must pass.

---

## Task 8: Wire all routes into dashboard.Server.Handler()

**Goal:** Confirm all route paths are registered and return expected status codes; catch any missing registration.

### Steps

- [ ] **8a. Audit `Server.Handler()` in `internal/dashboard/server.go`**

  Ensure all routes from Tasks 1–7 are registered in one place:

  ```
  GET  /-/auth/login                    → handleLoginGet
  POST /-/auth/logout                   → handleFormLogout
  GET  /-/auth/github                   → handleGitHubOAuth
  GET  /-/auth/github/callback          → handleGitHubCallback
  GET  /-/auth/google                   → handleGoogleOAuth
  GET  /-/auth/google/callback          → handleGoogleCallback

  POST /-/api/v1/auth/login             → handleAPILogin
  POST /-/api/v1/auth/logout            → handleAPILogout
  GET  /-/api/v1/auth/me                → RequireAuth(handleAPIMe)

  GET  /-/api/v1/repos                  → RequireAuth(handleAPIListRepos)
  POST /-/api/v1/repos                  → RequireAuth(handleAPICreateRepo)
  GET  /-/api/v1/repos/{id}             → RequireAuth(handleAPIGetRepo)
  PATCH /-/api/v1/repos/{id}            → RequireAuth(handleAPIUpdateRepo)
  DELETE /-/api/v1/repos/{id}           → RequireAuth(handleAPIDeleteRepo)
  POST /-/api/v1/repos/{id}/sync        → RequireAuth(handleAPIRepoSync)
  ```

- [ ] **8b. Write smoke test `TestAPIRoutesSmoke` in `internal/dashboard/smoke_test.go`**

  Using `httptest.NewServer` with a fully wired server (in-memory SQLite, no real git):

  | Route | Method | Expected status (unauthenticated) |
  |---|---|---|
  | `/-/auth/login` | GET | 200 |
  | `/-/api/v1/auth/login` | POST (empty body) | 422 |
  | `/-/api/v1/auth/me` | GET | 401 |
  | `/-/api/v1/repos` | GET | 401 |
  | `/-/api/v1/repos` | POST | 401 |
  | `/-/api/v1/repos/1` | GET | 401 |
  | `/-/api/v1/repos/1` | PATCH | 401 |
  | `/-/api/v1/repos/1` | DELETE | 401 |
  | `/-/api/v1/repos/1/sync` | POST | 401 |

  For authenticated variants: create user + session, add `Cookie: session=<token>` header:

  | Route | Method | Expected status |
  |---|---|---|
  | `/-/api/v1/auth/me` | GET | 200 |
  | `/-/api/v1/repos` | GET | 200 |
  | `/-/api/v1/repos` | POST (valid body) | 202 |

- [ ] **8c. Run full suite `go test ./internal/dashboard/... -run TestAPI -timeout 60s`** — all must pass.

- [ ] **8d. Run `go vet ./internal/dashboard/...`** — no warnings.

---

## Dependency Notes

- `golang.org/x/oauth2` must be present in `go.mod` (added in Phase 3 for auth package; verify with `go list -m golang.org/x/oauth2`).
- `modernc.org/sqlite` (Phase 1) used by test helpers for in-memory DB.
- No new direct dependencies are introduced in Phase 4.

## Out of Scope

- Dashboard SSR pages (`/-/dashboard/*` HTML) — Phase 5.
- Admin user/settings API — Phase 5.
- Setup wizard — Phase 2 (already complete).
- Password reset — out of scope per design spec.
