---
toc: true
---
# Folio

Folio is a self-hosted, multi-user documentation server that renders Markdown files directly from git repositories. Each registered repository acts as its own site, with automatic redirect to the first Markdown file.

## Features

- **Multi-user**: user accounts with email/password login and OAuth (GitHub, Google); admin and regular roles
- **Web dashboard**: add and manage repos through `/-/dashboard/` — no config files to edit
- **Git-native**: reads files directly from bare clones — no working tree, no checkout
- **Repo-as-site**: each repo acts as its own site; visiting the repo root redirects to the first Markdown file
- **Repo-relative links**: links like `../api/reference.md` are automatically rewritten to internal Folio URLs
- **Webhook-driven freshness**: push to GitHub and Folio updates immediately
- **TTL fallback**: optional background polling for repos without webhooks
- **Historical views**: add `?ref=<sha-or-branch>` to any URL to view any past state
- **Secure raw file serving**: extension allowlist, blocked prefixes, and size caps

## Quick navigation

- [Getting started](getting-started.md)
- [Configuration reference](configuration.md)
- [Webhook setup](webhooks.md)

## URL structure

```
/{host}/{owner}/{repo}[/{path}][?ref=<ref>]
```

Examples:

```
/github.com/pxgray/folio/docs/index.md
/github.com/pxgray/folio/docs/index.md?ref=v0.1.0
/tangled.sh/alice/notes/README.md
```

Visiting a repo root (`/{host}/{owner}/{repo}`) redirects to the first Markdown file in the navigation tree.

Raw (non-Markdown) files:

```
/{host}/{owner}/{repo}/-/raw/{path}[?ref=<ref>]
```

Web artifacts (per repo, served from the repo root):

```
/{host}/{owner}/{repo}/llms.txt
/{host}/{owner}/{repo}/llms-full.txt
/{host}/{owner}/{repo}/robots.txt
/{host}/{owner}/{repo}/sitemap.xml
```

## How it works

1. On first run, Folio redirects to `/-/setup` to create the first admin account
2. Repos are added through the dashboard — Folio bare-clones them into the cache directory
3. Each request reads the file directly from the git object store — no filesystem checkout needed
4. Markdown is rendered with [goldmark](https://github.com/yuin/goldmark) (GFM-compatible)
5. Relative links are rewritten to internal Folio URLs at render time
6. When a webhook fires, Folio immediately fetches the latest commits and clears its ref cache
