# Cloudflare Workers / Pages Functions

SSG generates static HTML. The transactional parts of a commercial site —
payments, form submissions, dynamic pricing, server-side conversion tracking —
need code that runs per request. On Cloudflare Pages that code is a **Pages
Function**, and SSG wires one into the same build and the same deployment.

The split is deliberate: **SSG for content, Workers for transactions.** The
content pages stay static (fast, cacheable, cheap); only the handful of `/api/*`
routes hit a Function.

## The `worker:` section

```yaml
worker:
  dir: workers/contact-form   # a Pages Functions project (contains functions/)
  mode: functions             # "functions" (default) or "worker"
  routes_include:             # what reaches the Function; default ["/api/*"]
    - /api/*
  routes_exclude: []          # carve static paths back out if needed
  wrangler_config: ""         # optional wrangler.toml outside the project root
```

At build time SSG:

1. copies `dir` (its `functions/` tree, or a prebuilt `_worker.js`) into the
   output, and
2. writes `_routes.json` from `routes_include`/`routes_exclude`, so every path
   **not** matched by `include` is served as a static asset and never invokes
   the Function.

`worker:` is empty by default; without it the build is static-only, unchanged.

## Several workers: the `workers:` list

One site can need more than one worker — a cookie-consent endpoint and a
comments API, say. `workers:` is the plural form: a list of **independent**
worker definitions, each with its own routes, config and source. When set it
supersedes the singular `worker:` (which stays for back-compat).

```yaml
workers:
  - name: cookie-consent
    dir: workers/cookie-consent
    routes_include: [/api/consent]
    config:                       # free-form, surfaced to this worker
      countries: [DE, FR, PL]

  - name: comments
    source: https://github.com/acme/ssg-comments   # fetched, not vendored
    auth:                                           # private repo (optional)
      type: bearer
      token: $GITHUB_TOKEN                          # env ref, never a literal
    routes_include: [/api/comments]
    config:
      d1_binding: COMMENTS
      retention_days: 90
```

Per entry:

| Key | Purpose |
|---|---|
| `name` | identifies the worker (logging, collision messages, `config:` key) |
| `dir` | local source directory; where a fetched `source:` lands |
| `source` | optional repo/zip URL to fetch the worker from (GitHub/GitLab repo, or a `.zip`) |
| `auth` | credentials for a private `source:` — `bearer` / `basic` / `header`, secrets as env refs |
| `mode`, `routes_include`, `routes_exclude`, `wrangler_config` | as for the singular `worker:` |
| `config` | free-form settings block passed through to the worker |

How the build treats them: Cloudflare Pages serves a **single** `functions/`
tree and one `_routes.json` per project, so the workers' functions are copied
into that shared tree and their routes are combined. Because they are
independent, **two workers claiming the same output file is a hard error**
(never a silent overwrite) — give them distinct routes. Only one worker may use
`mode: worker` (a project has one `_worker.js`).

A `source:` is fetched into `dir` (default `workers/<name>`) once and reused on
later builds — an already-populated directory is not re-fetched, so a build is
not gated on the network. Vendor the fetched worker (commit it) for
reproducible builds, or keep `source:` to always track upstream.

### `mode: functions` vs `mode: worker`

| Mode | `dir` contains | Deploy path |
|---|---|---|
| `functions` (default) | a `functions/` directory of `.ts`/`.js` handlers | `wrangler pages deploy` (Cloudflare builds them) |
| `worker` | a prebuilt, bundled `_worker.js` | pure-Go Direct Upload — no Node/wrangler needed |

Use `functions` for normal development. Use `worker` when you want SSG's
dependency-free Direct Upload deploy and have already bundled your Worker
yourself (e.g. with esbuild in CI).

## Scaffolding a Function

`ssg new worker <template>` drops a batteries-included, npm-dependency-free
template under `./workers/<template>/` and prints the `worker:` block to add:

| Template | What it does |
|---|---|
| `contact-form` | `POST /api/contact` — Turnstile verify, email via MailChannels (Resend optional) |
| `stripe-checkout` | `POST /api/checkout` (Checkout Session) + `POST /api/stripe-webhook` (HMAC signature verify) |
| `dynamic-price` | `GET /api/price/:sku` from KV or an upstream API, plus a client snippet |
| `conversions-proxy` | `POST /api/track` — server-side Meta CAPI relay with SHA-256-hashed PII |
| `cookie-consent` | GDPR/UK cookie banner: edge geo (EEA+UK), granular categories, script-gating, Consent Mode v2, optional audit log; ships a starter `cookie-policy.md`. See [its README](../workers/cookie-consent/README.md) |
| `comments` | Comments in D1: Turnstile, moderation panel behind a password, heuristic/Akismet spam filter, no accounts, IP kept only as a salted hash. Ships a widget and an admin page. See [its README](../workers/comments/README.md) |
| `republish-trigger` | `POST /api/republish` — one authenticated webhook that fires a CI build on GitHub / GitLab / Gitea (a CMS webhook, cron or curl can redeploy the site). Key-gated, provider token stays server-side, optional KV debounce. See [its README](../workers/republish-trigger/README.md) |

```sh
ssg new worker stripe-checkout
```

Each template ships a `README.md` listing the secrets it needs.

## Secrets

Secrets never live in `.ssg.yaml` or in the Function source — set them per Pages
project with wrangler:

```sh
wrangler pages secret put STRIPE_SECRET_KEY
wrangler pages secret put TURNSTILE_SECRET
```

The Function reads them from its `env` binding at runtime.

## Local development

```sh
ssg --config .ssg.yaml --http --watch
```

When a functions-mode worker is set, `--watch` serves the pages **and** the
Functions together by running `wrangler pages dev .` from the build output
directory (that is where SSG copies each worker's `functions/`, and where
`wrangler pages dev` looks for them). SSG also generates a starter
`wrangler.toml` first if the project has none (see below), so bindings are
available. A prebuilt `mode: worker` keeps `wrangler dev` from its own
directory. An explicit `watch_runner` (or `--wrangler`/`--workerd`) overrides
all of this.

## Generating a wrangler config

`wrangler pages dev` and `wrangler pages deploy` read a `wrangler.toml` for the
build output directory and any bindings. When a project uses workers and has no
wrangler config, SSG writes a starter one — on `--watch`, or on demand:

```sh
ssg new wrangler
```

It derives `name` from the domain and `pages_build_output_dir` from the output
dir, and appends each worker's `wrangler.snippet.toml` — a fragment the worker
ships declaring its bindings and vars (e.g. cookie-consent's optional
`CONSENT_LOG` KV namespace). An existing wrangler config, or one named via a
worker's `wrangler_config`, is never overwritten.

## Deploy

```sh
ssg --config .ssg.yaml --deploy cloudflare --deploy-project my-site
```

- With a `functions/` tree, SSG shells out to `npx wrangler pages deploy` so
  Cloudflare builds the Functions. `CLOUDFLARE_API_TOKEN` and
  `CLOUDFLARE_ACCOUNT_ID` are read from the environment.
- With `mode: worker` (a prebuilt `_worker.js`), the pure-Go Direct Upload path
  is used — no wrangler, no Node.

If wrangler is required but missing, the deploy fails with an actionable
message; switch to `mode: worker` with a prebuilt bundle to avoid the Node
dependency.

## What SSG does not do

- No JS/TS bundler — Cloudflare Pages builds Functions from source; `mode:
  worker` covers prebuilt bundles.
- No secret management — that is `wrangler pages secret put`.
- No KV/Durable Object provisioning — bind those in the Pages dashboard.
