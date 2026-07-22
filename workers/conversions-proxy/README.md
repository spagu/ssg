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
