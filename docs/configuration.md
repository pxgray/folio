---
toc: true
---
# Configuration reference

Folio is configured via a [TOML](https://toml.io) file, typically named `folio.toml`.

## `[server]`

| Key    | Type   | Default  | Description                  |
|--------|--------|----------|------------------------------|
| `addr` | string | `:8080`  | TCP address to listen on     |

```toml
[server]
addr = ":8080"
```

## `[cache]`

| Key         | Type     | Default           | Description                                                                  |
|-------------|----------|-------------------|------------------------------------------------------------------------------|
| `dir`       | string   | `~/.cache/folio`  | Directory where bare git clones are stored. `~` is expanded.                 |
| `stale_ttl` | duration | `5m`              | How long a resolved ref is cached before a background fetch is triggered. Set `"0"` to disable TTL-based polling entirely (webhooks only). |

```toml
[cache]
dir       = "~/.cache/folio"
stale_ttl = "5m"
```

### `stale_ttl` behaviour

| Value        | Effect                                                         |
|--------------|----------------------------------------------------------------|
| `"5m"`       | Re-check remote HEAD every 5 minutes in the background        |
| `"0"`        | Never poll; only webhooks invalidate the cache                 |
| `"30s"`      | Aggressive polling — useful for active development setups      |

## `[[repos]]`

Each `[[repos]]` section registers one remote repository.

| Key              | Type   | Required | Description                                                                              |
|------------------|--------|----------|------------------------------------------------------------------------------------------|
| `host`           | string | yes      | Git forge hostname, e.g. `github.com` or `tangled.sh`. Used in the URL: `/{host}/...`  |
| `owner`          | string | yes      | Repository owner / organisation                                                          |
| `repo`           | string | yes      | Repository name                                                                          |
| `remote`         | string | no       | Clone URL. Defaults to `https://{host}/{owner}/{repo}.git`                              |
| `webhook_secret` | string | no       | HMAC secret for webhook signature verification. Empty = no verification.                |
| `trusted_html`   | bool   | no       | When `true`, raw HTML in Markdown passes through without sanitization.                   |

### `[repos.web_artifacts]`

Each repo can serve standard root web artifacts (`llms.txt`, `llms-full.txt`, `robots.txt`, `sitemap.xml`). The `web_artifacts` table maps artifact filenames to their git paths within the repo.

| Key              | Type   | Description                                                                              |
|------------------|--------|------------------------------------------------------------------------------------------|
| `llms.txt`       | string | Git path for the `llms.txt` artifact. Empty string = disabled.                           |
| `llms-full.txt`  | string | Git path for the `llms-full.txt` artifact. Empty string = disabled.                      |
| `robots.txt`     | string | Git path for the `robots.txt` artifact. Empty string = disabled.                         |
| `sitemap.xml`    | string | Git path for the `sitemap.xml` artifact. Empty string = disabled.                        |

If an artifact key is absent from `web_artifacts`, Folio falls back to looking for a file with the same name at the repo root. An empty string explicitly disables the artifact (returns 404).

```toml
[[repos]]
host           = "github.com"
owner          = "pxgray"
repo           = "folio"
webhook_secret = ""

[repos.web_artifacts]
"llms.txt"      = "docs/ai/llms.txt"
"llms-full.txt" = "docs/ai/full.txt"
"robots.txt"    = ""
# sitemap.xml not configured → falls back to sitemap.xml in repo root

[[repos]]
host           = "tangled.sh"
owner          = "alice"
repo           = "notes"
remote         = "https://tangled.sh/alice/notes.git"
webhook_secret = "hunter2"
# no web_artifacts → all artifacts fall back to repo root filenames
```

## `[root_artifacts]`

The root site (`/`) can serve web artifacts from disk or inline content. This is independent of any configured repo — it does not fall back to repo content.

| Key         | Type              | Description                                                                              |
|-------------|-------------------|------------------------------------------------------------------------------------------|
| `dir`       | string            | Directory on disk containing artifact files.                                             |
| `files`     | table (string)    | Inline content mapping: artifact filename → content string.                              |

Inline `files` entries take precedence over `dir` for any given filename.

```toml
[root_artifacts]
dir = "/etc/folio/root-artifacts"

[root_artifacts.files]
"robots.txt" = "User-agent: *\nDisallow: /"
```

## Raw file serving

The `/-/raw/` route serves only files with safe extensions:

| Category   | Extensions                                                                 |
|------------|----------------------------------------------------------------------------|
| Images     | `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`, `.ico`, `.bmp`, `.tiff`, `.avif` |
| Fonts      | `.woff`, `.woff2`, `.ttf`, `.eot`, `.otf`                                  |
| Stylesheets| `.css`                                                                     |
| Documents  | `.pdf`                                                                     |
| Data files | `.json`, `.xml`, `.yaml`, `.yml`, `.csv`, `.tsv`                           |
| Media      | `.mp4`, `.webm`, `.ogg`, `.mp3`, `.wav`                                    |

Paths containing `..` or starting with `.` or `_` (at any path segment) are blocked. Responses are capped at 10 MB.

## Full example

```toml
[server]
addr = ":8080"

[cache]
dir       = "~/.cache/folio"
stale_ttl = "5m"

[root_artifacts]
dir = "/etc/folio/root-artifacts"
[root_artifacts.files]
"robots.txt" = "User-agent: *\nDisallow: /"

[[repos]]
host           = "github.com"
owner          = "pxgray"
repo           = "folio"
webhook_secret = "my-github-secret"

[repos.web_artifacts]
"llms.txt"      = "docs/ai/llms.txt"
"llms-full.txt" = "docs/ai/full.txt"

[[repos]]
host  = "github.com"
owner = "golang"
repo  = "go"
```
