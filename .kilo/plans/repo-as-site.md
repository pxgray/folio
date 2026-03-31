# Implementation Plan: Repo-as-Site with Configurable Root Artifacts

## Overview

This plan restructures Folio so that each configured repository acts as its own site with configurable root web artifacts (llms.txt, llms-full.txt, robots.txt, sitemap.xml). It removes the public file tree listing and removes arbitrary static file serving from repos, while keeping the raw handler for internal use (e.g., images in markdown).

## Changes by File

### 1. `internal/config/config.go` — Extend RepoConfig

Add a `WebArtifacts` map to `RepoConfig` and a new `RootArtifacts` section to `Config`:

```go
type RepoConfig struct {
    Host          string            `toml:"host"`
    Owner         string            `toml:"owner"`
    Repo          string            `toml:"repo"`
    Remote        string            `toml:"remote"`
    WebhookSecret string            `toml:"webhook_secret"`
    TrustedHTML   bool              `toml:"trusted_html"`
    WebArtifacts  map[string]string `toml:"web_artifacts"` // e.g. {"llms.txt": "llms.txt", "robots.txt": "robots.txt"}
}
```

`WebArtifacts` maps artifact filename (e.g. `llms.txt`) to the path in the git repo where that file lives. If a key is present but the value is empty string, it means "serve this artifact as 404 / not available". If the key is absent entirely, fall back to checking the repo root for the file with the same name.

Add a new top-level section:

```go
type RootArtifactsConfig struct {
    Dir string              `toml:"dir"` // directory on disk containing artifact files
    Files map[string]string `toml:"files"` // explicit filename -> content mapping (inline)
}

type Config struct {
    // ... existing fields
    RootArtifacts RootArtifactsConfig `toml:"root_artifacts"`
}
```

- `RootArtifacts.Dir`: If set, serve root artifacts from files in this directory on disk.
- `RootArtifacts.Files`: If set, serve inline content directly from config.
- `Files` takes precedence over `Dir` for any given filename.

### 2. `internal/web/server.go` — New Routes and Server Fields

**New Server fields:**
```go
rootArtifactDir string              // disk directory for root artifacts
rootArtifactFiles map[string]string // inline content map
```

**New routes (add before the catch-all repo routes):**
```
GET /llms.txt
GET /llms-full.txt
GET /robots.txt
GET /sitemap.xml
GET /{host}/{owner}/{repo}/llms.txt
GET /{host}/{owner}/{repo}/llms-full.txt
GET /{host}/{owner}/{repo}/robots.txt
GET /{host}/{owner}/{repo}/sitemap.xml
```

These routes are registered explicitly for the four artifact filenames. They intercept before `handleDoc` catches them as document paths.

**Route registration approach:**
Instead of hardcoding four filenames, define a constant slice `rootArtifactNames = []string{"llms.txt", "llms-full.txt", "robots.txt", "sitemap.xml"}` and loop over it to register both root-level and per-repo routes.

### 3. `internal/web/artifacts.go` — New File (Artifact Handlers)

Create a new file with handlers for web artifacts:

**`handleRootArtifact(w, r)`** — Serves root-level artifacts:
1. Determine requested filename from the URL path.
2. Check `rootArtifactFiles` map first (inline config). If found, serve content with appropriate content type.
3. If not found, check `rootArtifactDir`. If set, read the file from disk and serve.
4. If neither, return 404.

**`handleRepoArtifact(w, r)`** — Serves per-repo artifacts:
1. Extract host/owner/repo and artifact filename from URL.
2. Look up the repo config to find the `WebArtifacts` mapping.
3. Determine the git path:
   - If `WebArtifacts[filename]` is explicitly set to a non-empty string, use that path.
   - If `WebArtifacts[filename]` is explicitly set to empty string, return 404 (disabled).
   - If `WebArtifacts[filename]` key is absent, fall back to using the filename itself as the git path.
4. Resolve ref (default branch), read blob from git, serve with appropriate content type.
5. Content type detection: `.txt` → `text/plain`, `.xml` → `text/xml`, otherwise detect.

### 4. `internal/web/doc.go` — Repo Root Redirect + Remove Directory Listing

**New behavior: redirect repo root to first nav item**

In `serveRepo`, when `filePath == ""` (visitor landed on repo root):
1. Load nav items via `loadNav`.
2. Find the first leaf nav item (recursively descend into section headers until a leaf with a non-empty `Path` is found).
3. If a leaf is found, redirect to it with `http.StatusFound` (302), preserving `?ref=` if present.
4. If no nav items exist (no folio.yml, no .md files), fall through to the existing behavior: try to serve the repo root as a markdown file or return 404.

**Helper function `firstNavLeaf(items []nav.Item) string`:**
Recursively finds the first leaf with a non-empty `Path`. Returns the path string or empty string if none found.

**Modify `dispatchToContent`:**
- When `ReadBlob` returns `ErrNotFound` and `ReadTree` succeeds (i.e., it's a directory), return 404 instead of calling `serveDirPage`.

**Remove `serveDirPage`:**
- This function is no longer needed. Delete it entirely.

**Remove `dirData` struct and `EntryURL` method:**
- No longer used.

**Remove `dirTmpl` from Server struct and `New()`:**
- The dir.html template is no longer parsed or used.

### 5. `internal/web/raw.go` — Keep but Restrict with Extension Allowlist

The raw handler (`/-/raw/*`) stays but is hardened with an extension allowlist.

**Extension allowlist** — only serve files with these extensions:

| Category | Extensions |
|---|---|
| Images | `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`, `.ico`, `.bmp`, `.tiff`, `.avif` |
| Fonts | `.woff`, `.woff2`, `.ttf`, `.eot`, `.otf` |
| Stylesheets | `.css` |
| Documents | `.pdf` |
| Data files | `.json`, `.xml`, `.yaml`, `.yml`, `.csv`, `.tsv` |
| Media (safe) | `.mp4`, `.webm`, `.ogg`, `.mp3`, `.wav` |

All other extensions → 404.

**Additional protections:**
- Reject paths containing `..` (path traversal defense, though git object store should already prevent this)
- Reject paths starting with `.` or `_` (matches nav skip logic, prevents `.env`, `.git/`, etc.)
- Cap response size at 10MB to prevent DoS via large binary blobs

**Implementation:**
Add a package-level `allowedExtensions` map and a `isExtensionAllowed(path string) bool` helper. Add checks at the top of `handleRaw` before reading the blob.

**Remove the redirect from `dispatchToContent`:**
The redirect for non-.md files (when `allowRaw` is true) should be **removed** since directories now return 404. Only the explicit `/-/raw/` route will serve raw files.

### 6. Template Changes

**`internal/assets/templates/dir.html`** — Delete this file. It's no longer used.

**`internal/assets/templates/base.html`** — No changes needed.

**`internal/assets/templates/doc.html`** — No changes needed.

**`internal/assets/templates/index.html`** — No changes needed.

**`internal/assets/assets.go`** — No changes needed (dir.html just won't be referenced).

### 7. `internal/web/server.go` — Cleanup

- Remove `dirTmpl` field from `Server` struct.
- Remove `dirTmpl` parsing in `New()`.
- Add `rootArtifactDir` and `rootArtifactFiles` fields.
- Add artifact route registration loop.

## Content-Type Mapping

For artifact serving, use this mapping:
- `llms.txt` → `text/plain; charset=utf-8`
- `llms-full.txt` → `text/plain; charset=utf-8`
- `robots.txt` → `text/plain; charset=utf-8`
- `sitemap.xml` → `text/xml; charset=utf-8`

## Example Config After Changes

```toml
[server]
addr = ":8080"

[cache]
dir = "~/.cache/folio"
stale_ttl = "5m"

[root_artifacts]
dir = "/etc/folio/root-artifacts"  # optional: serve from disk
[root_artifacts.files]             # optional: inline content
"robots.txt" = "User-agent: *\nDisallow: /"

[[repos]]
host = "github.com"
owner = "myorg"
repo = "myproject"
[repos.web_artifacts]
"llms.txt" = "docs/ai/llms.txt"     # serve llms.txt from docs/ai/llms.txt in repo
"llms-full.txt" = "docs/ai/full.txt"
"robots.txt" = ""                    # explicitly disable
# sitemap.xml not configured → falls back to sitemap.xml in repo root

[[repos]]
host = "github.com"
owner = "other"
repo = "docs"
# no web_artifacts → all artifacts fall back to repo root filenames
```

## Migration Notes

- Existing configs without `web_artifacts` will continue to work — artifacts will fall back to looking for the file at the repo root with the same name.
- The directory listing feature is removed entirely; any paths that previously showed a file tree will now return 404 unless an `index.md` exists.
- The `/-/raw/` route remains functional for direct file access.

## Testing Strategy

1. **Unit tests for config parsing** — verify `web_artifacts` and `root_artifacts` parse correctly.
2. **Unit tests for root artifact handler** — test inline content, disk file, and missing artifact cases.
3. **Unit tests for repo artifact handler** — test explicit mapping, fallback to repo root, disabled artifact, and missing repo.
4. **Integration test** — verify that directory paths return 404 instead of listing.
5. **Verify markdown rendering still works** — images and links to `/-/raw/` should still function.

## Order of Implementation

1. Extend config structs and parsing (`config.go`)
2. Create artifact handler file (`artifacts.go`)
3. Update server routes and fields (`server.go`)
4. Remove directory listing from `doc.go`
5. Delete `dir.html` template
6. Add/update tests
7. Manual testing with `task run`
