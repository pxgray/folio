# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

Uses [Task](https://taskfile.dev) (not Make). All commands use `task`:

```bash
task build        # Compile binary to bin/folio
task run          # Build and run (usage: task run -- folio.toml)
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

- **`internal/config`** — TOML config loading (`folio.toml`). Expands `~` in paths, validates required fields.
- **`internal/gitstore`** — Core git layer. `Store` manages a map of bare clones; `Repo` wraps a go-git repository with in-memory ref caching and stale-while-revalidate semantics. `ResolveRef` caches commit hashes; if older than `stale_ttl`, returns cached value and triggers a background fetch. `FetchNow` is called by webhooks to force immediate fetch + cache invalidation.
- **`internal/web`** — Chi router. Routes are `/{host}/{owner}/{repo}[/*]` for docs, `/-/webhook` for push notifications, `/-/raw/*` for non-Markdown files. Handler logic is in `doc.go`, `raw.go`, `webhook.go`.
- **`internal/render`** — goldmark-based Markdown → HTML. `linkrewrite.go` is a goldmark AST transformer that rewrites relative links to Folio-internal URLs: `.md` links go to the doc handler; other relative paths go to `/-/raw/`; absolute URLs and fragments are left alone.
- **`internal/nav`** — Reads `folio.yml` from repo root for explicit nav, or auto-generates a tree by walking all `.md` files (skipping entries starting with `.` or `_`).
- **`internal/assets`** — `//go:embed` for HTML templates and `style.css`. Templates use `text/template`.

### URL Structure

```
/                                       → repo index
/{host}/{owner}/{repo}                  → repo root
/{host}/{owner}/{repo}/{path}           → document or directory
/{host}/{owner}/{repo}/-/raw/{path}     → raw file download
/{host}/{owner}/{repo}/-/webhook        → GitHub push webhook (POST)
/-/static/*                             → embedded CSS/assets
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
- **BurntSushi/toml** — config parsing; **yaml.v3** — `folio.yml` nav config.
