# Folio

A lightweight documentation server that renders Markdown from public git repositories, inspired by Google's internal g3doc.

**[→ Read the docs](docs/index.md)**

## Quick start

```sh
go install github.com/pxgray/folio/cmd/folio@latest
```

Create `folio.toml`:

```toml
[server]
addr = ":8080"

[cache]
dir = "~/.cache/folio"

[[repos]]
host  = "github.com"
owner = "your-username"
repo  = "your-repo"
```

Run:

```sh
folio folio.toml
```

Open `http://localhost:8080`.

## Features

- Bare-clone backed — reads directly from git object store, no working tree needed
- Repo-as-site model — each configured repo acts as its own site
- Auto-redirects repo root to the first Markdown file in navigation
- Configurable root web artifacts (llms.txt, robots.txt, sitemap.xml) per repo and for the root site
- `?ref=<sha-or-branch>` for historical views
- Webhook-driven instant updates (GitHub, tangled.sh)
- TTL fallback for repos without webhooks
- Extension allowlist for raw file serving

## Configuration

### Web artifacts

Each repo can serve standard root web artifacts. Configure the git path for each artifact:

```toml
[[repos]]
host  = "github.com"
owner = "myorg"
repo  = "myproject"

[repos.web_artifacts]
"llms.txt"      = "docs/ai/llms.txt"
"llms-full.txt" = "docs/ai/full.txt"
"robots.txt"    = ""                    # explicitly disabled
# sitemap.xml not configured → falls back to sitemap.xml in repo root
```

If an artifact key is absent, Folio looks for a file with the same name at the repo root. An empty string disables the artifact entirely.

### Root site artifacts

The root site (`/`) can serve artifacts from disk or inline content:

```toml
[root_artifacts]
dir = "/etc/folio/root-artifacts"
[root_artifacts.files]
"robots.txt" = "User-agent: *\nDisallow: /"
```

Inline `files` entries take precedence over `dir`.

### Raw file serving

The `/-/raw/` route serves only files with safe extensions: images (`.png`, `.jpg`, `.gif`, `.webp`, `.svg`), fonts (`.woff`, `.woff2`), stylesheets (`.css`), documents (`.pdf`), data files (`.json`, `.xml`, `.yaml`, `.csv`), and media (`.mp4`, `.webm`, `.ogg`, `.mp3`, `.wav`). Paths starting with `.` or `_` are blocked. Responses are capped at 10 MB.

## URL structure

```
/{host}/{owner}/{repo}[/{path}][?ref=<ref>]
/{host}/{owner}/{repo}/-/raw/{path}[?ref=<ref>]
/{host}/{owner}/{repo}/llms.txt
/{host}/{owner}/{repo}/robots.txt
/{host}/{owner}/{repo}/sitemap.xml
/{host}/{owner}/{repo}/llms-full.txt
```

Root artifacts:

```
/llms.txt
/robots.txt
/sitemap.xml
/llms-full.txt
```

## How it works

1. On startup, Folio bare-clones each configured repo into `~/.cache/folio/`
2. Each request reads the file directly from the git object store — no filesystem checkout needed
3. Markdown is rendered with [goldmark](https://github.com/yuin/goldmark) (GFM-compatible)
4. Relative links are rewritten to internal Folio URLs at render time
5. When a webhook fires, Folio immediately fetches the latest commits and clears its ref cache

## License

MIT
