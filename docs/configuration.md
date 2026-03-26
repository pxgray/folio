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

```toml
[[repos]]
host           = "github.com"
owner          = "pxgray"
repo           = "folio"
webhook_secret = ""

[[repos]]
host           = "tangled.sh"
owner          = "alice"
repo           = "notes"
remote         = "https://tangled.sh/alice/notes.git"
webhook_secret = "hunter2"
```

## Full example

```toml
[server]
addr = ":8080"

[cache]
dir       = "~/.cache/folio"
stale_ttl = "5m"

[[repos]]
host           = "github.com"
owner          = "pxgray"
repo           = "folio"
webhook_secret = "my-github-secret"

[[repos]]
host  = "github.com"
owner = "golang"
repo  = "go"
```
