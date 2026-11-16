# Server endpoints (portable, no vendor lock-in)

Some sites need a little server behind the static output — a redirect that
depends on the request, or a proxy that keeps an upstream key server-side.
`endpoints:` declares those **once, in a vendor-neutral way**. The built-in
server runs them natively (`--http`), in the single Go binary, with no external
runtime — so a self-hosted deploy behind nginx/Caddy or the Docker image gets the
dynamic bits for free. Empty `endpoints:` ⇒ a pure-static build, unchanged.

Endpoints are the small, declarative end of server code. For anything with its
own storage or an admin panel — comments, a newsletter, a shop — use a worker
template instead: [WORKERS.md](WORKERS.md). A worked example is in
[examples/server-endpoints](../examples/server-endpoints/).

## The keys

| Key | Notes |
|---|---|
| `path` | Request path handled by this endpoint, e.g. `/api/quote` (exact match) |
| `type` | `redirect`, `proxy`, `form` or `auth` |
| `to` / `status` | `redirect`: destination and 3xx code (default `302`) |
| `target` | `proxy`: upstream URL; the client's path is replaced by the target's |
| `methods` | `proxy`: allowed HTTP methods (empty = any) |
| `to` | `form`: the webhook the submission is POSTed to as JSON |
| `fields` | `form`: which fields to forward (empty = all submitted fields) |
| `honeypot` | `form`: a field that must stay empty — a filled one is a bot, silently dropped |
| `redirect` | `form`: where the browser goes after a successful submit (`303`); empty = a small JSON ok |
| `allow_private` | `proxy`/`form`: permit a private/loopback upstream or webhook (a self-hosted service) |
| `user` / `password` | `auth`: Basic-auth credentials; `password` should reference an env var (`$MEMBERS_PW`), never a literal |

```yaml
endpoints:
  - path: /go/latest
    type: redirect
    to: /releases/1-8-14/
    status: 302
  - path: /api/quote
    type: proxy
    target: https://api.example.com/v1/quote   # upstream key stays server-side
    methods: [GET, POST]
  - path: /api/contact
    type: form
    to: https://hooks.example.com/email        # delivery webhook stays server-side
    fields: [name, email, message]
    honeypot: company                          # bots that fill it are dropped
    redirect: /thanks/
```

## Forms

A `form` endpoint accepts a `POST`ed submission, drops obvious bots via the
`honeypot` (a hidden field a human leaves empty), and delivers the collected
fields as JSON to `to` — so the delivery webhook (an email service, a chat hook)
is never exposed to the browser. On the self-hosted server the delivery uses the
same dial-time SSRF guard as `proxy`.

## Password-protected sections

An `auth` endpoint guards its `path` **as a prefix** with HTTP Basic auth:

```yaml
endpoints:
  - path: /members/          # protects /members/ and everything under it
    type: auth
    user: ada
    password: $MEMBERS_PW    # from the environment, never a literal
```

The password is read from the named environment variable; the comparison is
constant-time. Auth guards run on the **built-in server only** — on a serverless
platform, protect a section with that platform's own access control — so
`endpoints_platform` compiles the other endpoint types and skips `auth`.

## Proxies and the SSRF guard

A `proxy` endpoint resolves and vets the upstream IP itself and **refuses
loopback/private ranges at dial time** — the same SSRF / DNS-rebinding guard the
external-source client uses — so it can't be turned into a pivot to internal
hosts. Set `allow_private: true` only when the upstream really is a private
self-hosted API. Endpoint responses are sent `Cache-Control: no-store`.

## Same declaration, any target

The built-in server runs endpoints directly. To run the *same* endpoints on a
serverless platform instead, set `endpoints_platform` and the build compiles
them into that platform's functions — no rewrite, no second definition:

| `endpoints_platform` | Emits |
|---|---|
| _(empty)_ | Self-hosted only — served natively by `--http` |
| `cloudflare` | `functions/<path>.js` Pages Functions (the same tree hand-written workers use) |
| `netlify` | `netlify/functions/<name>.mjs` (v2, each declares its own `path` — no `_redirects` wiring) |
| `vercel` | `api/<name>.js` Edge Functions + a `vercel.json` that rewrites each path to its function |

```yaml
endpoints_platform: cloudflare   # compile endpoints: into functions/ at build time
```

Adapters are self-contained plugins — one file per platform — so a new target
drops in without touching the format or your config. Redirect and proxy behave
the same on every target; the proxy's dial-time SSRF guard is specific to the
self-hosted server (on a platform the upstream runs at the edge).
