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
- Serves all `.md` files; rewrites repo-relative links automatically
- `?ref=<sha-or-branch>` for historical views
- Webhook-driven instant updates (GitHub, tangled.sh)
- TTL fallback for repos without webhooks

## License

MIT
