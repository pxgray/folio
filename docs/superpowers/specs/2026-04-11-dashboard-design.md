# Repo & Configuration Management Dashboard

**Date:** 2026-04-11
**Status:** Approved

## Overview

Add a multi-user management dashboard to Folio. Users can log in, add and remove repos, and configure those repos. An admin role can manage all users and change server-level settings. This is the foundation for Folio as a hosted service.

The existing `folio.toml` config format is retired. All configuration lives in a SQLite database. The server enters a setup wizard on first run to bootstrap the initial admin account and server settings.

---

## Architecture

The existing `folio` binary gains three new internal packages and a database file:

```
internal/db/        — database layer (SQLite via modernc.org/sqlite, pure Go, no CGO)
internal/auth/      — session management, password hashing, OAuth flows
internal/dashboard/ — HTTP handlers for /-/dashboard/ and /-/api/v1/
```

**Route namespaces:**

```
/-/setup/*          — first-run wizard (blocked once setup is complete)
/-/auth/*           — login, logout, OAuth callbacks
/-/dashboard/*      — SSR management UI (requires auth)
/-/api/v1/*         — JSON REST API (requires auth)
/-/static/*         — existing static assets (unchanged)
```

All existing doc/raw/webhook routes are unchanged.

**Bootstrap:** `folio serve [--db folio.db]`. If no DB file exists, the server opens in setup-wizard mode at `/-/setup`. The `--db` flag (or `FOLIO_DB` env var) is the only bootstrap config — everything else lives in the DB.

**Live repo management:** `gitstore.Store` gains `AddRepo` / `RemoveRepo` methods so repos can be added and removed at runtime without restarting the process. The database is the authoritative source; the store is a runtime mirror of it.

---

## Database Schema

Six tables. The `internal/db` package exposes typed Go methods (`CreateUser`, `GetReposByOwner`, `UpsertSetting`, etc.) behind a `Store` interface so Postgres can be swapped in later.

**`users`**
```sql
id          INTEGER PRIMARY KEY
email       TEXT UNIQUE NOT NULL
name        TEXT NOT NULL
password    TEXT          -- bcrypt hash; NULL for OAuth-only accounts
is_admin    BOOLEAN NOT NULL DEFAULT FALSE
created_at  DATETIME NOT NULL
```

**`oauth_accounts`**
```sql
id          INTEGER PRIMARY KEY
user_id     INTEGER NOT NULL REFERENCES users(id)
provider    TEXT NOT NULL   -- "github", "google"
provider_id TEXT NOT NULL   -- provider's user ID
UNIQUE(provider, provider_id)
```

**`sessions`**
```sql
token       TEXT PRIMARY KEY  -- random 32-byte hex, stored in cookie
user_id     INTEGER NOT NULL REFERENCES users(id)
expires_at  DATETIME NOT NULL
created_at  DATETIME NOT NULL
```

**`repos`**
```sql
id              INTEGER PRIMARY KEY
owner_id        INTEGER NOT NULL REFERENCES users(id)
host            TEXT NOT NULL
repo_owner      TEXT NOT NULL
repo_name       TEXT NOT NULL
remote_url      TEXT          -- override; NULL means infer from host/owner/repo
webhook_secret  TEXT
trusted_html    BOOLEAN NOT NULL DEFAULT FALSE
stale_ttl_secs  INTEGER       -- NULL means inherit server default
status          TEXT NOT NULL DEFAULT 'pending_clone'  -- pending_clone | ready | error
created_at      DATETIME NOT NULL
UNIQUE(host, repo_owner, repo_name)
```

**`server_settings`**
```sql
key    TEXT PRIMARY KEY
value  TEXT NOT NULL
```
Keys: `addr`, `cache_dir`, `stale_ttl`, `base_url`, `setup_complete`, `oauth_github_client_id`, `oauth_github_client_secret`, `oauth_google_client_id`, `oauth_google_client_secret`.

**`repo_web_artifacts`**
```sql
id       INTEGER PRIMARY KEY
repo_id  INTEGER NOT NULL REFERENCES repos(id)
name     TEXT NOT NULL
path     TEXT NOT NULL
UNIQUE(repo_id, name)
```

**Schema notes:**
- `owner_id` on `repos` is flat user ownership. The schema is designed to accommodate a future `org_id` column without restructuring.
- `server_settings` is a key/value table rather than typed columns so new settings can be added without schema migrations.

---

## Auth

**Sessions** are server-side cookie sessions. On login a 32-byte random token is generated, stored in `sessions`, and set as an `HttpOnly; Secure; SameSite=Lax` cookie. Expiry defaults to 30 days, sliding on activity. Logout deletes the row. Expired sessions are lazily deleted on access; a background goroutine runs every 24 hours to purge any remaining stale rows.

**Email/password** uses bcrypt (cost 12). Password reset is out of scope for the initial build — admin can set passwords directly via the admin panel.

**OAuth flow** (GitHub primary, Google secondary):
1. User clicks "Sign in with GitHub" → redirect to provider with a random `state` stored in a short-lived cookie.
2. Provider redirects to `/-/auth/{provider}/callback` with code + state.
3. Server exchanges code for token, fetches user profile, matches on `oauth_accounts.provider_id`.
4. Match found → log in. No match → create `users` row + `oauth_accounts` row (first OAuth sign-in auto-registers).
5. Redirect to `/-/dashboard/`.

**Middleware:**

- `RequireAuth` — checks session cookie, loads user from DB, injects into request context. Returns 401 (API) or redirect to `/-/auth/login` (dashboard) if missing/expired.
- `RequireAdmin` — wraps `RequireAuth`; additionally checks `user.is_admin`. Returns 403 if not admin.

**Setup wizard** at `/-/setup` is exempt from auth. It checks on each request whether setup is complete (the `setup_complete` key in `server_settings`); if yes, it redirects to `/`. The wizard collects: server addr, cache dir, first admin email/name/password. OAuth credentials are configured post-setup in the admin panel.

---

## API Layer (`/-/api/v1/`)

All endpoints return JSON. Auth errors return `401` or `403`. Validation errors return `422` with body `{"error": "..."}`.

**Auth**
```
POST   /-/api/v1/auth/login       — email + password → set session cookie
POST   /-/api/v1/auth/logout      — clear session cookie
GET    /-/api/v1/auth/me          — current user info (JSON)
```

OAuth flows are browser redirects and live under `/-/auth/`, not the JSON API prefix:
```
GET    /-/auth/github             — redirect to GitHub
GET    /-/auth/github/callback    — GitHub redirect back
GET    /-/auth/google             — redirect to Google
GET    /-/auth/google/callback    — Google redirect back
```

**Repos** (scoped to authenticated user)
```
GET    /-/api/v1/repos
POST   /-/api/v1/repos
GET    /-/api/v1/repos/{id}
PATCH  /-/api/v1/repos/{id}
DELETE /-/api/v1/repos/{id}
POST   /-/api/v1/repos/{id}/sync
```

`POST /repos` body:
```json
{
  "host": "github.com",
  "owner": "acme",
  "repo": "docs",
  "remote_url": "",
  "webhook_secret": "",
  "trusted_html": false
}
```

`POST /repos` returns `202 Accepted` immediately; clone happens in the background. The `status` field on the repo record tracks progress: `pending_clone` → `ready` (or `error`).

**Admin — users** (admin only)
```
GET    /-/api/v1/admin/users
PATCH  /-/api/v1/admin/users/{id}
DELETE /-/api/v1/admin/users/{id}
```

`DELETE /admin/users/{id}` cascades: all repos owned by that user are removed from the DB and from the live `gitstore`. An admin cannot delete their own account.

**Admin — server settings** (admin only)
```
GET    /-/api/v1/admin/settings
PATCH  /-/api/v1/admin/settings
```

---

## Dashboard UI (`/-/dashboard/`)

Server-rendered HTML using the existing template system. All forms POST to `/-/api/v1/` endpoints. Inline JS used only where unavoidable (delete confirmation dialogs). No build step, no bundler, no npm.

**Pages:**

| Route | Description |
|---|---|
| `/-/dashboard/` | Repo list: table with name, host, status badge, edit/sync/delete actions |
| `/-/dashboard/repos/new` | Add repo form: host, owner/repo, remote URL override, webhook secret, trusted HTML toggle |
| `/-/dashboard/repos/{id}` | Edit repo: same fields pre-populated; shows copyable webhook URL; Sync Now and Delete buttons |
| `/-/dashboard/settings` | Account settings: display name, change password, linked OAuth accounts (link/unlink) |
| `/-/dashboard/admin/` | Admin user list: all users, role badges, promote/demote, delete (admin only) |
| `/-/dashboard/admin/settings` | Admin server settings: addr, cache dir, stale TTL, OAuth credentials, base URL (admin only) |

**Layout:** The dashboard uses the existing `base.html` shell with a new `dashboard-sidebar` nav (Repos, Settings; plus an Admin section for admins). The doc-viewer topnav and TOC are not shown in dashboard pages.

**Flash messages:** A `_flash` cookie carries one-time success/error messages across POST→redirect cycles. The base template renders a dismissable banner if the cookie is present.

**Settings that require restart** (addr, cache dir) show a warning banner in the admin settings form.

---

## Out of Scope (Initial Build)

- Password reset / forgot-password flow
- Email verification
- Per-repo access control (repo is visible to owner only for now)
- Organization/team model (schema is ready; feature is not)
- Audit log
- Billing / usage limits
