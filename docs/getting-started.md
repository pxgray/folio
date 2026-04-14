---
toc: true
---
# Getting started

## Prerequisites

- Go 1.22 or later
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

## Run

```sh
folio serve
```

By default Folio stores its database in `folio.db` in the current directory. To use a different path:

```sh
folio serve --db /var/lib/folio/folio.db
```

You can also set the path via the `FOLIO_DB` environment variable.

## First-run setup

On first launch, open `http://localhost:8080` in your browser. Folio will redirect you to the setup wizard at `/-/setup`.

Fill in:

- **Your name** — display name for the admin account
- **Email** — used to log in
- **Password** — at least 8 characters
- **Listen address** (optional) — defaults to `:8080`
- **Cache directory** (optional) — where bare git clones are stored; defaults to `~/.cache/folio`

After completing setup you'll be redirected to the login page.

## Add a repository

1. Log in at `/-/auth/login`
2. Go to `/-/dashboard/` → **Add repo**
3. Enter the host (e.g. `github.com`), owner, and repo name
4. Click **Save** — Folio begins cloning in the background

Once cloning completes, navigate to `http://localhost:8080/github.com/your-username/your-repo/` to see your docs.

## View a specific commit or branch

Append `?ref=` to any URL:

```
http://localhost:8080/github.com/your-username/your-repo/README.md?ref=v1.0.0
http://localhost:8080/github.com/your-username/your-repo/README.md?ref=abc1234
```

## Next steps

- [Set up webhooks](webhooks.md) for instant updates on push
- [Configuration reference](configuration.md) for all system settings
