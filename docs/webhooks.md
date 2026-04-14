---
toc: true
---
# Webhook setup

Webhooks let Folio update immediately when you push to a repo, rather than waiting for the TTL to expire.

## How it works

Folio exposes a webhook endpoint for each configured repo:

```
POST /{host}/{owner}/{repo}/-/webhook
```

When this endpoint receives a valid push event, Folio immediately fetches the latest commits and clears its ref cache. The next request will see the new content.

## GitHub

### 1. Generate a secret

```sh
openssl rand -hex 32
```

Copy the output — you'll need it in both GitHub and Folio's repo settings.

### 2. Add the secret to the repo settings

In Folio's dashboard, open the repo's edit page (`/-/dashboard/repos/{id}`) and paste the secret into the **Webhook secret** field.

### 3. Configure the webhook in GitHub

1. Go to your repo → **Settings** → **Webhooks** → **Add webhook**
2. Set **Payload URL** to: `https://your-folio-server.example.com/github.com/your-username/your-repo/-/webhook`
3. Set **Content type** to `application/json`
4. Set **Secret** to the same secret you put in `folio.toml`
5. Choose **Just the push event**
6. Click **Add webhook**

GitHub will send a ping request — Folio will respond with `200 ok`.

### Signature verification

Folio verifies the `X-Hub-Signature-256` header on all webhook requests when `webhook_secret` is non-empty. Requests with an invalid or missing signature are rejected with `401 Unauthorized`.

If `webhook_secret` is empty, **no verification is performed** — any POST to the endpoint triggers a fetch.

## tangled.sh

tangled.sh uses standard git-over-HTTPS. Configure a webhook pointing to the same endpoint format. If tangled.sh supports HMAC signatures, set the same `webhook_secret` in your config.

## Testing a webhook

You can fire a test webhook manually:

```sh
# Without a secret (no HMAC check):
curl -X POST https://your-folio-server.example.com/github.com/owner/repo/-/webhook

# With a secret:
SECRET="your-secret"
BODY='{"ref":"refs/heads/main"}'
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print "sha256="$2}')
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-Hub-Signature-256: $SIG" \
  -d "$BODY" \
  https://your-folio-server.example.com/github.com/owner/repo/-/webhook
```

A successful response is `200 ok`.
