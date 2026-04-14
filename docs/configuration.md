---
toc: true
---
# Configuration reference

Folio stores all configuration in its SQLite database. There is no config file to edit — settings are managed through the admin dashboard at `/-/dashboard/admin/settings`.

## System settings

These settings are available to admins at `/-/dashboard/admin/settings` and via the `/-/api/v1/admin/settings` API.

| Key                          | Description                                                                              |
|------------------------------|------------------------------------------------------------------------------------------|
| `addr`                       | TCP address to listen on (e.g. `:8080`). **Requires restart.**                          |
| `cache_dir`                  | Directory where bare git clones are stored. `~` is expanded. **Requires restart.**      |
| `stale_ttl`                  | How long a resolved ref is cached before a background fetch is triggered (e.g. `5m`, `30s`, `0`). |
| `base_url`                   | Public base URL (e.g. `https://folio.example.com`). Required for OAuth callbacks.       |
| `oauth_github_client_id`     | GitHub OAuth app client ID.                                                              |
| `oauth_github_client_secret` | GitHub OAuth app client secret.                                                          |
| `oauth_google_client_id`     | Google OAuth client ID.                                                                  |
| `oauth_google_client_secret` | Google OAuth client secret.                                                              |

### `stale_ttl` behaviour

| Value   | Effect                                                              |
|---------|---------------------------------------------------------------------|
| `5m`    | Re-check remote HEAD every 5 minutes in the background             |
| `0`     | Never poll; only webhooks invalidate the cache                      |
| `30s`   | Aggressive polling — useful for active development setups           |

Settings marked **Requires restart** take effect on the next server start.

## Per-repo settings

Repos are managed per-user at `/-/dashboard/`. Each repo has:

| Field             | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| **Host**          | Git forge hostname, e.g. `github.com` or `tangled.sh`. Used in the URL: `/{host}/...`  |
| **Owner**         | Repository owner / organisation                                                          |
| **Repo name**     | Repository name                                                                          |
| **Remote URL**    | Clone URL. Defaults to `https://{host}/{owner}/{repo}.git`                              |
| **Webhook secret**| HMAC secret for webhook signature verification. Empty = no verification.                |
| **Trusted HTML**  | When enabled, raw HTML in Markdown passes through without sanitization.                  |

## OAuth setup

To enable GitHub or Google login, you need to create an OAuth app and save the credentials in admin settings.

### GitHub

1. Go to GitHub → **Settings** → **Developer settings** → **OAuth Apps** → **New OAuth App**
2. Set **Authorization callback URL** to: `{base_url}/-/auth/github/callback`
3. Copy the **Client ID** and **Client secret** into Folio's admin settings

### Google

1. Go to [Google Cloud Console](https://console.cloud.google.com/) → **APIs & Services** → **Credentials** → **Create OAuth client ID**
2. Application type: **Web application**
3. Add `{base_url}/-/auth/google/callback` as an authorised redirect URI
4. Copy the **Client ID** and **Client secret** into Folio's admin settings

Set `base_url` in admin settings to the public URL of your Folio instance before configuring OAuth.

## Database path

The database path is set at startup only, not through the admin UI:

```sh
folio serve --db /var/lib/folio/folio.db
```

Or via environment variable:

```sh
FOLIO_DB=/var/lib/folio/folio.db folio serve
```

Defaults to `folio.db` in the current directory.

## Raw file serving

The `/-/raw/` route serves only files with safe extensions:

| Category    | Extensions                                                                                   |
|-------------|----------------------------------------------------------------------------------------------|
| Images      | `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`, `.ico`, `.bmp`, `.tiff`, `.avif`          |
| Fonts       | `.woff`, `.woff2`, `.ttf`, `.eot`, `.otf`                                                   |
| Stylesheets | `.css`                                                                                       |
| Documents   | `.pdf`                                                                                       |
| Data files  | `.json`, `.xml`, `.yaml`, `.yml`, `.csv`, `.tsv`                                             |
| Media       | `.mp4`, `.webm`, `.ogg`, `.mp3`, `.wav`                                                      |

Paths containing `..` or starting with `.` or `_` (at any path segment) are blocked. Responses are capped at 10 MB.
