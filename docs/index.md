---
toc: true
---
# Folio

Folio is a lightweight documentation server that renders Markdown files directly from public git repositories. It mirrors the directory structure of your codebase, rewrites repo-relative links, and keeps content fresh via webhooks.

## Features

- **Git-native**: Reads files directly from bare clones — no working tree, no checkout
- **All Markdown**: Serves every `.md` file in a repo at a clean URL
- **Repo-relative links**: Links like `../api/reference.md` are automatically rewritten to internal Folio URLs
- **Webhook-driven freshness**: Push to GitHub or tangled.sh and Folio updates immediately
- **TTL fallback**: Optional background polling for repos without webhooks
- **Historical views**: Add `?ref=<sha-or-branch>` to any URL to view any past state

## Quick navigation

- [Getting started](getting-started.md)
- [Configuration reference](configuration.md)
- [Webhook setup](webhooks.md)

## URL structure

```
/{host}/{owner}/{repo}/{path}[?ref=<ref>]
```

Examples:

```
/github.com/pxgray/folio/docs/index.md
/github.com/pxgray/folio/docs/index.md?ref=v0.1.0
/tangled.sh/alice/notes/README.md
```

Raw (non-Markdown) files:

```
/{host}/{owner}/{repo}/-/raw/{path}[?ref=<ref>]
```

## How it works

1. On startup, Folio bare-clones each configured repo into `~/.cache/folio/`
2. Each request reads the file directly from the git object store — no filesystem checkout needed
3. Markdown is rendered with [goldmark](https://github.com/yuin/goldmark) (GFM-compatible)
4. Relative links are rewritten to internal Folio URLs at render time
5. When a webhook fires, Folio immediately fetches the latest commits and clears its ref cache
