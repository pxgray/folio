# Phase 5: Dashboard Repo Management UI

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-rendered dashboard pages for repo management (list, add, edit, delete, sync) and account settings. After this phase, users can manage repos entirely through the browser.

**Architecture:** SSR form POST handlers call business logic directly (same DB calls as the API). Templates extend dashboard_base.html. Delete confirmation uses minimal inline JS (onclick confirm). Flash messages use a short-lived cookie.

**Tech Stack:** html/template (server-side rendering), existing chi router, no new dependencies.

---

## Prerequisites

This plan assumes Phases 1–4 are complete and the following are available:

- `internal/db` — `db.Store` interface with `CreateRepo`, `GetRepo`, `UpdateRepo`, `DeleteRepo`, `ListReposByOwner`, `GetUser`, `UpdateUser` methods; `db.Repo`, `db.User`, `db.RepoStatus*` constants.
- `internal/auth` — `auth.Auth` with `RequireAuth` middleware; `auth.UserFromContext(ctx)` returns `*db.User`.
- `internal/dashboard` package already contains `server.go` (the `Server` struct and `Handler()`) from Phase 4 API work, with fields `dbStore db.Store`, `gitStore *gitstore.Store`, `authn *auth.Auth`, `tmplFS embed.FS`.
- `internal/assets/templates/dashboard_base.html` already exists (sidebar layout, flash banner) from Phase 4.

If `internal/dashboard` does not yet exist, create `internal/dashboard/server.go` with the stub shown in Task 5 before starting.

---

## Flash message helpers

The flash helpers are shared across all dashboard handlers. Add them once, in `internal/dashboard/flash.go`, before starting any task that calls `setFlash` or `flash`.

```go
package dashboard

import (
    "net/http"
    "time"
)

const flashCookieName = "_flash"

// setFlash writes a one-time flash message into a short-lived cookie.
func setFlash(w http.ResponseWriter, msg string) {
    http.SetCookie(w, &http.Cookie{
        Name:     flashCookieName,
        Value:    msg,
        Path:     "/-/dashboard/",
        MaxAge:   30,
        HttpOnly: true,
        SameSite: http.SameSiteLaxMode,
    })
}

// flash reads and clears the flash cookie from the request.
// Returns empty string if none is set.
func flash(w http.ResponseWriter, r *http.Request) string {
    c, err := r.Cookie(flashCookieName)
    if err != nil {
        return ""
    }
    // Clear immediately so it is truly one-time.
    http.SetCookie(w, &http.Cookie{
        Name:     flashCookieName,
        Path:     "/-/dashboard/",
        MaxAge:   -1,
        HttpOnly: true,
        SameSite: http.SameSiteLaxMode,
    })
    return c.Value
}
```

---

## Task 1: Repo list page — `GET /-/dashboard/`

**Write the test first, then the handler, then the template.**

### 1a — Test (`internal/dashboard/repos_test.go`)

- [ ] Create `internal/dashboard/repos_test.go`.
- [ ] `TestDashboardRepoList_Unauthenticated`: `GET /-/dashboard/` with no session cookie → expect `302` redirect to `/-/auth/login`.
- [ ] `TestDashboardRepoList_Empty`: authenticated user, zero repos in DB → expect `200`, body contains `"No repos yet"`.
- [ ] `TestDashboardRepoList_WithRepos`: authenticated user, seed two repos → expect `200`, body contains both repo names and the status badges (`pending`, `ready`, or `error`).

Test setup pattern:
```go
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
    t.Helper()
    store := db.NewMemoryStore()  // in-memory stub satisfying db.Store
    authn := auth.NewTestAuth(store)
    s := &Server{dbStore: store, authn: authn, tmplFS: assets.FS}
    ts := httptest.NewServer(s.Handler())
    t.Cleanup(ts.Close)
    return s, ts
}
```

Run:
```bash
go test ./internal/dashboard/... -run TestDashboardRepoList -timeout 60s
```
Expected: compile errors (handler/template missing) — that is correct at this point.

### 1b — Handler (`internal/dashboard/repos.go`)

- [ ] Create `internal/dashboard/repos.go`.
- [ ] Define template data struct:
  ```go
  type repoListData struct {
      Flash string
      User  *db.User
      Repos []*db.Repo
  }
  ```
- [ ] Implement `handleRepoList`:
  ```go
  func (s *Server) handleRepoList(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      repos, err := s.dbStore.ListReposByOwner(r.Context(), user.ID)
      if err != nil {
          http.Error(w, "failed to load repos", http.StatusInternalServerError)
          return
      }
      data := repoListData{
          Flash: flash(w, r),
          User:  user,
          Repos: repos,
      }
      s.renderTemplate(w, "dashboard_repos.html", data)
  }
  ```
- [ ] Add `renderTemplate` helper to `server.go` (or a new `helpers.go`) if not already present:
  ```go
  func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
      tmpl, err := template.New("").ParseFS(s.tmplFS,
          "templates/dashboard_base.html",
          "templates/"+name,
      )
      if err != nil {
          http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
          return
      }
      w.Header().Set("Content-Type", "text/html; charset=utf-8")
      if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
          // Headers already sent; log only.
          log.Printf("template execute error: %v", err)
      }
  }
  ```

### 1c — Template (`internal/assets/templates/dashboard_repos.html`)

- [ ] Create `internal/assets/templates/dashboard_repos.html`:
  ```html
  {{template "base" .}}
  {{define "content"}}
  <div class="dashboard-page">
    <div class="dashboard-page-header">
      <h1>Repos</h1>
      <a href="/-/dashboard/repos/new" class="btn btn-primary">Add Repo</a>
    </div>

    {{if .Flash}}
    <div class="flash flash-success">{{.Flash}}</div>
    {{end}}

    {{if .Repos}}
    <table class="dashboard-table">
      <thead>
        <tr>
          <th>Repo</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {{range .Repos}}
        <tr>
          <td>{{.Host}}/{{.RepoOwner}}/{{.RepoName}}</td>
          <td><span class="badge badge-{{.Status}}">{{.Status}}</span></td>
          <td>
            <a href="/-/dashboard/repos/{{.ID}}">Edit</a>
            <form method="post" action="/-/dashboard/repos/{{.ID}}/sync" style="display:inline">
              <button type="submit" class="btn-link">Sync</button>
            </form>
            <form method="post" action="/-/dashboard/repos/{{.ID}}/delete" style="display:inline">
              <button type="submit" class="btn-link btn-danger"
                onclick="return confirm('Delete this repo?')">Delete</button>
            </form>
          </td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}
    <p class="empty-state">No repos yet. <a href="/-/dashboard/repos/new">Add one.</a></p>
    {{end}}
  </div>
  {{end}}
  ```
- [ ] Add CSS for `.badge-pending` (yellow), `.badge-ready` (green), `.badge-error` (red) to `internal/assets/static/style.css`.

### 1d — Verify

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestDashboardRepoList -timeout 60s
  ```
  Expected: all three sub-tests pass.

---

## Task 2: Add repo form — `GET` + `POST /-/dashboard/repos/new`

### 2a — Test

- [ ] Add to `internal/dashboard/repos_test.go`:
  - `TestDashboardRepoNew_GET`: authenticated → `200`, body contains form fields `host`, `owner`, `repo_name`.
  - `TestDashboardRepoNew_POST_Valid`: POST with valid `host`, `owner`, `repo_name` → repo row created in DB, response is `303` redirect to `/-/dashboard/`.
  - `TestDashboardRepoNew_POST_MissingField`: POST with `host` empty → `200` (re-render form), body contains an error message (does not redirect).

Run:
```bash
go test ./internal/dashboard/... -run TestDashboardRepoNew -timeout 60s
```
Expected: compile errors (handlers/template missing).

### 2b — Template data struct (add to `repos.go`)

- [ ] Add:
  ```go
  type repoFormData struct {
      Flash      string
      User       *db.User
      Repo       *db.Repo   // nil for new, populated for edit
      Error      string
      WebhookURL string     // only populated on edit page
  }
  ```

### 2c — GET handler

- [ ] Add `handleRepoNew` to `repos.go`:
  ```go
  func (s *Server) handleRepoNew(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      data := repoFormData{User: user}
      s.renderTemplate(w, "dashboard_repo_form.html", data)
  }
  ```

### 2d — POST handler

- [ ] Add `handleRepoCreate` to `repos.go`:
  ```go
  func (s *Server) handleRepoCreate(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      if err := r.ParseForm(); err != nil {
          http.Error(w, "bad request", http.StatusBadRequest)
          return
      }

      host     := strings.TrimSpace(r.FormValue("host"))
      owner    := strings.TrimSpace(r.FormValue("owner"))
      repoName := strings.TrimSpace(r.FormValue("repo_name"))

      if host == "" || owner == "" || repoName == "" {
          data := repoFormData{
              User:  user,
              Error: "Host, Owner, and Repo Name are required.",
          }
          w.WriteHeader(http.StatusUnprocessableEntity)
          s.renderTemplate(w, "dashboard_repo_form.html", data)
          return
      }

      repo := &db.Repo{
          OwnerID:       user.ID,
          Host:          host,
          RepoOwner:     owner,
          RepoName:      repoName,
          RemoteURL:     strings.TrimSpace(r.FormValue("remote_url")),
          WebhookSecret: strings.TrimSpace(r.FormValue("webhook_secret")),
          TrustedHTML:   r.FormValue("trusted_html") == "on",
          Status:        db.RepoStatusPending,
          CreatedAt:     time.Now(),
      }
      if err := s.dbStore.CreateRepo(r.Context(), repo); err != nil {
          data := repoFormData{User: user, Error: "Could not create repo: " + err.Error()}
          w.WriteHeader(http.StatusInternalServerError)
          s.renderTemplate(w, "dashboard_repo_form.html", data)
          return
      }

      // Trigger background clone via gitstore.
      go func() {
          if err := s.gitStore.AddRepo(repo); err != nil {
              log.Printf("background clone failed for %s/%s/%s: %v", host, owner, repoName, err)
              _ = s.dbStore.UpdateRepoStatus(context.Background(), repo.ID, db.RepoStatusError)
          }
      }()

      setFlash(w, "Repo added — cloning in background.")
      http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
  }
  ```

### 2e — Template (`internal/assets/templates/dashboard_repo_form.html`)

- [ ] Create `internal/assets/templates/dashboard_repo_form.html`:
  ```html
  {{template "base" .}}
  {{define "content"}}
  <div class="dashboard-page">
    <h1>{{if .Repo}}Edit Repo{{else}}Add Repo{{end}}</h1>

    {{if .Flash}}<div class="flash flash-success">{{.Flash}}</div>{{end}}
    {{if .Error}}<div class="flash flash-error">{{.Error}}</div>{{end}}

    <form method="post"
          action="{{if .Repo}}/-/dashboard/repos/{{.Repo.ID}}{{else}}/-/dashboard/repos/new{{end}}">

      <label>Host
        <input type="text" name="host" required
               value="{{if .Repo}}{{.Repo.Host}}{{end}}">
      </label>

      <label>Owner
        <input type="text" name="owner" required
               value="{{if .Repo}}{{.Repo.RepoOwner}}{{end}}">
      </label>

      <label>Repo Name
        <input type="text" name="repo_name" required
               value="{{if .Repo}}{{.Repo.RepoName}}{{end}}">
      </label>

      <label>Remote URL <small>(optional override)</small>
        <input type="url" name="remote_url"
               value="{{if .Repo}}{{.Repo.RemoteURL}}{{end}}">
      </label>

      <label>Webhook Secret <small>(optional)</small>
        <input type="text" name="webhook_secret"
               value="{{if .Repo}}{{.Repo.WebhookSecret}}{{end}}">
      </label>

      <label>
        <input type="checkbox" name="trusted_html"
               {{if .Repo}}{{if .Repo.TrustedHTML}}checked{{end}}{{end}}>
        Trust raw HTML in Markdown
      </label>

      <button type="submit" class="btn btn-primary">
        {{if .Repo}}Save Changes{{else}}Add Repo{{end}}
      </button>
      <a href="/-/dashboard/" class="btn">Cancel</a>
    </form>

    {{if .Repo}}
    <hr>

    {{if .WebhookURL}}
    <section>
      <h2>Webhook URL</h2>
      <code>{{.WebhookURL}}</code>
    </section>
    {{end}}

    <section class="danger-zone">
      <h2>Actions</h2>
      <form method="post" action="/-/dashboard/repos/{{.Repo.ID}}/sync" style="display:inline">
        <button type="submit" class="btn">Sync Now</button>
      </form>
      <form method="post" action="/-/dashboard/repos/{{.Repo.ID}}/delete" style="display:inline">
        <button type="submit" class="btn btn-danger"
          onclick="return confirm('Delete this repo? This cannot be undone.')">Delete</button>
      </form>
    </section>
    {{end}}
  </div>
  {{end}}
  ```

### 2f — Verify

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestDashboardRepoNew -timeout 60s
  ```
  Expected: all three sub-tests pass.

---

## Task 3: Edit, delete, and sync repo — `GET`/`POST /-/dashboard/repos/{id}`, `/delete`, `/sync`

### 3a — Test

- [ ] Add to `internal/dashboard/repos_test.go`:
  - `TestDashboardRepoEdit_GET`: authenticated user owns the repo → `200`, form pre-populated (body contains repo's host/owner/name values), webhook URL visible.
  - `TestDashboardRepoEdit_GET_WrongOwner`: authenticated user does NOT own the repo → `403`.
  - `TestDashboardRepoEdit_POST_Valid`: POST valid updated fields → DB row updated, `303` redirect to `/-/dashboard/`.
  - `TestDashboardRepoDelete`: POST to `/-/dashboard/repos/{id}/delete` → repo removed from DB, `303` redirect to `/-/dashboard/`.
  - `TestDashboardRepoSync`: POST to `/-/dashboard/repos/{id}/sync` → `303` redirect to `/-/dashboard/repos/{id}` with flash message.

Run:
```bash
go test ./internal/dashboard/... -run TestDashboardRepoEdit|TestDashboardRepoDelete|TestDashboardRepoSync -timeout 60s
```
Expected: compile errors (handlers missing).

### 3b — Edit GET handler

- [ ] Add `handleRepoEdit` to `repos.go`:
  ```go
  func (s *Server) handleRepoEdit(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      id   := mustParseID(chi.URLParam(r, "id"))
      repo, err := s.dbStore.GetRepo(r.Context(), id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if repo.OwnerID != user.ID && !user.IsAdmin {
          http.Error(w, "forbidden", http.StatusForbidden)
          return
      }
      webhookURL := fmt.Sprintf("/%s/%s/%s/-/webhook", repo.Host, repo.RepoOwner, repo.RepoName)
      data := repoFormData{
          Flash:      flash(w, r),
          User:       user,
          Repo:       repo,
          WebhookURL: webhookURL,
      }
      s.renderTemplate(w, "dashboard_repo_form.html", data)
  }
  ```
- [ ] Add `mustParseID` helper (returns 0 if parse fails; callers treat 0 as not-found):
  ```go
  func mustParseID(s string) int64 {
      id, _ := strconv.ParseInt(s, 10, 64)
      return id
  }
  ```

### 3c — Edit POST handler

- [ ] Add `handleRepoUpdate` to `repos.go`:
  ```go
  func (s *Server) handleRepoUpdate(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      id   := mustParseID(chi.URLParam(r, "id"))
      repo, err := s.dbStore.GetRepo(r.Context(), id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if repo.OwnerID != user.ID && !user.IsAdmin {
          http.Error(w, "forbidden", http.StatusForbidden)
          return
      }
      if err := r.ParseForm(); err != nil {
          http.Error(w, "bad request", http.StatusBadRequest)
          return
      }

      host     := strings.TrimSpace(r.FormValue("host"))
      owner    := strings.TrimSpace(r.FormValue("owner"))
      repoName := strings.TrimSpace(r.FormValue("repo_name"))
      if host == "" || owner == "" || repoName == "" {
          data := repoFormData{
              User:  user,
              Repo:  repo,
              Error: "Host, Owner, and Repo Name are required.",
          }
          w.WriteHeader(http.StatusUnprocessableEntity)
          s.renderTemplate(w, "dashboard_repo_form.html", data)
          return
      }

      repo.Host          = host
      repo.RepoOwner     = owner
      repo.RepoName      = repoName
      repo.RemoteURL     = strings.TrimSpace(r.FormValue("remote_url"))
      repo.WebhookSecret = strings.TrimSpace(r.FormValue("webhook_secret"))
      repo.TrustedHTML   = r.FormValue("trusted_html") == "on"

      if err := s.dbStore.UpdateRepo(r.Context(), repo); err != nil {
          data := repoFormData{User: user, Repo: repo, Error: "Save failed: " + err.Error()}
          w.WriteHeader(http.StatusInternalServerError)
          s.renderTemplate(w, "dashboard_repo_form.html", data)
          return
      }

      setFlash(w, "Repo updated.")
      http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
  }
  ```

### 3d — Delete POST handler

- [ ] Add `handleRepoDelete` to `repos.go`:
  ```go
  func (s *Server) handleRepoDelete(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      id   := mustParseID(chi.URLParam(r, "id"))
      repo, err := s.dbStore.GetRepo(r.Context(), id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if repo.OwnerID != user.ID && !user.IsAdmin {
          http.Error(w, "forbidden", http.StatusForbidden)
          return
      }
      s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
      if err := s.dbStore.DeleteRepo(r.Context(), id); err != nil {
          http.Error(w, "delete failed", http.StatusInternalServerError)
          return
      }
      setFlash(w, "Repo deleted.")
      http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
  }
  ```

### 3e — Sync POST handler

- [ ] Add `handleRepoSync` to `repos.go`:
  ```go
  func (s *Server) handleRepoSync(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      id   := mustParseID(chi.URLParam(r, "id"))
      repo, err := s.dbStore.GetRepo(r.Context(), id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if repo.OwnerID != user.ID && !user.IsAdmin {
          http.Error(w, "forbidden", http.StatusForbidden)
          return
      }
      go func() {
          if err := s.gitStore.FetchNow(repo.Host, repo.RepoOwner, repo.RepoName); err != nil {
              log.Printf("sync failed for %s/%s/%s: %v", repo.Host, repo.RepoOwner, repo.RepoName, err)
          }
      }()
      setFlash(w, "Sync triggered.")
      http.Redirect(w, r, fmt.Sprintf("/-/dashboard/repos/%d", id), http.StatusSeeOther)
  }
  ```

### 3f — Verify

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run "TestDashboardRepoEdit|TestDashboardRepoDelete|TestDashboardRepoSync" -timeout 60s
  ```
  Expected: all five sub-tests pass.

---

## Task 4: Account settings — `GET` + `POST /-/dashboard/settings`

### 4a — Test (`internal/dashboard/settings_test.go`)

- [ ] Create `internal/dashboard/settings_test.go`.
- [ ] `TestDashboardSettings_GET`: authenticated → `200`, body contains display name and password-change fields.
- [ ] `TestDashboardSettings_POST_UpdateName`: POST `display_name=Alice` → DB user name updated to `"Alice"`, `303` redirect to `/-/dashboard/settings`.
- [ ] `TestDashboardSettings_POST_WrongCurrentPassword`: POST `current_password=wrong` + `new_password=x` → `200` (re-render), body contains error message about incorrect password.
- [ ] `TestDashboardSettings_POST_ChangePassword_Valid`: POST valid `current_password` + `new_password` → password hash updated in DB, `303` redirect.
- [ ] `TestDashboardSettings_UnlinkOAuth`: POST to `/-/dashboard/settings/unlink/github` → OAuth account row removed from DB, `303` redirect to `/-/dashboard/settings`.

Run:
```bash
go test ./internal/dashboard/... -run TestDashboardSettings -timeout 60s
```
Expected: compile errors (handler/template missing).

### 4b — Handler (`internal/dashboard/settings.go`)

- [ ] Create `internal/dashboard/settings.go`.
- [ ] Define template data struct:
  ```go
  type settingsData struct {
      Flash       string
      User        *db.User
      LinkedOAuth []string // provider names already linked, e.g. ["github"]
      Error       string
  }
  ```
- [ ] Implement `handleSettingsGet`:
  ```go
  func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
      user    := auth.UserFromContext(r.Context())
      linked, _ := s.dbStore.ListOAuthProviders(r.Context(), user.ID)
      data := settingsData{
          Flash:       flash(w, r),
          User:        user,
          LinkedOAuth: linked,
      }
      s.renderTemplate(w, "dashboard_settings.html", data)
  }
  ```
- [ ] Implement `handleSettingsPost`:
  ```go
  func (s *Server) handleSettingsPost(w http.ResponseWriter, r *http.Request) {
      user := auth.UserFromContext(r.Context())
      if err := r.ParseForm(); err != nil {
          http.Error(w, "bad request", http.StatusBadRequest)
          return
      }

      // Update display name if provided.
      if name := strings.TrimSpace(r.FormValue("display_name")); name != "" {
          user.Name = name
          if err := s.dbStore.UpdateUser(r.Context(), user); err != nil {
              s.renderSettingsError(w, r, user, "Failed to update name: "+err.Error())
              return
          }
      }

      // Change password if current_password provided.
      if current := r.FormValue("current_password"); current != "" {
          if err := s.authn.CheckPassword(user, current); err != nil {
              s.renderSettingsError(w, r, user, "Current password is incorrect.")
              return
          }
          newPw := r.FormValue("new_password")
          if len(newPw) < 8 {
              s.renderSettingsError(w, r, user, "New password must be at least 8 characters.")
              return
          }
          hash, err := s.authn.HashPassword(newPw)
          if err != nil {
              s.renderSettingsError(w, r, user, "Failed to hash password.")
              return
          }
          user.Password = hash
          if err := s.dbStore.UpdateUser(r.Context(), user); err != nil {
              s.renderSettingsError(w, r, user, "Failed to save password: "+err.Error())
              return
          }
      }

      setFlash(w, "Settings saved.")
      http.Redirect(w, r, "/-/dashboard/settings", http.StatusSeeOther)
  }

  func (s *Server) renderSettingsError(w http.ResponseWriter, r *http.Request, user *db.User, msg string) {
      linked, _ := s.dbStore.ListOAuthProviders(r.Context(), user.ID)
      data := settingsData{User: user, LinkedOAuth: linked, Error: msg}
      w.WriteHeader(http.StatusUnprocessableEntity)
      s.renderTemplate(w, "dashboard_settings.html", data)
  }
  ```
- [ ] Implement `handleOAuthUnlink`:
  ```go
  func (s *Server) handleOAuthUnlink(w http.ResponseWriter, r *http.Request) {
      user     := auth.UserFromContext(r.Context())
      provider := chi.URLParam(r, "provider")
      if err := s.dbStore.DeleteOAuthAccount(r.Context(), user.ID, provider); err != nil {
          setFlash(w, "Failed to unlink: "+err.Error())
      } else {
          setFlash(w, "Unlinked "+provider+".")
      }
      http.Redirect(w, r, "/-/dashboard/settings", http.StatusSeeOther)
  }
  ```

### 4c — Template (`internal/assets/templates/dashboard_settings.html`)

- [ ] Create `internal/assets/templates/dashboard_settings.html`:
  ```html
  {{template "base" .}}
  {{define "content"}}
  <div class="dashboard-page">
    <h1>Account Settings</h1>

    {{if .Flash}}<div class="flash flash-success">{{.Flash}}</div>{{end}}
    {{if .Error}}<div class="flash flash-error">{{.Error}}</div>{{end}}

    <section>
      <h2>Profile</h2>
      <form method="post" action="/-/dashboard/settings">
        <label>Display Name
          <input type="text" name="display_name" value="{{.User.Name}}">
        </label>
        <button type="submit" class="btn btn-primary">Save Name</button>
      </form>
    </section>

    <section>
      <h2>Change Password</h2>
      <form method="post" action="/-/dashboard/settings">
        <label>Current Password
          <input type="password" name="current_password" autocomplete="current-password">
        </label>
        <label>New Password
          <input type="password" name="new_password" autocomplete="new-password">
        </label>
        <button type="submit" class="btn btn-primary">Change Password</button>
      </form>
    </section>

    <section>
      <h2>Linked Accounts</h2>
      {{$linked := .LinkedOAuth}}

      <div class="oauth-row">
        <span>GitHub</span>
        {{if has $linked "github"}}
        <form method="post" action="/-/dashboard/settings/unlink/github" style="display:inline">
          <button type="submit" class="btn">Unlink</button>
        </form>
        {{else}}
        <a href="/-/auth/github" class="btn">Link GitHub</a>
        {{end}}
      </div>

      <div class="oauth-row">
        <span>Google</span>
        {{if has $linked "google"}}
        <form method="post" action="/-/dashboard/settings/unlink/google" style="display:inline">
          <button type="submit" class="btn">Unlink</button>
        </form>
        {{else}}
        <a href="/-/auth/google" class="btn">Link Google</a>
        {{end}}
      </div>
    </section>
  </div>
  {{end}}
  ```
- [ ] Register a `has` template func in `renderTemplate` (or globally in `server.go`):
  ```go
  "has": func(slice []string, item string) bool {
      for _, s := range slice {
          if s == item { return true }
      }
      return false
  },
  ```

### 4d — Verify

- [ ] Run tests:
  ```bash
  go test ./internal/dashboard/... -run TestDashboardSettings -timeout 60s
  ```
  Expected: all five sub-tests pass.

---

## Task 5: Wire routes into `Handler()` with `RequireAuth`

### 5a — Integration test (`internal/dashboard/integration_test.go`)

- [ ] Create `internal/dashboard/integration_test.go`.
- [ ] `TestDashboardRoutes_AllRequireAuth`: table-driven test — for each route below, assert that a request **without** a session cookie gets a `302` redirect to `/-/auth/login` (not `200` or `500`):
  ```
  GET  /-/dashboard/
  GET  /-/dashboard/repos/new
  POST /-/dashboard/repos/new
  GET  /-/dashboard/repos/1
  POST /-/dashboard/repos/1
  POST /-/dashboard/repos/1/delete
  POST /-/dashboard/repos/1/sync
  GET  /-/dashboard/settings
  POST /-/dashboard/settings
  POST /-/dashboard/settings/unlink/github
  ```

Run:
```bash
go test ./internal/dashboard/... -run TestDashboardRoutes_AllRequireAuth -timeout 60s
```
Expected: compile errors (routes not yet wired).

### 5b — Wire routes

- [ ] Open `internal/dashboard/server.go` (or create it if Phase 4 did not produce it) and update `Handler()`:
  ```go
  func (s *Server) Handler() http.Handler {
      r := chi.NewRouter()

      // All /-/dashboard/* routes require authentication.
      r.Route("/-/dashboard", func(r chi.Router) {
          r.Use(s.authn.RequireAuth)

          r.Get("/", s.handleRepoList)

          r.Get("/repos/new",  s.handleRepoNew)
          r.Post("/repos/new", s.handleRepoCreate)

          r.Get("/repos/{id}",         s.handleRepoEdit)
          r.Post("/repos/{id}",        s.handleRepoUpdate)
          r.Post("/repos/{id}/delete", s.handleRepoDelete)
          r.Post("/repos/{id}/sync",   s.handleRepoSync)

          r.Get("/settings",                    s.handleSettingsGet)
          r.Post("/settings",                   s.handleSettingsPost)
          r.Post("/settings/unlink/{provider}", s.handleOAuthUnlink)
      })

      return r
  }
  ```

  Note: if `Handler()` already mounts API routes from Phase 4, add the dashboard route group alongside those existing routes rather than replacing them.

### 5c — Verify

- [ ] Run all dashboard tests:
  ```bash
  go test ./internal/dashboard/... -run TestDashboard -timeout 60s
  ```
  Expected: all tests pass.

- [ ] Run full test suite:
  ```bash
  go test ./... -timeout 60s
  ```
  Expected: no regressions in other packages.

- [ ] Run vet:
  ```bash
  go vet ./internal/dashboard/...
  ```
  Expected: no issues.

---

## Checklist summary

```
Task 1  [ ] repos_test.go (list tests)
        [ ] repos.go (repoListData, handleRepoList, renderTemplate)
        [ ] dashboard_repos.html
        [ ] style.css badge classes

Task 2  [ ] repos_test.go (new-form tests)
        [ ] repos.go (repoFormData, handleRepoNew, handleRepoCreate)
        [ ] dashboard_repo_form.html

Task 3  [ ] repos_test.go (edit/delete/sync tests)
        [ ] repos.go (handleRepoEdit, handleRepoUpdate, handleRepoDelete, handleRepoSync, mustParseID)

Task 4  [ ] settings_test.go
        [ ] settings.go (settingsData, handleSettingsGet, handleSettingsPost, handleOAuthUnlink, renderSettingsError)
        [ ] dashboard_settings.html

Task 5  [ ] integration_test.go (all-routes-require-auth)
        [ ] server.go Handler() wired with RequireAuth group
        [ ] Full suite green
```
