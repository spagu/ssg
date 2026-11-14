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
into that shared tree and their routes are combined. Combining **normalises**:
Cloudflare rejects a `_routes.json` whose rules overlap, so a rule already
covered by a splat in the same list is folded into it — `/api/contact` and
`/api/consent/*` beside `/api/*` are published as `/api/*` alone, and the build
says which rules were absorbed. Include and exclude are normalised separately.
This matters most for a middleware worker, which legitimately covers every route
rather than owning one, and for a worker that names no routes at all: it
defaults to `/api/*`, which is exactly the value that overlaps. Because they are
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
| `contact-form` | `POST /api/contact` — Turnstile verify, email via Resend (`CONTACT_RESEND_API_KEY`) or MailChannels (`CONTACT_MAILCHANNELS_API_KEY`); one key is required, 503 without it |
| `stripe-checkout` | `POST /api/checkout` (Checkout Session) + `POST /api/stripe-webhook` (HMAC signature verify) |
| `dynamic-price` | `GET /api/price/:sku` from KV or an upstream API, plus a client snippet |
| `conversions-proxy` | `POST /api/track` — server-side Meta CAPI relay with SHA-256-hashed PII |
| `cookie-consent` | GDPR/UK cookie banner: edge geo (EEA+UK), granular categories, script-gating, Consent Mode v2, optional audit log; ships a starter `cookie-policy.md`. `ssgtheme` wires it from `variables.cookie_consent` rather than literal HTML — see [its README](../workers/cookie-consent/README.md) |
| `comments` | Comments in D1: Turnstile, heuristic/Akismet spam filter, no reader accounts, IP kept only as a salted hash. Ships a widget and a moderation page behind a shared password, Cloudflare Access or `auth` accounts (read, moderate, delete and import as separate permissions), with a record of who moderated what. See [its README](../workers/comments/README.md) |
| `republish-trigger` | `POST /api/republish` — one authenticated webhook that fires a CI build on GitHub / GitLab / Gitea (a CMS webhook, cron or curl can redeploy the site). Key-gated (or an `auth` service token holding `republish-trigger:run`), provider token stays server-side, optional KV debounce. See [its README](../workers/republish-trigger/README.md) |
| `newsletter` | Newsletter / waitlist sign-up in D1: Turnstile, required consent stored with its wording, identical answer for new and existing addresses, RFC 8058 one-click unsubscribe, optional double opt-in (Resend, MailChannels or your own SMTP server), admin counts + CSV export behind a shared password, Cloudflare Access or `auth` accounts, retention purge, IP/UA kept only as salted hashes. Ships a form that works without JavaScript. See [its README](../workers/newsletter/README.md) |
| `web-push` | Browser notifications about new posts: subscriptions in D1, `POST /api/push/notify` (key-gated or an `auth` service token, called from CI after a deploy) reads the feed and sends one message per new post, RFC 8291 encryption and RFC 8292 VAPID with WebCrypto only, dead subscriptions removed. Ships `sw.js` and `push.js`; pair it with `web_push: true`. See [its README](../workers/web-push/README.md) |
| `rate-limit` | `functions/_middleware.ts` — a request budget for every Function in the project, including ones added later. Exact and free through the Workers Rate Limiting binding, KV as a fallback. See [its README](../workers/rate-limit/README.md) |
| `auth` | Optional accounts for the people who run the site's panels. Invitations are the only way in; each person signs in with a password (PBKDF2-SHA256) or Google, Microsoft, Apple, GitHub, Facebook or Cloudflare Access. Roles and groups hand out `worker:action` permissions; an optional second factor (TOTP, backup codes, passkeys; off until `AUTH_MFA_KEY` is set); service tokens for CI; sign-in, account and admin pages in five languages; an audit log. Sessions are a short ES256 token in a `__Host-` cookie with a rotating refresh token, checked by the other workers locally with the public key. See [its README](../workers/auth/README.md) |
| `core` | The admin home page at `/core/`: what needs attention in every module, a card per module, the site's build and configuration problems; a shared navigation bar (`/core/nav.js`) the panels include; module order, visibility and labels with the optional `CORE_DB`; the panels in the site's colours from `admin_theme:`. Reads the module registry the build writes when `core` is in `workers:` and asks each module how it is (same host only, timeouts, only the caller's own credential forwarded). Shared password with a lockout, the `auth` worker's session, or Cloudflare Access. Not a security boundary: it never proxies or authorizes for a module. See [its README](../workers/core/README.md) |
| `newsletter-ui` | **Standalone Worker**, not Pages Functions (see below). Sends the `newsletter` template's list, on the same D1 database: a browser panel and admin API, campaigns (Markdown or HTML, segments, preview, test send, a draft from a feed post), a send engine on a cron trigger and a queue (rate limit, each reader at most once, uncertain sends shown instead of repeated), bounces and complaints from SMTP replies, signed Resend/MailChannels webhooks or a CSV, CSV import of lists from other tools, statistics without tracking anyone. Cloudflare Access, `auth` accounts with per-action permissions and an audit trail, or HTTP Basic with a per-IP failed-login lockout, CSRF header on every write, its own tables, never alters `subscribers`. API in OpenAPI 3.1. See [its README](../workers/newsletter-ui/README.md) |

```sh
ssg new worker stripe-checkout
```

Each template ships a `README.md` listing the secrets it needs, and a
`module.json` — its module manifest: name and description in five languages,
admin panel, navigation, the permissions its admin surface distinguishes, and
how to switch it off. `comments`, `newsletter`, `newsletter-ui`, `auth` and
`core` also serve it at `…/api/module` behind their admin auth, and all but
`core` a summary (counts, things that need attention, configuration problems)
at `…/api/module/summary`. See
[WORKER-MODULES.md](WORKER-MODULES.md) for the format and how to make your own
worker a module.

**The module registry.** When one of the workers in `workers:` is the `core`
worker (its `module.json` has `"id": "core"`), the build reads every listed
worker's `module.json`, checks it, and writes `functions/_ssg-modules.json` —
a registry the core Pages Function imports, which wrangler bundles and never
serves as a static file. Modules the build cannot see, such as a standalone
Worker deployed on its own, are declared under `modules:`:

```yaml
modules:
  - id: newsletter-ui
    url: /newsletter-admin      # its base path on the same host
    dir: workers/newsletter-ui  # optional: read its module.json at build time
```

Without a `core` worker the build writes nothing new and the output is the
same as before; `modules:` entries then only earn a warning that they have no
effect. Problems are reported under `check_modules` (`off`, `warn` — the
default — or `strict`). Details: [WORKER-MODULES.md](WORKER-MODULES.md#the-module-registry),
keys: [CONFIGURATION.md](CONFIGURATION.md#the-module-registry-modules).

### Pages Functions vs a standalone Worker

Every template above except `newsletter-ui` is a set of **Pages Functions**:
a `functions/` tree that the build copies into the site, deployed together with
the pages by `wrangler pages deploy`, configured through `worker:`/`workers:`
and a `wrangler.snippet.toml` that `ssg new wrangler` folds into the site's
config.

`newsletter-ui` is a **standalone Worker**: it ships its own `wrangler.toml`
(`main = "src/index.ts"`) and is deployed on its own, from its directory:

```sh
ssg new worker newsletter-ui
cd workers/newsletter-ui
# database_id = the same D1 id as the newsletter template
npx wrangler queues create ssg-newsletter-send
npx wrangler queues create ssg-newsletter-send-dlq
npx wrangler deploy
```

It exists because Pages Functions only answer requests. They have no cron
trigger and cannot consume a queue, and a newsletter campaign needs both: the
cron starts scheduled campaigns, and the queue sends them in batches that
survive a failure, can be paused and resumed, and end up in a dead-letter queue
instead of disappearing. The two workers share one D1 database
(`NEWSLETTER_DB`): `newsletter` owns `subscribers`, `newsletter-ui` owns its own
tables and never changes that one.

For a template like this, `ssg new worker` prints deploy steps instead of a
`worker:` block, and `ssg new wrangler` leaves it out of the site's
`wrangler.toml`. Listing such a directory under `worker:`/`workers:` stops the
build with an error. Copying it into `functions/` would turn its sources and
tests into public routes. The rule: a directory with a `wrangler.toml`, no
`functions/` and no `pages_build_output_dir` is standalone.

### Bounding how often they can be called

Every template above that writes something — sends an email, stores a comment,
logs a consent — is a public, unauthenticated endpoint. Turnstile raises the
cost of abusing one without capping it: a solved token can be replayed inside
its validity window, and Turnstile is optional in the first place.

`rate_limit` / `rate_burst` in `.ssg.yaml` bound the **built-in preview server**
only. `rate-limit` is the deployed counterpart:

```sh
ssg new worker contact-form
ssg new worker rate-limit
cp -r workers/rate-limit/functions/_middleware.ts workers/contact-form/functions/
```

It is middleware, so it wraps whatever is in `functions/` without either side
knowing about the other. `RATE_LIMIT_SKIP` carves paths back out — the
newsletter's one-click unsubscribe, POSTed by mailbox providers from a few
shared addresses, is the case it exists for. Without a backend bound it is a **no-op** — a limiter
that turns visitors away because nobody finished configuring it is worse than no
limiter — and it fails open by default, which is right for a contact form and
wrong for a checkout, so `RATE_LIMIT_FAIL = "closed"` is there for the latter.

Prefer the `RATE_LIMITER` binding over KV. KV is eventually consistent, so a
burst arriving at several points of presence at once can overshoot the cap,
which is exactly the shape a spam run takes.

## Admin sign-in: which mode to choose

`comments`, `newsletter`, `newsletter-ui` and `core` have an admin side, and
each offers the same three ways in. A worker uses the first one configured,
in the order **Cloudflare Access → `auth` → password**; with none configured
its admin routes answer `503`, never open.

| Mode | Choose it when | What you set |
|---|---|---|
| **Shared password** | One person runs the site. The simplest setup, and fully supported: one secret per worker, nothing else to deploy | `<PREFIX>_ADMIN_PASSWORD` (secret) |
| **Cloudflare Access** | You already run Cloudflare Zero Trust and want your identity provider at the edge | `<PREFIX>_ACCESS_TEAM` + `<PREFIX>_ACCESS_AUD` |
| **`auth`** | Several people, several panels, or different permissions per person (a moderator who may not delete, an editor who may not send) | deploy [`auth`](../workers/auth/README.md), then `<PREFIX>_AUTH_ISSUER`, `<PREFIX>_AUTH_LOGIN_URL` and `<PREFIX>_AUTH_PUBLIC_KEY` (or `_JWKS_URL`) |

The prefixes are `COMMENTS`, `NEWSLETTER` (one set serves both newsletter
templates) and `CORE`. Access and the password allow every action; with
`auth` each action needs its permission, and the password is **not accepted**
while any `<PREFIX>_AUTH_*` variable is set. Delete the password secret when
you switch, so removing the variables later does not quietly bring it back.
`web-push` and `republish-trigger` are called by machines: they keep their
shared key and can also accept an `auth` service token.

A walk-through from one panel with a password to a team with accounts, a
second factor and the admin home page is in
[ADMIN-GUIDE.md](ADMIN-GUIDE.md).

## Secrets

Secrets never live in `.ssg.yaml` or in the Function source — set them per Pages
project with wrangler:

```sh
wrangler pages secret put STRIPE_SECRET_KEY
wrangler pages secret put CONTACT_TURNSTILE_SECRET
```

The Function reads them from its `env` binding at runtime.

### One project, one environment: prefix your secrets

Cloudflare Pages gives a project **one** set of variables and secrets, and every
worker merged into that project's `functions/` tree reads all of it. A secret
set for one template is therefore visible to every other — and a template that
reads an unprefixed name picks up a value meant for someone else.

That is how a shared `TURNSTILE_SECRET` used to break the cookie-consent audit
log: set for `contact-form` or `comments`, it switched Turnstile verification on
in `cookie-consent` too, whose banner never sends a token, so every consent
record was dropped with `{"ok":true,"stored":false}` (#325).

Each template now reads its **own prefixed name first**:

| Template | Turnstile secret | Falls back to `TURNSTILE_SECRET`? |
|---|---|---|
| `contact-form` | `CONTACT_TURNSTILE_SECRET` | yes — existing projects keep working |
| `comments` | `COMMENTS_TURNSTILE_SECRET` | yes — existing projects keep working |
| `newsletter` | `NEWSLETTER_TURNSTILE_SECRET` | yes |
| `auth` | `AUTH_TURNSTILE_SECRET` (after repeated failed logins) | yes |
| `cookie-consent` | `CONSENT_TURNSTILE_SECRET` (opt-in, audit log only) | **no** — the shared name is ignored |

The fallback means nothing changes for a project that already sets
`TURNSTILE_SECRET` for its contact form or comments, except that the consent log
starts storing records again. New projects should set the prefixed names. The
same rule applies to anything that switches behaviour on: the newsletter's
double opt-in keys (`NEWSLETTER_RESEND_API_KEY`, `NEWSLETTER_MAILCHANNELS_API_KEY`)
deliberately have no unprefixed fallback.

`newsletter-ui` is a separate Worker, so it has its own environment: set its
secrets with `wrangler secret put` (not `wrangler pages secret put`) from
`workers/newsletter-ui/`. It reads the **same names** as the `newsletter`
admin, so one set of values serves both:

| Template | Secret / variable | Purpose |
|---|---|---|
| `newsletter-ui` | `NEWSLETTER_ADMIN_PASSWORD` (secret) | HTTP Basic admin password, when Access is not used |
| `newsletter-ui` | `NEWSLETTER_ACCESS_TEAM` + `NEWSLETTER_ACCESS_AUD` (vars) | Cloudflare Access instead of a password |
| `newsletter`, `newsletter-ui` | `NEWSLETTER_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` (vars) | accounts of the `auth` worker with per-action permissions (`newsletter:read`, `:edit`, `:send`, …) and, in `newsletter-ui`, an audit trail; checked after Access, before the password, which is then not accepted |
| `newsletter-ui` | `NEWSLETTER_AUDIT_RETENTION_DAYS` (var) | days the admin audit trail keeps a row, default 365, `0` = forever |
| `newsletter-ui` | `NEWSLETTER_HASH_SALT` (secret, optional) | salts the short-lived failed-login records |
| `newsletter-ui` | `NEWSLETTER_UI_BASE` (var) | path prefix, default `/newsletter-admin` |
| `newsletter`, `newsletter-ui` | `NEWSLETTER_SMTP_PASSWORD` (secret) + `NEWSLETTER_SMTP_HOST`, `_PORT`, `_USER`, `_HELO` (vars) | your own SMTP server as the mail transport, used when neither provider key is set (order: Resend → MailChannels → SMTP); port 465 or 587, never 25 |
| `newsletter-ui` | `NEWSLETTER_RESEND_WEBHOOK_SECRET`, `NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY` (secrets) | verify the bounce/complaint webhooks |

The full list of `newsletter-ui` variables (send rate, batch size, click
counting, feed URL, …) is the "Configuration" table in
[its README](../workers/newsletter-ui/README.md#configuration).

The admin sign-in settings of the other templates, and the secrets of `auth`
and `core` (both Pages Functions: `wrangler pages secret put`):

| Template | Secret / variable | Purpose |
|---|---|---|
| `comments` | `COMMENTS_ADMIN_PASSWORD` (secret) | shared-password moderation |
| `comments` | `COMMENTS_ACCESS_TEAM` + `COMMENTS_ACCESS_AUD` (vars) | Cloudflare Access |
| `comments` | `COMMENTS_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` (vars) | `auth` accounts: `comments:read`, `:moderate`, `:delete`, `:import` |
| `newsletter` | `NEWSLETTER_ADMIN_PASSWORD` (secret), `NEWSLETTER_ACCESS_TEAM` + `_AUD`, `NEWSLETTER_AUTH_*` (vars) | the same three modes, the same names as `newsletter-ui` |
| `core` | `CORE_ADMIN_PASSWORD` (secret) | shared password, with a per-IP lockout (`CORE_LOGIN_MAX_FAILURES`, `CORE_LOGIN_WINDOW_MINUTES`) |
| `core` | `CORE_ACCESS_TEAM` + `CORE_ACCESS_AUD`, `CORE_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` (vars) | Cloudflare Access; `auth` accounts, each seeing only the modules it may open |
| `core` | `CORE_DB` (D1), `CORE_LOGIN_LIMITER` (rate limit) (optional bindings) | module management and a lockout shared by all isolates; a ceiling on password attempts |
| `web-push` | `PUSH_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` (vars) | accept `auth` service tokens holding `web-push:notify`; `PUSH_KEY` keeps working |
| `republish-trigger` | `REPUBLISH_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` (vars) | accept `auth` service tokens holding `republish-trigger:run`; `REPUBLISH_KEY` keeps working |
| `auth` | `AUTH_SIGNING_KEY` (secret, required) | the private signing key from `node workers/auth/scripts/auth-keys.mjs` |
| `auth` | `AUTH_BOOTSTRAP_TOKEN` (secret) | creates the first owner once; delete it afterwards |
| `auth` | `AUTH_MFA_KEY` (secret) | switches the second factor on and encrypts the authenticator secrets; `AUTH_MFA_KEY_PREVIOUS` during a rotation, `AUTH_MFA_RESET_TOKEN` for a sole owner's break-glass |
| `auth` | `AUTH_HASH_SALT`, `AUTH_TURNSTILE_SECRET` (secrets) | salted IP/User-Agent hashes; Turnstile after repeated failures |
| `auth` | `AUTH_<PROVIDER>_CLIENT_SECRET`, `AUTH_APPLE_PRIVATE_KEY` (secrets) with `AUTH_<PROVIDER>_CLIENT_ID` (vars) | sign-in through Google, Microsoft, GitHub, Facebook, Apple |
| `auth` | `AUTH_RESEND_API_KEY`, `AUTH_MAILCHANNELS_API_KEY` or `AUTH_SMTP_PASSWORD` (secrets) with `AUTH_MAIL_FROM` | invitation and reset mail; without a transport the link is shown to the administrator |

Every `auth` variable is in the "Configuration" section of
[its README](../workers/auth/README.md#configuration) and in its
`wrangler.snippet.toml`; `core`'s in [its README](../workers/core/README.md#variables).

**Tests.** Templates with a `test/` directory (`auth`, `comments`,
`contact-form`, `core`, `newsletter`, `newsletter-ui`, `republish-trigger`,
`web-push`) run with `make test-workers`, after the cross-template test of
every `module.json` and summary (`workers/*.test.ts`) — Node 23.6 or newer,
no npm packages. Pull requests that change `workers/` run the same target in
CI (the `Workers` workflow). The `compat-*` tests pin existing answers so an
existing install keeps working: `newsletter`'s every endpoint as in 1.9.7,
`comments`' admin endpoints, `web-push`'s `notify` and `republish-trigger`'s
key check as in 1.9.8.

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

The runner needs Node.js (`npx wrangler`). If the runner is not installed, the
preview still starts, and ssg says once that `/api/*` is not served. This is
the case in the ssg Docker image, which has no Node. Two ways to handle it:

- Run `npx wrangler pages dev output` on the host, beside the container,
  for the Functions.
- Set `watch_runner: none` to preview the static site alone without the note.

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

## Deployment topologies (which requests reach your Functions)

Whether a `workers:` endpoint actually runs depends on the shape of your
Cloudflare deployment, not just on `routes_include`. Two caveats bite silently.

| Topology | What runs | Functions active? |
|---|---|---|
| Pure static (no `worker:`/`workers:`) | assets only | — |
| Functions mode (`mode: functions`, the default) | `functions/` tree per `_routes.json` | **yes** |
| Advanced mode (a `_worker.js` at the output root) | that single Worker for **every** request | **no** |
| Zone Worker route (e.g. `example.com/api/*`) owns a prefix | the zone Worker on that prefix | shadowed there |

- **Advanced mode disables Functions entirely.** If a `_worker.js` sits at the
  output root (Pages "advanced mode"), Cloudflare runs *only* that Worker and the
  `functions/` tree is ignored — so every `workers:` endpoint is silently dead.
  Pick one: the `functions/` model (`mode: functions`) **or** a single hand-built
  `_worker.js` (`mode: worker`) that does its own routing. SSG's `workers:` merge
  targets the Functions model.
- **Zone-level Worker routes shadow Function routes.** A route like
  `example.com/api/*` bound to a standalone Worker on the zone runs *before* Pages
  Functions, so a `workers:` endpoint under the same prefix (say
  `/api/consent/*`) never fires. Either move the endpoint off the owned prefix, or
  carve it out of the zone route. A quick smoke test after deploy — `curl` each
  endpoint path — catches both cases before your visitors do.

## What SSG does not do

- No JS/TS bundler — Cloudflare Pages builds Functions from source; `mode:
  worker` covers prebuilt bundles.
- No secret management — that is `wrangler pages secret put`.
- No KV/Durable Object provisioning — bind those in the Pages dashboard.
