# conversions-proxy worker

A Cloudflare Pages Function that relays conversion events to the Meta
Conversions API (Pinterest variant commented in the source). The access token
stays server-side and PII (email) is SHA-256 hashed at the edge before it is
sent — the reason to keep this off the client. No npm dependencies.

## Config

```yaml
worker:
  dir: workers/conversions-proxy
  mode: functions
  routes_include:
    - /api/track
```

## Secrets

```sh
wrangler pages secret put META_PIXEL_ID
wrangler pages secret put META_ACCESS_TOKEN
```

## Front-end

`POST /api/track` with `{ event, email?, value?, currency? }`. The Function
adds the client IP, user-agent and referer from the request automatically.

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, and how to switch it off. It is a description only — no secrets, no configuration.

There is no admin surface to put a manifest endpoint behind, so the template ships the file only; the build reads it into the module registry when the site has the [`core`](../core/README.md) worker.
