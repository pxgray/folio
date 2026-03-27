---
toc: true
---
# Getting started

## Prerequisites

- Go 1.22 or later
- Git (for initial clone operations)
- A public git repository on GitHub or [tangled.sh](https://tangled.sh)

## Install

```sh
go install github.com/pxgray/folio/cmd/folio@latest
```

Or build from source:

```sh
git clone https://github.com/pxgray/folio.git
cd folio
go build ./cmd/folio/
```

## Create a config file

Create `folio.toml` in your working directory:

```toml
[server]
addr = ":8080"

[cache]
dir       = "~/.cache/folio"
stale_ttl = "5m"

[[repos]]
host  = "github.com"
owner = "your-username"
repo  = "your-repo"
```

See [configuration](configuration.md) for all available options.

## Run

```sh
folio folio.toml
```

Folio will clone the configured repos into `~/.cache/folio/` on first run (this may take a moment for large repos), then start serving on `:8080`.

Open `http://localhost:8080` in your browser to see the repo list, or navigate directly to:

```
http://localhost:8080/github.com/your-username/your-repo/
```

## View a specific commit

Append `?ref=` to any URL:

```
http://localhost:8080/github.com/your-username/your-repo/README.md?ref=v1.0.0
http://localhost:8080/github.com/your-username/your-repo/README.md?ref=abc1234
```

## Next steps

- [Set up webhooks](webhooks.md) for instant updates on push
- [Full configuration reference](configuration.md)
