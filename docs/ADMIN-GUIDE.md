# Setting up the admin tools

This guide takes a site built with ssg from no admin tools at all to a working
set: a newsletter with its sending panel, comment moderation, accounts with
roles and a second factor, one sign-in for every panel, and an admin home page
in the site's own colours. It follows the order you would install things in,
and every command is meant to be run from the site's root directory (the one
with `.ssg.yaml` and `workers/`).

Each worker has its own README with the full reference; this page links to
the parts you need and does not repeat them. Background on workers in general:
[WORKERS.md](WORKERS.md). The module manifest and the registry:
[WORKER-MODULES.md](WORKER-MODULES.md).

## What each worker does

| Worker | Kind | What it gives you | Needs |
|---|---|---|---|
| `newsletter` | Pages Functions | the sign-up form, confirmation, one-click unsubscribe, counts and a CSV | D1, Turnstile |
| `newsletter-ui` | standalone Worker | the sending panel: campaigns, the send engine (cron and queue), bounces, statistics, an audit trail | the same D1 as `newsletter`, two queues, a mail transport |
| `comments` | Pages Functions | comments in D1 with a moderation page at `/comments-admin.html` | D1, Turnstile |
| `auth` | Pages Functions | accounts, invitations, roles and permissions, a second factor, the accounts admin panel at `/auth/admin/`, a short signed token every other worker can check | its own D1 |
| `core` | Pages Functions | the admin home at `/core/`, the shared navigation bar, module management, the panels' colours from `admin_theme:` | optionally its own D1 (`CORE_DB`) |

## Which ones you need

| You want | Install |
|---|---|
| A sign-up form, and you send with something else | `newsletter` |
| To send campaigns from your own Cloudflare account | `newsletter` + `newsletter-ui` |
| Comments | `comments` |
| More than one person in the panels, or different rights per person | add `auth` |
| One page that says what needs attention, one bar across all panels, your brand colours | add `core` |

Every panel works on its own, with nothing but a password. `auth` and `core`
are additions, not requirements.

## Choose how people sign in

The panels (`comments`, `newsletter`, `newsletter-ui`, `core`) each accept the
same three modes, checked in this order: **Cloudflare Access → `auth` →
shared password**. The first one configured decides; nothing configured
answers `503`, never an open panel.

| Mode | Choose it when | What you set per worker |
|---|---|---|
| **Shared password** | one person runs the site — the simplest choice, nothing else to deploy | the `<PREFIX>_ADMIN_PASSWORD` secret |
| **Cloudflare Access** | you already use Cloudflare Zero Trust | `<PREFIX>_ACCESS_TEAM` + `<PREFIX>_ACCESS_AUD` |
| **`auth` accounts** | a team: several people, several panels, different rights — the recommended step once there is more than one of you | `<PREFIX>_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` (or `_JWKS_URL`) |

The prefixes are `COMMENTS`, `NEWSLETTER` (shared by `newsletter` and
`newsletter-ui`) and `CORE`. The shared password is a fully supported mode
and stays one. With the password or Access every signed-in person may do
everything; with `auth` each action needs its own permission
(`newsletter:send`, `comments:moderate`, …) and each change records who made
it. More on the choice: [WORKERS.md](WORKERS.md#admin-sign-in-which-mode-to-choose).

The rest of this guide sets up `auth`. If you stay with the password, set
`NEWSLETTER_ADMIN_PASSWORD`, `COMMENTS_ADMIN_PASSWORD` and
`CORE_ADMIN_PASSWORD` instead of the `*_AUTH_*` variables below and skip the
"Accounts" steps.

## 1. Scaffold the workers

```sh
ssg new worker newsletter
ssg new worker comments
ssg new worker auth
ssg new worker core
ssg new worker newsletter-ui
```

Each command copies a template into `workers/<name>/`. Scaffold `core` before
`newsletter-ui`: once `workers/core` exists, `ssg new worker newsletter-ui`
also prints the `modules:` entry it needs.

**Use the routes below, not the generic block `ssg new worker` prints.** It
prints `routes_include: [/api/*]` for every Pages template; `auth` also needs
`/auth/*` for its sign-in pages, and its static files excluded.

## 2. Configure `.ssg.yaml`

```yaml
build_report: build.json        # optional: the admin home shows the last build

workers:
  - name: newsletter
    dir: workers/newsletter
    routes_include: [/api/newsletter/*]
  - name: comments
    dir: workers/comments
    routes_include: [/api/comments/*]
  - name: auth
    dir: workers/auth
    routes_include: [/api/auth/*, /auth/*]
    routes_exclude: [/auth/assets/*]
  - name: core
    dir: workers/core            # with core listed, the build writes the module registry
    routes_include: [/api/core/*]

modules:                         # standalone Workers the build does not bundle
  - id: newsletter-ui
    url: /newsletter-admin       # its base path on this host
    dir: workers/newsletter-ui   # read its module.json at build time

admin_theme:                     # optional: the panels in your brand
  name: Harbour Bakery
  logo: /img/logo.svg            # a file in static/
  logo_dark: /img/logo-dark.svg  # for the bar, which is dark in both schemes
  main: "#1d3557"
  accent: "#e76f51"
```

Do **not** list `newsletter-ui` under `workers:`: it is a standalone Worker,
and the build refuses it there. Build once:

```sh
ssg --config .ssg.yaml
```

The output should end with `module registry → functions/_ssg-modules.json`
and, with `admin_theme:`, `admin theme → core/theme.css`. The registry is
bundled into the `core` function by wrangler and never served as a file.

`admin_theme` keys, checks and how the colours are derived:
[CONFIGURATION.md](CONFIGURATION.md) ("Admin panels' look") and the
[core README](../workers/core/README.md#panel-look-admin_theme). Give `main` a
colour that can carry white text; the build moves a colour that fails a
contrast pair to the nearest lightness that passes and the admin home's
**Appearance** view says where.

## 3. Databases, keys and secrets

Generate a starter `wrangler.toml` for the Pages project; it appends each
worker's `wrangler.snippet.toml` as comments:

```sh
ssg new wrangler
```

Create the databases and paste each id into its uncommented
`[[d1_databases]]` block:

```sh
npx wrangler d1 create ssg-newsletter   # binding NEWSLETTER_DB
npx wrangler d1 create ssg-comments     # binding COMMENTS_DB
npx wrangler d1 create ssg-auth         # binding AUTH_DB
npx wrangler d1 create core             # binding CORE_DB (optional: module management)
```

The signing key for `auth`, made by the script the template ships:

```sh
node workers/auth/scripts/auth-keys.mjs
```

It prints three lines: `# key id …`, `AUTH_SIGNING_KEY={…"d":…}` (the
private key — a secret) and `AUTH_PUBLIC_KEY={…}` (the public key — safe to
put in a config file). Then the secrets:

```sh
npx wrangler pages secret put AUTH_SIGNING_KEY       # paste the JSON after "AUTH_SIGNING_KEY="
openssl rand -base64 32 | npx wrangler pages secret put AUTH_BOOTSTRAP_TOKEN
openssl rand -base64 32 | npx wrangler pages secret put AUTH_MFA_KEY
openssl rand -base64 32 | npx wrangler pages secret put AUTH_HASH_SALT   # optional
npx wrangler pages secret put NEWSLETTER_TURNSTILE_SECRET
npx wrangler pages secret put COMMENTS_TURNSTILE_SECRET
```

Keep the bootstrap token you generated; you need it once, in step 5.

**Set `AUTH_MFA_KEY` now.** The second factor is off until this secret exists
(at least 32 characters). Without it an owner signs in with a password alone,
and an owner who has not enrolled a factor yet can enrol one on a password
sign-in — so until the first owner has a factor, whoever has that password
can take the account ([SECOND-FACTOR.md](../workers/auth/docs/SECOND-FACTOR.md),
[the security review](../workers/auth/docs/SECURITY-REVIEW.md)).

## 4. Tell every panel to accept `auth` sessions

In the site's `wrangler.toml`, under `[vars]` — the same three values for each
prefix, with the `AUTH_PUBLIC_KEY=` line from step 3:

```toml
[vars]
COMMENTS_AUTH_ISSUER = "https://example.com/api/auth"
COMMENTS_AUTH_LOGIN_URL = "/auth/login"
COMMENTS_AUTH_PUBLIC_KEY = '{"kty":"EC","crv":"P-256","x":"…","y":"…"}'
NEWSLETTER_AUTH_ISSUER = "https://example.com/api/auth"
NEWSLETTER_AUTH_LOGIN_URL = "/auth/login"
NEWSLETTER_AUTH_PUBLIC_KEY = '{"kty":"EC","crv":"P-256","x":"…","y":"…"}'
CORE_AUTH_ISSUER = "https://example.com/api/auth"
CORE_AUTH_LOGIN_URL = "/auth/login"
CORE_AUTH_PUBLIC_KEY = '{"kty":"EC","crv":"P-256","x":"…","y":"…"}'
```

The issuer is `auth`'s default, `https://<your host>/api/auth`. Each worker
checks the token itself with the public key, without calling `auth`
([CONTRACT.md](../workers/auth/docs/CONTRACT.md)). A set that is incomplete — an
issuer without a login URL, no key, two keys — answers `503 not_configured`
naming what is missing; it never falls back to the password.

So that the accounts panel offers your modules' permissions when you build a
role, print `AUTH_KNOWN_PERMISSIONS` from their manifests and add it to
`[vars]` as a single-quoted string:

```sh
node workers/auth/scripts/auth-permissions.mjs --base newsletter-ui=/newsletter-admin workers/newsletter-ui/module.json workers/comments/module.json workers/core/module.json
```

The value carries English descriptions only (Cloudflare caps one variable at
5 KB), plus each module's title and panel, so "Your panels" (`/auth/`) can
name and link it. A standalone Worker such as `newsletter-ui` lives under a
base path of its own, which its `module.json` cannot know: `--base` gives it
(use the same path as `NEWSLETTER_UI_BASE`). Without `--base` that module is
listed without a link, and the script says so.

Then build and deploy the site:

```sh
ssg --config .ssg.yaml --deploy cloudflare --deploy-project my-site
```

## 5. The first owner, and their second factor

While no account exists and `AUTH_BOOTSTRAP_TOKEN` is set:

```sh
curl -X POST https://example.com/api/auth/bootstrap \
  -H 'content-type: application/json' -H 'x-ssg-auth: 1' \
  -d '{"token":"<AUTH_BOOTSTRAP_TOKEN>","email":"you@example.com","password":"<12+ characters>","name":"You"}'
```

`201` creates the account with the built-in `owner` role. From then on the
route answers `404`. Delete the token:

```sh
npx wrangler pages secret delete AUTH_BOOTSTRAP_TOKEN
```

**Enrol straight away.** Open `https://example.com/auth/login` and sign in.
Because `owner` needs a second factor, you land on **Set up a second
factor**; until you finish, your session carries none of the owner's
permissions. Choose **Set up an authenticator app**, scan the QR code (or type
the key), enter the six-digit code, and save the ten backup codes the next
page shows once. From now on the password alone opens nothing.

Passkeys work as a second factor too; passwordless sign-in with a passkey is
not built. Recovery (another owner resets the factors, or a one-time
`AUTH_MFA_RESET_TOKEN` for a sole owner) is in
[SECOND-FACTOR.md](../workers/auth/docs/SECOND-FACTOR.md#recovery).

**Password hashing and the plan.** Passwords are hashed with PBKDF2-SHA256 at
100 000 iterations, the most Cloudflare's production runtime accepts; one
derivation took about 9 ms of CPU locally, close to Workers Free's 10 ms per
request, so use Workers Paid for `auth` in production
([SECURITY.md](../workers/auth/docs/SECURITY.md#password-hashing)).

## 6. The sending panel (`newsletter-ui`)

It deploys on its own, from its directory, on the **same D1 database** as
`newsletter`:

```sh
cd workers/newsletter-ui
npx wrangler queues create ssg-newsletter-send
npx wrangler queues create ssg-newsletter-send-dlq
npx wrangler secret put NEWSLETTER_SMTP_PASSWORD   # or NEWSLETTER_RESEND_API_KEY / NEWSLETTER_MAILCHANNELS_API_KEY
npx wrangler deploy
```

Before deploying, in its own `wrangler.toml`: the `newsletter` database id in
`database_id`, your zone in `routes`, `NEWSLETTER_SITE_URL`,
`NEWSLETTER_MAIL_FROM` and the SMTP variables if you use SMTP, and the same
three `NEWSLETTER_AUTH_*` lines as in step 4 (it is a separate Worker with its
own environment). The whole list: [its README](../workers/newsletter-ui/README.md#install-from-zero).

Because it is on the same host under `/newsletter-admin`, the `auth` cookie
reaches it, and its pages load `/core/nav.js` and `/core/theme.css` from the
site root — still `'self'` in its strict CSP.

## 7. Invite people and give them roles

**Accounts admin → Invitations** (`/auth/admin/#/invitations`): address,
name, language, roles, and how it goes out. With a mail transport for `auth`
(`AUTH_MAIL_FROM` and one of `AUTH_RESEND_API_KEY`,
`AUTH_MAILCHANNELS_API_KEY`, `AUTH_SMTP_HOST`) the invitation is mailed;
without one you get a one-time link, shown once, to pass on yourself. It works
once, for 72 hours (`AUTH_INVITE_HOURS`), and only for roles you hold
yourself.

Built-in roles ([PERMISSIONS.md](../workers/auth/docs/PERMISSIONS.md#built-in-roles)):

| Role | Permissions | Second factor by default |
|---|---|---|
| `owner` | `*` | required |
| `newsletter-editor` | `newsletter:edit`, `newsletter:read`, `newsletter:send` | required (it sends mail) |
| `newsletter-viewer` | `newsletter:read` | not required |
| `comments-moderator` | `comments:moderate`, `comments:read` | not required |

The invited person opens the link, chooses a password (or a login provider,
when you have configured one), and — for a role that needs a second factor —
is taken to set one up before that role's permissions count. Change which
roles need one with `AUTH_MFA_ROLES`, e.g.
`'{"newsletter-editor": false}'`.

A `newsletter-editor` writes, tests and schedules campaigns; import, export,
erasing a person, suppressions and the Activity view need other permissions.
The panel hides what the account cannot use and names the missing permission
when asked anyway; every worker refuses the request on its own regardless.

A changed role reaches the other workers when the person's token renews: at
most `AUTH_TOKEN_MINUTES` + 30 s, **10 min 30 s** by default. To stop someone
at once, end their sessions or block the account in **People** — and know
that other workers still accept a token already issued until it expires.

## 8. The admin home and module management

`/core/` shows, for the signed-in person only, what needs attention across
the modules (comments awaiting moderation, subscribers never sent a
confirmation, a paused campaign…), a card per module with its numbers, and
the site's build and configuration problems. It asks the modules when you
open or refresh it, never on a timer.

With `CORE_DB` bound, `/core/#modules` (needs `core:manage`) sets which
modules the bar and the home page show, in which order and under which label,
with a change log and a status history; a settings file can be exported and
imported between environments. Hiding a module is cosmetic: it keeps running,
and its panel still decides who may use it.

`/core/#appearance` shows the colours the build derived from `admin_theme:`,
every colour it had to adjust and why, and the contrast of each pair. It is
read-only: change `.ssg.yaml` and rebuild.

## Putting the panels behind Cloudflare Access instead

Plain Access mode needs no `auth` at all: one Access application per panel
path, then `<PREFIX>_ACCESS_TEAM` and `<PREFIX>_ACCESS_AUD`.

| Worker | Paths behind the Access application | Must stay reachable (Bypass, include Everyone) |
|---|---|---|
| `newsletter-ui` | `newsletter-admin` (its base) | `newsletter-admin/hooks` — provider webhooks and the click redirect |
| `comments` | `/comments-admin.html`, `/api/comments/admin*`, `/api/comments/module*` | — (the public widget is not under those paths) |
| `newsletter` | `/api/newsletter/admin*` (add it to the `newsletter-ui` application, or its own) | — |
| `core` | `/api/core/*` (and `/core/*` if you like) | — |

Access applies the most specific path, which is what makes a Bypass
application for the hooks work. Steps:
[newsletter-ui](../workers/newsletter-ui/README.md#putting-the-panel-behind-cloudflare-access-zero-trust),
[comments](../workers/comments/README.md#cloudflare-access), and the
[core README](../workers/core/README.md#sign-in).

**Access as a way into `auth`** (identity from Access, permissions from
`auth`): put only `auth/access` behind an Access application and set
`AUTH_ACCESS_TEAM` + `AUTH_ACCESS_AUD`. Never put `/auth/*` or `/api/auth/*`
behind it. If the whole host is already behind Access, add Bypass
applications for at least `api/auth/jwks`, `api/auth/token` and
`api/auth/oauth` — the provider callbacks (Apple answers with a cross-site
POST that carries no Access cookie) and a script's token exchange would
otherwise get Access's login page. The two opt-ins
(`AUTH_ACCESS_LINK_BY_EMAIL`, `AUTH_ACCESS_AUTO_CREATE`) are off by default;
read their risks in [ACCESS.md](../workers/auth/docs/ACCESS.md) first.

## Costs and Cloudflare plans

Numbers from the READMEs, which checked Cloudflare's documentation on the
dates they give; plans change, so follow their links before relying on one.

- **`auth`:** Workers Paid recommended — a sign-in's password hash is close
  to the Free plan's 10 ms CPU per request (above). Pages Functions requests
  count against the Workers plan.
- **The admin home:** each module summary is at most a few D1 statements, but
  a count reads every row it scans. With 100 000 subscribers one
  `newsletter-ui` summary reads a few hundred thousand rows; Workers Free
  includes 5 million rows read a day
  ([WORKER-MODULES.md](WORKER-MODULES.md#what-a-summary-costs-on-d1)).
- **Sending:** about 7 D1 rows written per recipient; on Workers Free about
  13 000 recipients a day fit the 100 000 rows written. Beyond about 10 000
  readers, or to send in one evening, plan on Workers Paid
  ([newsletter-ui](../workers/newsletter-ui/README.md#limits-per-cloudflare-plan)).

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `503 {"error": "not_configured"}` from a panel's API | some `<PREFIX>_AUTH_*` set, not all | set the issuer, the login URL and exactly one of public key / JWKS URL |
| `503 signin_not_configured` from `/api/core/*` | no sign-in mode for `core` | `CORE_AUTH_*`, `CORE_ACCESS_*` or `CORE_ADMIN_PASSWORD` |
| `503 access_misconfigured` | only one of `*_ACCESS_TEAM` / `*_ACCESS_AUD` | set both, or neither |
| `503 registry_missing` from every `core` route | the build did not write `functions/_ssg-modules.json` (core copied by hand, or an older ssg) | keep `core` in `workers:` and build with ssg 1.9.9 |
| Every `auth` route `503 not_configured` | no `AUTH_DB` or no usable `AUTH_SIGNING_KEY` | bind the database; paste the whole private JWK |
| `503 mfa_unavailable` on the code step | `AUTH_MFA_KEY` shorter than 32 characters | set a longer value |
| Build stops: `output collision at /core/index.html` | a page of your own at `/core/` and the `core` worker's home page | give your page another `slug:`/`link:`, or leave `core` out |
| `/auth/login` is 404 | `auth`'s routes are `[/api/*]` | `routes_include: [/api/auth/*, /auth/*]`, `routes_exclude: [/auth/assets/*]` |
| A panel is missing from the bar for someone | the bar lists a module only when the account holds a permission of one of its navigation entries | grant the role; hidden in `/core/#modules`? |
| A new grant does not work yet | the token renews within 10 min 30 s | wait, or have the person sign out and in |
| A module card says *unreachable* | `core` asks modules on the same host only, without following redirects | check the module answers at its base path |

## What has not been tried against live services

From the READMEs:

- Cloudflare Access as a way into `auth` was built against a scripted fake of
  an Access team, not a real Zero Trust account
  ([ACCESS.md](../workers/auth/docs/ACCESS.md)).
- None of the five login providers was exercised against the live provider
  ([PROVIDERS.md](../workers/auth/docs/PROVIDERS.md)).
- The password hash's CPU time was measured locally; the time on
  Cloudflare's machines is unknown, and no Cloudflare account was used
  ([SECURITY.md](../workers/auth/docs/SECURITY.md#password-hashing)).
- `newsletter-ui`'s deploy, queues and routes were not exercised against a
  live account; whether Workers Free's CPU limit applies to a queue consumer
  is not verified ([its README](../workers/newsletter-ui/README.md)).
- Whether Pages Functions can open the SMTP socket `newsletter`'s
  confirmation mail needs could not be verified
  ([newsletter README](../workers/newsletter/README.md#mail-transports)).

The walkthrough this guide follows was run locally: the build output's real
functions, the `newsletter-ui` Worker and real tokens signed by `auth` and
verified by every other worker on one origin, with in-memory databases, a
pretend SMTP server and no Cloudflare account.
