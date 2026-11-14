# republish-trigger worker

A single authenticated **webhook** that fires a CI build on **GitHub**,
**GitLab** or **Gitea** — point a headless CMS's "content published" webhook at
it, call it from a cron, or `curl` it, and the site rebuilds and redeploys
without anyone touching the repository. Scaffold it with
`ssg new worker republish-trigger`.

The caller proves itself with a shared secret you choose (`REPUBLISH_KEY`); the
provider credential (`REPUBLISH_TOKEN`) stays server-side in the Function and is
never exposed to the caller.

## Files

```
workers/republish-trigger/
├── functions/api/republish/index.ts   POST (and optional GET) → dispatch
├── functions/api/republish/_lib.ts    key check, provider dispatch, KV debounce
├── functions/api/republish/_auth.ts   who may trigger: REPUBLISH_KEY and/or a service token from `auth`
├── functions/api/republish/_session-verifier.ts  byte-identical copy of workers/auth/portable/ (never edit here)
├── test/                              node --test suite (see *Tests*)
└── wrangler.snippet.toml              optional REPUBLISH_KV + [vars] stub for `ssg new wrangler`
```

## Endpoint

```
POST /api/republish
```

Send the key one of three ways (a header is preferred — a query string leaks
into logs):

```sh
# 1. Bearer
curl -X POST https://your-site/api/republish -H "Authorization: Bearer $KEY"
# 2. custom header
curl -X POST https://your-site/api/republish -H "X-Republish-Key: $KEY"
# 3. query string (only if REPUBLISH_ALLOW_GET=true, for webhooks that can't POST)
curl "https://your-site/api/republish?key=$KEY"
```

**As a CMS webhook:** in your CMS (Sanity, Contentful, Strapi, Ghost, …) add an
outgoing webhook for "content published" pointing at
`https://your-site/api/republish`, method `POST`, with a custom header
`X-Republish-Key: <your key>`. Every publish then rebuilds the site.

| Response | Meaning |
|---|---|
| `202 { ok, provider, ref }` | build dispatched |
| `401 unauthorized` | missing or wrong key (with `REPUBLISH_AUTH_*` set: no valid key or service token, plus a `reason`) |
| `403 forbidden` | a service token without `republish-trigger:run` |
| `405` | GET used while `REPUBLISH_ALLOW_GET` is off |
| `429 too soon` | inside the debounce window (`Retry-After` set) |
| `502 dispatch failed` | provider rejected the call (`upstream_status`, `detail`) |
| `503 republish not configured` | key/token/provider not set (the key may be left unset when `REPUBLISH_AUTH_*` is) |
| `503 not_configured` | `REPUBLISH_AUTH_*` set only partly — `detail` names what is missing |

## 1. Wire it into `.ssg.yaml`

```yaml
workers:
  - name: republish-trigger
    dir: workers/republish-trigger
    routes_include: [/api/republish]
```

## 2. Configure the provider

Set `[vars]` in `wrangler.toml` (see `wrangler.snippet.toml`) and the two
secrets with `wrangler pages secret put`.

### GitHub

```toml
[vars]
REPUBLISH_PROVIDER = "github"
REPUBLISH_REPO = "owner/repo"
REPUBLISH_REF = "main"
REPUBLISH_WORKFLOW = "deploy.yml"   # omit to use repository_dispatch instead
```

- `REPUBLISH_TOKEN`: a PAT with `repo` + `workflow` scope (fine-grained:
  **Actions: write**).
- With `REPUBLISH_WORKFLOW` set, the worker calls **workflow_dispatch** on that
  workflow — which must declare `on: workflow_dispatch:`.
- Without it, the worker sends a **repository_dispatch** event
  (`REPUBLISH_EVENT_TYPE`, default `republish`); your workflow keys off it:

  ```yaml
  on:
    repository_dispatch:
      types: [republish]
  ```

### GitLab

```toml
[vars]
REPUBLISH_PROVIDER = "gitlab"
REPUBLISH_PROJECT_ID = "12345678"   # or URL-encoded "group%2Fproject"
REPUBLISH_REF = "main"
```

- `REPUBLISH_TOKEN`: a **pipeline trigger token** (Settings → CI/CD → Pipeline
  triggers), not a personal access token.
- Self-hosted: set `REPUBLISH_API_BASE = "https://gitlab.example.com/api/v4"`.

### Gitea

```toml
[vars]
REPUBLISH_PROVIDER = "gitea"
REPUBLISH_REPO = "owner/repo"
REPUBLISH_WORKFLOW = "deploy.yaml"
REPUBLISH_API_BASE = "https://git.example.com/api/v1"   # required — Gitea is self-hosted
REPUBLISH_REF = "main"
```

- `REPUBLISH_TOKEN`: a Gitea access token with repo/actions write.
- Needs Gitea Actions enabled and the workflow to declare `on: workflow_dispatch:`.

## 3. Set the secrets

```sh
npx wrangler pages secret put REPUBLISH_KEY     # the secret callers will send
npx wrangler pages secret put REPUBLISH_TOKEN   # the provider credential above
```

## Calling this from CI

Two ways, and you may set up both:

1. **The shared key** — `REPUBLISH_KEY`, sent as above. One secret, nothing
   else to run. It stays a first-class mode.
2. **A service token from the [`auth`](../auth/README.md) worker** — when you
   already run `auth`: one token per caller (a CMS, a cron, a CI job), each
   with its own expiry, revocable alone, carrying exactly
   `republish-trigger:run`. Set `REPUBLISH_AUTH_*` (below); `REPUBLISH_KEY`
   then becomes optional. The token goes in `Authorization: Bearer` only
   (never `X-Republish-Key` or `?key=`).

With both set, both are accepted. That is safe here, unlike in an admin panel:
the key opens this one action and nothing else, so accepting it beside `auth`
bypasses no permission. A half-set `REPUBLISH_AUTH_*` answers `503` naming
what is missing — even to the right key — never a silent fall-back.

A person's session cookie is never accepted, even with the permission: this
endpoint is for machines. A "Republish" button for a signed-in person would be
a separate, CSRF-checked route and is not part of this release. The bearer
value is the short access token from `POST /api/auth/token`, not the
`ssgsvc_…` secret itself.

| Var | Meaning |
|---|---|
| `REPUBLISH_AUTH_ISSUER` | the `iss` to accept, e.g. `https://example.com/api/auth` (required) |
| `REPUBLISH_AUTH_PUBLIC_KEY` or `REPUBLISH_AUTH_JWKS_URL` | exactly one: the public JWK/JWKS, or `/api/auth/jwks` |
| `REPUBLISH_AUTH_LOGIN_URL` | required by the shared verifier, e.g. `/auth/login`; never used here (a machine is never redirected) |
| `REPUBLISH_AUTH_AUDIENCE` | optional; default the request's host |

A complete example — once, as an owner signed in to `auth` (cookies in
`jar.txt`; see [`auth`'s service tokens](../auth/docs/SERVICE-TOKENS.md)):

```sh
H='-H content-type:application/json -H x-ssg-auth:1 -b jar.txt'
SA=$(curl -s $H -d '{"name":"cms-webhook"}' https://example.com/api/auth/admin/service-accounts | jq -r .serviceAccount.id)
ROLE=$(curl -s $H -d '{"name":"republisher","permissions":["republish-trigger:run"]}' https://example.com/api/auth/admin/roles | jq -r .role.id)
curl -s $H -d "{\"roleId\":\"$ROLE\",\"userId\":\"$SA\"}" https://example.com/api/auth/admin/grants
curl -s $H -d '{"name":"nightly rebuild","expiresInDays":90}' \
     https://example.com/api/auth/admin/service-accounts/$SA/tokens | jq -r .secret
# → ssgsvc_… : store it where the caller keeps secrets (shown only once)
```

Then, from the cron or CI job:

```sh
ACCESS=$(curl -fsS -X POST -H "Authorization: Bearer $SSG_SERVICE_TOKEN" https://example.com/api/auth/token | jq -r .accessToken)
curl -fsS -X POST -H "Authorization: Bearer $ACCESS" https://example.com/api/republish
```

A CMS webhook can send only a fixed header, not exchange a token first: keep
`REPUBLISH_KEY` for it, and use service tokens for the callers that run code.

## Guardrails

- **Debounce.** Bind a KV namespace as `REPUBLISH_KV` and set
  `REPUBLISH_MIN_INTERVAL_SEC` (e.g. `60`) to collapse a burst of triggers into
  one build — a second call inside the window gets `429` with `Retry-After`.
- **GET off by default.** A deploy trigger over GET can be fired by a prefetch
  or a crawler, and the key would ride in the URL. Leave `REPUBLISH_ALLOW_GET`
  unset unless a webhook can only send a GET, and prefer a header even then.
- **Rotate the key** by updating the `REPUBLISH_KEY` secret; callers switch to
  the new value. The provider token rotates independently.

## Security notes

The key is compared in constant time. The provider token is only ever sent to
the provider's API, never returned to the caller. Upstream error bodies are
truncated to 300 characters in `detail` for debugging and do not contain the
token.

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, the permissions its admin surface distinguishes, and how to switch it off. It is a description only — no secrets, no configuration.

| Permission | Allows |
|---|---|
| `republish-trigger:run` | Start a build and deploy of the site. |

With `REPUBLISH_AUTH_*` set, a service token from `auth` must hold `republish-trigger:run` ([Calling this from CI](#calling-this-from-ci)); `REPUBLISH_KEY`, when set, keeps working as before.

There is no admin surface to put a manifest endpoint behind, so the template ships the file only; the build reads it into the module registry when the site has the [`core`](../core/README.md) worker.

## Tests

From the repository root, with Node 22.18 or later (TypeScript runs natively):

```sh
node --import ./workers/republish-trigger/test/resolve.mjs --test 'workers/republish-trigger/test/*.test.ts'
```

`compat-key.test.ts` pins the shared-key answers byte for byte (every way of
sending the key, GET on and off, unconfigured, a provider refusing);
`auth-mode.test.ts` covers service tokens (accepted with the permission, `403`
without, never from a cookie, both modes at once, half-configured);
`auth-copies.test.ts` keeps `_session-verifier.ts` identical to
`workers/auth/portable/` (skipped, with the reason, when scaffolded alone).
