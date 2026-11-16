# Workers, modules and services

SSG generates static HTML. The parts of a site that run per request — payments,
forms, comments, a newsletter, accounts, editing in the browser — are worker
templates that ship in this repository, plus two small services you run
yourself.

The split is deliberate: **SSG for content, workers for transactions.** The
content pages stay static (fast, cacheable, cheap); only the handful of `/api/*`
routes and admin panels reach code. Every worker below is optional, and a site
without any of them builds exactly as before.

## Every worker and service in the repository

Sixteen worker templates under `workers/` (each with a `module.json` that
names and describes it) and two services under `services/`. *Pages Function*
means the build copies it into the site through `workers:` and it deploys with
the pages; *standalone Worker* means it has its own `wrangler.toml` and is
deployed on its own from its directory; *Docker service* means a container you
host. Sign-in modes are explained [below](#admin-sign-in-which-mode-to-choose).

| Template | What it does | Runs as | Admin panel and sign-in | Read more |
|---|---|---|---|---|
| `auth` | Accounts with passwords or login providers, revocable sessions, and short signed tokens the other workers verify with a public key | Pages Function | `/auth/admin/`; its own accounts: a password per person, Google, Microsoft, Apple, GitHub, Facebook or Cloudflare Access | [README](../workers/auth/README.md), [ADMIN-GUIDE.md](ADMIN-GUIDE.md) |
| `comments` | Reader comments in D1, protected by Turnstile and held for moderation | Pages Function | `/comments-admin.html`; Access, `auth` or password | [README](../workers/comments/README.md) |
| `contact-form` | Receives a contact or job-application form, checks Turnstile, sends it by email through Resend or MailChannels | Pages Function | none | [README](../workers/contact-form/README.md) |
| `conversions-proxy` | Relays conversion events to the Meta Conversions API; the token stays on the server, the email address is hashed | Pages Function | none | [README](../workers/conversions-proxy/README.md) |
| `cookie-consent` | A GDPR, ePrivacy and UK PECR consent banner: scripts wait for consent, rejecting is as easy as accepting, choices can be logged | Pages Function | none | [README](../workers/cookie-consent/README.md) |
| `core` | The admin home page: lists the site's modules, checks which answer, gathers what needs attention, shares one navigation bar | Pages Function | `/core/`; Access, `auth` or password | [README](../workers/core/README.md), [ADMIN-THEME.md](ADMIN-THEME.md) |
| `designer-ui` | The site's settings and theme files in the browser; each publication is one commit | standalone Worker | its own host or path; **`auth` only** | [README](../workers/designer-ui/README.md), [DESIGNER.md](DESIGNER.md) |
| `dynamic-price` | Current prices from a KV namespace or a pricing API, so a static page shows today's amounts | Pages Function | none | [README](../workers/dynamic-price/README.md) |
| `ecommerce` | A shop for digital products: Stripe or PayPal checkout, VAT, invoices, download links; orders in D1, files in R2 | Pages Function | `/ecommerce-admin/`; Access, `auth` or password | [README](../workers/ecommerce/README.md), [ACP.md](ACP.md), [UCP.md](UCP.md) |
| `editor-ui` | The site's Markdown articles and images in the browser; each publication is one commit (or one MDDB write) | standalone Worker | its own host or path; **`auth` only** | [README](../workers/editor-ui/README.md), [EDITOR.md](EDITOR.md) |
| `newsletter` | Newsletter or waitlist sign-up: addresses in D1, consent stored with its wording, one-click unsubscribe, optional double opt-in | Pages Function | no panel; an admin API (counts, CSV export) behind Access, `auth` or password | [README](../workers/newsletter/README.md) |
| `newsletter-ui` | The sending side of the newsletter: panel, campaigns, a queued send engine, bounces and complaints, statistics | standalone Worker | `/newsletter-admin` by default; Access, `auth` or password | [README](../workers/newsletter-ui/README.md), [SMTP-SERVER.md](SMTP-SERVER.md) |
| `rate-limit` | A request budget for every Function in the project, including ones added later (middleware) | Pages Function | none | [README](../workers/rate-limit/README.md), [below](#bounding-how-often-they-can-be-called) |
| `republish-trigger` | One authenticated webhook that starts a CI build on GitHub, GitLab or Gitea | Pages Function | none; a shared key or an `auth` service token | [README](../workers/republish-trigger/README.md) |
| `stripe-checkout` | Stripe Checkout sessions for one-time payments and subscriptions, with webhook signature checks | Pages Function | none | [README](../workers/stripe-checkout/README.md) |
| `web-push` | Browser notifications for new posts: one button to subscribe, a call after each deploy announces new feed items | Pages Function | none; a shared key or an `auth` service token for sending | [README](../workers/web-push/README.md), [`web_push`](WORKERS-CONFIGURATION.md#web-push) |
| `services/smtp-relay` | A Postfix submission server with DKIM for the newsletter workers' SMTP transport | Docker service (or a Raspberry Pi without Docker) | none; SMTP logins | [README](../services/smtp-relay/README.md), [SMTP-SERVER.md](SMTP-SERVER.md) |
| `services/tts-server` | A text-to-speech HTTP API (espeak-ng, Piper) for build-time MP3s of articles | Docker service | none; API keys, admin keys for voices | [README](../services/tts-server/README.md), [AUDIO.md](AUDIO.md) |

The worker descriptions above are the English `description` of each
`module.json`, shortened; the manifest also carries them in Polish, Hindi,
German and French ([WORKER-MODULES.md](WORKER-MODULES.md)).

## Where to read next

- [WORKERS-CONFIGURATION.md](WORKERS-CONFIGURATION.md) — the `.ssg.yaml` keys:
  `worker:`, `workers:`, `modules:`, `web_push:`.
- [WORKERS-SECRETS.md](WORKERS-SECRETS.md) — every secret and variable, by template.
- [WORKERS-DEPLOYMENT.md](WORKERS-DEPLOYMENT.md) — local development, the
  generated `wrangler.toml`, deploying, and set-ups that keep a Function from running.
- [SERVER-ENDPOINTS.md](SERVER-ENDPOINTS.md) — `endpoints:`, small redirects,
  proxies and forms declared once and run by `--http` or compiled for a platform.
- [ADMIN-GUIDE.md](ADMIN-GUIDE.md) — from one panel with a password to a team
  with accounts, a second factor and the admin home page;
  [ADMIN-THEME.md](ADMIN-THEME.md) — the panels in the site's colours.
- [WORKER-MODULES.md](WORKER-MODULES.md) and
  [MODULE-REQUIREMENTS.md](MODULE-REQUIREMENTS.md) — the module manifest, and
  what your own worker needs to join `auth` and `core`.

## Scaffolding a worker

`ssg new worker <template>` drops a batteries-included, npm-dependency-free
template under `./workers/<template>/` and prints the `worker:` block to add
(or, for a standalone Worker, its deploy steps). It also writes the four files
the templates' tests share — `workers/module-manifest.ts`,
`module-summary-kit.ts`, `module.schema.json` and `module-summary.schema.json`
— when they are missing, so a template's own tests pass in a project of its
own. An existing copy is kept and, if it differs from this release's, named on
stderr; stdout stays the `worker:` block.

```sh
ssg new worker stripe-checkout
```

Each template ships a `README.md` listing the secrets it needs, and a
`module.json` — its module manifest: name and description in five languages,
admin panel, navigation, the permissions its admin surface distinguishes, and
how to switch it off. `comments`, `newsletter`, `newsletter-ui`, `auth` and
`core` also serve it at `…/api/module` behind their admin auth, and all but
`core` a summary (counts, things that need attention, configuration problems)
at `…/api/module/summary`. When `core` is among the site's workers, the build
gathers the manifests into a module registry
([WORKERS-CONFIGURATION.md](WORKERS-CONFIGURATION.md#the-module-registry-modules)).

### Pages Functions vs a standalone Worker

Every template except `newsletter-ui`, `editor-ui` and `designer-ui` is a set
of **Pages Functions**: a `functions/` tree that the build copies into the
site, deployed together with the pages by `wrangler pages deploy`, configured
through `worker:`/`workers:` and a `wrangler.snippet.toml` that
`ssg new wrangler` folds into the site's config.

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
`functions/` and no `pages_build_output_dir` is standalone. A core worker
learns about a standalone Worker from a `modules:` entry.

### Bounding how often they can be called

Every template that writes something — sends an email, stores a comment, logs
a consent — is a public, unauthenticated endpoint. Turnstile raises the cost of
abusing one without capping it: a solved token can be replayed inside its
validity window, and Turnstile is optional in the first place.

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

`comments`, `newsletter`, `newsletter-ui`, `ecommerce` and `core` have an
admin side, and each offers the same three ways in. A worker uses the first
one configured, in the order **Cloudflare Access → `auth` → password**; with
none configured its admin routes answer `503`, never open.

| Mode | Choose it when | What you set |
|---|---|---|
| **Shared password** | One person runs the site. The simplest setup, and fully supported: one secret per worker, nothing else to deploy | `<PREFIX>_ADMIN_PASSWORD` (secret) |
| **Cloudflare Access** | You already run Cloudflare Zero Trust and want your identity provider at the edge | `<PREFIX>_ACCESS_TEAM` + `<PREFIX>_ACCESS_AUD` |
| **`auth`** | Several people, several panels, or different permissions per person (a moderator who may not delete, an editor who may not send) | deploy [`auth`](../workers/auth/README.md), then `<PREFIX>_AUTH_ISSUER`, `<PREFIX>_AUTH_LOGIN_URL` and `<PREFIX>_AUTH_PUBLIC_KEY` (or `_JWKS_URL`) |

The prefixes are `COMMENTS`, `NEWSLETTER` (one set serves both newsletter
templates), `SHOP` and `CORE`. Access and the password allow every action;
with `auth` each action needs its permission, and the password is **not
accepted** while any `<PREFIX>_AUTH_*` variable is set. Delete the password
secret when you switch, so removing the variables later does not quietly bring
it back. `web-push` and `republish-trigger` are called by machines: they keep
their shared key and can also accept an `auth` service token.

`editor-ui` and `designer-ui` are the exception to the three modes: they
accept **only** `auth` (`EDITOR_AUTH_*`, `DESIGNER_AUTH_*`), because every
commit they make needs an author and a permission per action. Without a
complete set they answer `503` on every path ([EDITOR.md](EDITOR.md),
[DESIGNER.md](DESIGNER.md)).

A walk-through from one panel with a password to a team with accounts, a
second factor and the admin home page is in
[ADMIN-GUIDE.md](ADMIN-GUIDE.md). The names of every secret are in
[WORKERS-SECRETS.md](WORKERS-SECRETS.md).

## Tests

Templates with a `test/` directory (`auth`, `comments`, `contact-form`,
`core`, `designer-ui`, `ecommerce`, `editor-ui`, `newsletter`,
`newsletter-ui`, `republish-trigger`, `web-push`) run
with `make test-workers`, after the cross-template test of every `module.json`
and summary (`workers/*.test.ts`) — Node 23.6 or newer, no npm packages. Pull
requests that change `workers/` run the same target in CI (the `Workers`
workflow). The `compat-*` tests pin existing answers so an existing install
keeps working: `newsletter`'s every endpoint as in 1.9.7, `comments`' admin
endpoints, `web-push`'s `notify` and `republish-trigger`'s key check as in
1.9.8.
