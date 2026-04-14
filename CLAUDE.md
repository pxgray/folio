# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

Uses [Task](https://taskfile.dev) (not Make). All commands use `task`. On this development system the binary may be installed as `go-task` rather than `task` — try `go-task` if `task` is not found, before falling back to raw `go` commands.

```bash
task build        # Compile binary to bin/folio
task run          # Build and run (usage: task run -- serve)
task test         # Run all tests (60s timeout)
task test:watch   # Watch mode (requires entr)
task fmt          # gofmt all source files
task vet          # go vet static analysis
task check        # fmt + vet + test (run before committing)
task clean        # Remove build artifacts
task tidy         # go mod tidy
```

Run a single test package: `go test ./internal/render/...`
Run a single test: `go test -run TestLinkRewriter ./internal/render/`

## Architecture

**Folio** serves Markdown files directly from bare-cloned git repositories without a working tree. The flow: HTTP request → resolve git ref → read blob from object store → render Markdown → serve HTML.

### Package Structure

- **`internal/config`** — TOML config loading (legacy; used only for initial dev/test setups). The production server reads all settings from the database.
- **`internal/db`** — SQLite persistence via `db.Store` interface. Holds users, sessions, repos, OAuth accounts, and key/value settings. Implementation in `sqlite.go`.
- **`internal/auth`** — Session management, bcrypt password hashing, OAuth (GitHub, Google) client helpers. `middleware.go` provides `RequireAuth` and `RequireAdmin` chi middlewares.
- **`internal/dashboard`** — Multi-user web dashboard (chi router). Phases 3-6: setup wizard, login/OAuth, repo CRUD, user settings, admin panel. REST API under `/-/api/v1/`.
- **`internal/gitstore`** — Core git layer. `Store` manages a map of bare clones; `Repo` wraps a go-git repository with in-memory ref caching and stale-while-revalidate semantics. `ResolveRef` caches commit hashes; if older than `stale_ttl`, returns cached value and triggers a background fetch. `FetchNow` is called by webhooks to force immediate fetch + cache invalidation.
- **`internal/web`** — Chi router for doc serving. Routes are `/{host}/{owner}/{repo}[/*]` for docs, `/{host}/{owner}/{repo}/-/webhook` for push notifications, `/{host}/{owner}/{repo}/-/raw/*` for non-Markdown files. Reads per-repo config (trusted HTML, webhook secrets) from `db.Store`. Handler logic is in `doc.go`, `raw.go`, `webhook.go`.
- **`internal/render`** — goldmark-based Markdown → HTML. `linkrewrite.go` is a goldmark AST transformer that rewrites relative links to Folio-internal URLs: `.md` links go to the doc handler; other relative paths go to `/-/raw/`; absolute URLs and fragments are left alone.
- **`internal/nav`** — Reads `folio.yml` from repo root for explicit nav, or auto-generates a tree by walking all `.md` files (skipping entries starting with `.` or `_`).
- **`internal/assets`** — `//go:embed` for HTML templates and `style.css`. Templates use `text/template`.

### URL Structure

Doc serving (handled by `internal/web`):

```
/                                           → repo index
/{host}/{owner}/{repo}                      → repo root (redirects to first .md)
/{host}/{owner}/{repo}/{path}               → document or directory
/{host}/{owner}/{repo}/-/raw/{path}         → raw file download
/{host}/{owner}/{repo}/-/webhook            → GitHub push webhook (POST)
/-/static/*                                 → embedded CSS/assets
```

Dashboard and auth (handled by `internal/dashboard`):

```
/-/setup                                    → first-run setup wizard
/-/auth/login                               → login page
/-/auth/github[/callback]                   → GitHub OAuth
/-/auth/google[/callback]                   → Google OAuth
/-/dashboard/                               → repo list (auth required)
/-/dashboard/repos/new                      → add repo
/-/dashboard/repos/{id}                     → edit repo
/-/dashboard/settings                       → user profile/password/OAuth links
/-/dashboard/admin/                         → admin: user list
/-/dashboard/admin/users/{id}               → admin: edit user
/-/dashboard/admin/settings                 → admin: system settings
/-/api/v1/...                               → REST API (JSON)
```

All doc/raw routes accept `?ref=<branch-or-sha>` for historical views.

### Caching Model

- Ref resolution (branch → commit hash) is cached in memory per `Repo`.
- `stale_ttl = 0`: cache forever, update only via webhook.
- `stale_ttl = 5m` (default): background fetch triggered when cache is stale; stale value is still returned immediately.
- Webhooks bypass TTL: they call `FetchNow()` then clear the ref cache.

## Frontend Philosophy

Prefer no JS. If JS is truly necessary, write it inline in the templates — no build step, no bundler, no npm. Keep the total page weight minimal; the current site ships zero JavaScript.

### Key Dependency Choices

- **go-git/v5** — pure-Go git, reads directly from the object store without spawning `git`.
- **goldmark** — extensible Markdown parser; GFM extensions enabled (tables, strikethrough, task lists).
- **chi** — lightweight HTTP router used for URL parameter extraction.
- **BurntSushi/toml** — legacy config parsing; **yaml.v3** — `folio.yml` nav config.
- **mattn/go-sqlite3** — CGo SQLite driver backing `db.Store`.
- **golang.org/x/oauth2** — OAuth2 client for GitHub and Google login.
