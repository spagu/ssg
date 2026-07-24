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
└── functions/api/republish/_lib.ts    auth, provider dispatch, KV debounce
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
| `401 unauthorized` | missing or wrong key |
| `405` | GET used while `REPUBLISH_ALLOW_GET` is off |
| `429 too soon` | inside the debounce window (`Retry-After` set) |
| `502 dispatch failed` | provider rejected the call (`upstream_status`, `detail`) |
| `503 republish not configured` | key/token/provider not set |

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
