# Folio

A self-hosted, multi-user documentation server that renders Markdown from git repositories, inspired by Google's internal g3doc.

**[→ Read the docs](docs/index.md)**

## Quick start

```sh
go install github.com/pxgray/folio/cmd/folio@latest
folio serve
```

Open `http://localhost:8080`. You'll be redirected to the setup wizard to create your admin account and configure the server.

## Features

- **Multi-user** — user accounts with email/password login and OAuth (GitHub, Google)
- **Web dashboard** — add and manage repos through a browser UI; no config files to edit
- **Git-native** — reads directly from bare clones, no working tree needed
- **Repo-as-site** — each repo acts as its own site; the root redirects to the first Markdown file
- **Webhook-driven freshness** — push to GitHub and Folio updates immediately
- **TTL fallback** — optional background polling for repos without webhooks
- **Historical views** — `?ref=<sha-or-branch>` on any URL
- **Secure raw file serving** — extension allowlist, blocked prefixes, 10 MB cap

## URL structure

Doc and raw routes:

```
/{host}/{owner}/{repo}[/{path}][?ref=<ref>]
/{host}/{owner}/{repo}/-/raw/{path}[?ref=<ref>]
/{host}/{owner}/{repo}/-/webhook        (POST — push webhook)
```

Dashboard and auth (all under `/-/`):

```
/-/setup                                 first-run setup wizard
/-/auth/login                            login page
/-/auth/github                           GitHub OAuth
/-/auth/google                           Google OAuth
/-/dashboard/                            repo list
/-/dashboard/repos/new                   add a repo
/-/dashboard/repos/{id}                  edit a repo
/-/dashboard/settings                    user settings
/-/dashboard/admin/                      admin: user list
/-/dashboard/admin/settings              admin: system settings
```

REST API:

```
/-/api/v1/auth/login                     POST
/-/api/v1/auth/logout                    POST
/-/api/v1/auth/me                        GET
/-/api/v1/repos                          GET, POST
/-/api/v1/repos/{id}                     GET, PATCH, DELETE
/-/api/v1/repos/{id}/sync                POST
/-/api/v1/admin/users                    GET (admin)
/-/api/v1/admin/users/{id}               PATCH, DELETE (admin)
/-/api/v1/admin/settings                 GET, PATCH (admin)
```

## How it works

1. On first run, Folio redirects to `/-/setup` to create the admin account
2. Repos are added through `/-/dashboard/` — each is bare-cloned into the cache directory
3. Each doc request reads the file directly from the git object store — no filesystem checkout
4. Markdown is rendered with [goldmark](https://github.com/yuin/goldmark) (GFM-compatible)
5. Relative links are rewritten to internal Folio URLs at render time
6. When a webhook fires, Folio immediately fetches the latest commits and clears its ref cache

## License

MIT
