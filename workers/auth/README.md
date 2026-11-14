# auth — accounts, sessions and signed access tokens

![Pages Functions](https://img.shields.io/badge/Cloudflare-Pages%20Functions-F38020)
![D1](https://img.shields.io/badge/storage-D1-F38020)
![Dependencies](https://img.shields.io/badge/npm%20dependencies-0-1a7f37)
![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)

An optional Pages Functions template that gives a site real accounts: an
e-mail address and a password per person, sessions you can end, and a short
access token signed with a private key. Other workers on the same host check
that token with the public key alone, without calling this worker on every
request.

> **The second factor is off until you set `AUTH_MFA_KEY`.** Without that
> secret an owner signs in with a password (or a provider) alone. Set it
> before you invite anyone, and enrol every owner at once — an owner with no
> factor yet can enrol on a first-factor sign-in, so whoever holds that
> password could enrol first ([second factor](docs/SECOND-FACTOR.md),
> [security review](docs/SECURITY-REVIEW.md)).

## Do you need it?

Probably not, if one person runs one panel. The workers that have a panel
(`comments`, `newsletter`, `newsletter-ui`, `core`) already work with **one shared
password** or with **Cloudflare Access**, and both stay fully supported:

| Situation | Choose |
|---|---|
| One person, one panel | the worker's own password — a secret, nothing else to run |
| You already use Cloudflare Zero Trust | Cloudflare Access — your identity provider, no passwords stored here |
| Several people, several panels, or different rights per person | `auth` |

`auth` is the third option, and nothing switches it on by itself: a worker
uses it only when you set its `<PREFIX>_AUTH_*` variables. Which mode suits
you, and a walk-through from one password to a team:
[docs/WORKERS.md](../../docs/WORKERS.md#admin-sign-in-which-mode-to-choose),
[docs/ADMIN-GUIDE.md](../../docs/ADMIN-GUIDE.md).

## What it contains — and what it does not

Built: accounts with passwords, login (JSON and plain HTML form), refresh with
rotation and theft detection, logout and "log out everywhere", password change,
the first administrator, the signing keys and their rotation, the public key
set (JWKS); roles, groups and grants, the permissions inside the token, and an
admin API to manage them ([permissions](docs/PERMISSIONS.md)); the sign-in,
"Your panels" and account pages, invitations — the only way to a new account —
and password reset links, by mail when you configure a transport
([pages](docs/PAGES.md)); service accounts and tokens for CI, cron and scripts
([service tokens](docs/SERVICE-TOKENS.md)); sign-in through login providers —
Google, Microsoft, Apple, GitHub, Facebook — each switched on by its own
variables, chosen by each person next to (or instead of) the password, for
invited people only ([providers](docs/PROVIDERS.md)); Cloudflare Access as one
more way to sign in — identity from Access, permissions from `auth` — with
optional group mapping ([Access](docs/ACCESS.md)); a second factor —
authenticator apps (TOTP), one-time backup codes and passkeys — switched on by
one secret (`AUTH_MFA_KEY`), required by default for `owner` and any role that
can erase, send or administer, with step-up before sensitive changes and a
recovery path ([second factor](docs/SECOND-FACTOR.md)); the **accounts admin
panel** at `/auth/admin/` — people, invitations, groups, roles, an access
matrix, sessions, service accounts, Access mappings, the audit log and the
worker's status, in five languages ([panel](docs/PANEL.md)).

Other workers accept these sessions through one file they copy, the
[portable verifier](#accepting-these-sessions-in-another-worker): `comments`,
`newsletter`, `newsletter-ui` and `core` for people, `web-push` and
`republish-trigger` for service tokens.

**Not built** (no dates promised): passwordless sign-in with a passkey alone
(a passkey is a second factor here), sign-in for the site's visitors, and one
session across several hosts.

## Install from zero

```sh
ssg new worker auth                       # copies the template into workers/auth/
npx wrangler d1 create ssg-auth           # paste the id into wrangler.toml (see wrangler.snippet.toml)
node workers/auth/scripts/auth-keys.mjs   # prints AUTH_SIGNING_KEY and the public key
npx wrangler pages secret put AUTH_SIGNING_KEY       # paste the JSON after "AUTH_SIGNING_KEY="
npx wrangler pages secret put AUTH_BOOTSTRAP_TOKEN   # any long random string, e.g. `openssl rand -base64 32`
```

Add the worker to `.ssg.yaml` and deploy:

```yaml
workers:
  - dir: workers/auth
    routes_include: [/api/auth/*, /auth/*]   # the API and the pages
    routes_exclude: [/auth/assets/*]         # the pages' stylesheet and script are plain files
```
 The tables are created on first use; `schema.sql` holds the same
schema for applying by hand.

### The first administrator

While the `users` table is empty and `AUTH_BOOTSTRAP_TOKEN` is set:

```sh
curl -X POST https://example.com/api/auth/bootstrap \
  -H 'content-type: application/json' -H 'x-ssg-auth: 1' \
  -d '{"token":"<AUTH_BOOTSTRAP_TOKEN>","email":"you@example.com","password":"<12+ characters>","name":"You"}'
```

`201` creates the account, makes it the **owner** (the built-in role holding
`*`) and logs it in. From then on the route answers `404`
whatever you send — the condition "no account exists yet" is part of the same
SQL statement that inserts the row, so two simultaneous calls cannot both
succeed. Delete the secret afterwards (`wrangler pages secret delete
AUTH_BOOTSTRAP_TOKEN`). Wrong tokens count towards a per-IP lock (5, then the
growing delay in [SECURITY.md](docs/SECURITY.md#lockouts-turnstile-csrf)).

### Signing in, and inviting the others

Open **`/auth/login`**. Without `next` it leads to **`/auth/`** ("Your
panels"), and **`/auth/account`** changes the password and ends sessions — all
plain forms that work without JavaScript, in English, Polish, Hindi, German
and French ([PAGES.md](docs/PAGES.md)). Every other account comes from an
invitation:

```sh
curl -X POST https://example.com/api/auth/admin/invitations \
  -H 'content-type: application/json' -H 'x-ssg-auth: 1' -H 'cookie: __Host-ssg-auth=<your access token>' \
  -d '{"email":"eva@example.com","name":"Eva","roleIds":["<newsletter-editor role id>"],"lang":"de"}'
```

With a mail transport (`AUTH_MAIL_FROM` and one of `AUTH_RESEND_API_KEY`,
`AUTH_MAILCHANNELS_API_KEY`, `AUTH_SMTP_HOST` — [Mail](docs/PAGES.md#mail)) Eva
gets a mail in German; without one the answer carries the one-time `link` to
pass on yourself. She opens it, chooses her password and is signed in with the
roles you picked — never more than you hold yourself. A forgotten password:
`POST /api/auth/admin/users/<id>/reset-link` gives a one-time link (by mail
when configured); with mail, the login page also offers "Forgot your
password?".

To let people sign in with Google, Microsoft, Apple, GitHub or Facebook
instead of (or as well as) a password, register an app at the provider, paste
`https://<your host>/api/auth/oauth/<provider>/callback` as its redirect URI
and set two variables — step by step in [PROVIDERS.md](docs/PROVIDERS.md). It
is still invitation only: a provider account opens nothing by itself.

Already on Cloudflare Zero Trust? Put `/auth/access` (only that path) behind
an Access application and set `AUTH_ACCESS_TEAM` and `AUTH_ACCESS_AUD`: the
login page gains "Sign in with Cloudflare Access" — step by step, with the
two opt-ins and their risks, in [ACCESS.md](docs/ACCESS.md).

## Reference

| Document | What is in it |
|---|---|
| [docs/API.md](docs/API.md) | every route: sessions, invitations, reset links, own sessions, the admin API |
| [docs/PANEL.md](docs/PANEL.md) | the accounts admin panel: each view, the permission it needs, step-up, the audit log and its retention, look and colours, the preview and its checks |
| [docs/PAGES.md](docs/PAGES.md) | the pages, `next`, languages, look and accessibility, mail, the local preview |
| [docs/PERMISSIONS.md](docs/PERMISSIONS.md) | roles, groups, grants, the owner invariant, freshness, the token ceiling, the registry |
| [docs/SECURITY.md](docs/SECURITY.md) | password hashing and its measurement, lockouts, Turnstile, CSRF, login providers, one-time links |
| [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) | assets, attackers, trust boundaries, what is out of scope (and how same-origin XSS meets these cookies) |
| [docs/SECURITY-REVIEW.md](docs/SECURITY-REVIEW.md) | the 1.9.9 security review: every attempt, every finding with its severity, fix and regression test |
| [docs/PROVIDERS.md](docs/PROVIDERS.md) | signing in with Google, Microsoft, Apple, GitHub or Facebook: registering the app at each, the redirect URI, variables, quirks; who gets in; narrowing by domain and per role |
| [docs/ACCESS.md](docs/ACCESS.md) | Cloudflare Access as a sign-in method: which path goes behind Access, the bypass for the public parts, who gets in, linking by address and auto-provisioning (off, and why), group mapping, sign-out |
| [docs/CONTRACT.md](docs/CONTRACT.md) | what another worker needs to accept these sessions |
| [docs/SECOND-FACTOR.md](docs/SECOND-FACTOR.md) | the second factor: authenticator apps, backup codes, passkeys; which roles need it (`AUTH_MFA_ROLES`); step-up; recovery and break-glass; `AUTH_MFA_KEY` and its rotation |
| [docs/SERVICE-TOKENS.md](docs/SERVICE-TOKENS.md) | service accounts and tokens for CI, cron and scripts; `POST /api/auth/token`; the newsletter purge from cron |

## Tokens and cookies

| Cookie | Holds | Attributes |
|---|---|---|
| `__Host-ssg-auth` | the access token (JWT) | `Path=/; Secure; HttpOnly; SameSite=Lax`, lives `AUTH_TOKEN_MINUTES` |
| `__Secure-ssg-auth-refresh` | the refresh token (32 random bytes) | `Path=/api/auth/; Secure; HttpOnly; SameSite=Strict`, lives until the session ends |

The refresh cookie is scoped to `/api/auth/` so no other code on the host ever
receives it. The `__Host-` prefix cannot be used for it — browsers accept a
`__Host-` cookie only with `Path=/` — so it uses `__Secure-` (Secure required)
and no `Domain` (host-only).

The access token is a JWS signed with **ES256** (ECDSA P-256), header
`{alg: "ES256", typ: "JWT", kid}`, claims:

| Claim | Value |
|---|---|
| `iss` | `AUTH_ISSUER`, default `https://<host>/api/auth` |
| `aud` | `AUTH_AUDIENCE`, default the site host (one string) |
| `sub`, `sid` | the user id and the session id |
| `email` | the account's address |
| `iat`, `exp` | issued / expires, Unix seconds |
| `ver` | `1` — the contract version; an incompatible change is a new number |
| `perms` | the account's effective permissions, sorted — see [Permissions](docs/PERMISSIONS.md) |
| `mfa_at` | optional: when the session last passed a second factor ([second factor](docs/SECOND-FACTOR.md)); verifiers ignore it |

A verifier must take the algorithm from its own code (never from the header),
pick the key by `kid` among the keys it trusts, and check `ver`, `iss`, `aud`,
`exp` and `iat` (30 s leeway). `functions/api/auth/_jwt.ts` (`verifyToken`) is
the reference implementation.

### Accepting these sessions in another worker

Other workers do not reimplement any of this: they copy one import-free file,
[`portable/session-verifier.ts`](portable/session-verifier.ts)
(`verifySession`, `can`, `configFrom`, `denied`, `forbidden`), configure
`<PREFIX>_AUTH_ISSUER`, `_AUTH_PUBLIC_KEY` (or `_AUTH_JWKS_URL`) and
`_AUTH_LOGIN_URL`, and guard the copy with
[`portable/copy-guard.ts`](portable/copy-guard.ts). The token format, cookie
rules, key rotation from a consumer's side, every refusal reason and its HTTP
answer, the order of admin modes (Access → auth → password), what may change
without a new contract version, and a complete example handler are in
**[docs/CONTRACT.md](docs/CONTRACT.md)**.

**What the lifetime means for revoking access.** Logging out, "log out
everywhere", a password change or blocking an account end the session in D1
at once: this worker refuses its tokens immediately, and the next refresh
fails. A worker that verifies the token locally with the public key does not
ask D1, so it accepts an already-issued token until it expires — at most
`AUTH_TOKEN_MINUTES` (default **10 minutes**). Shorter means faster revocation
and more refreshes.

### Refresh rotation and the two-tab race

Every refresh replaces the refresh token; only SHA-256 hashes are stored, and
every rotated-away hash is kept with its successor until the session ends.

- The **current** token rotates.
- The **immediately previous** token, presented within
  `AUTH_REFRESH_GRACE_SECONDS` (default 30) of its rotation, is two tabs
  refreshing at once: it gets a new access token but **no** new refresh token
  (the browser already received the successor in the other tab's answer). The
  swap is one conditional `UPDATE`, so of two truly simultaneous requests one
  rotates and the other takes this path.
- **Any other rotated token** means someone holds a copy of an old token: the
  whole session is revoked (`revoke_reason = refresh_reuse`) — the thief and the
  real user are both logged out.
- An **unknown** token is refused without revoking anything, so guessing cannot
  log anyone out.

The grace window is the trade-off: a stolen token replayed within seconds of
the real rotation earns one access token instead of revoking the session.

## Signing keys

`scripts/auth-keys.mjs` prints a P-256 key pair: `AUTH_SIGNING_KEY` is the
private key as a JWK (`{"kty":"EC","crv":"P-256","x","y","d"}`) — JWK rather
than PKCS#8 because it carries the public half, so one secret is enough. The key
id is the RFC 7638 thumbprint, computed by the worker; at start the worker signs
and verifies a probe message, so a key whose halves do not match is refused
(`503`) instead of signing tokens nobody can verify. `GET /api/auth/jwks` lists
public members only.

### Rotating the key

1. Run `node scripts/auth-keys.mjs` for a new pair.
2. Set the **old** public key (the `AUTH_PUBLIC_KEY=` line printed when it was
   made, or the entry from `/api/auth/jwks`) as the var
   `AUTH_PREVIOUS_PUBLIC_KEY`, and the new private key as the secret
   `AUTH_SIGNING_KEY`. Deploy. New tokens are signed with the new key; both
   public keys are published, so tokens signed with the old one keep verifying.
3. After `AUTH_TOKEN_MINUTES` plus the JWKS cache time (5 minutes) — or the
   longest time a verifier caches the JWKS — remove `AUTH_PREVIOUS_PUBLIC_KEY`
   and deploy. Old-key tokens are now refused.

Rotate at least once a year, and at once when the private key may have
leaked. The panel's Status view shows each key's age (counted from the first
token this worker signed with it — a JWK carries no date) and flags a key past
365 days; a new key id in use is a `signing_key_seen` line in the audit log.

Sessions survive a rotation (refresh tokens are not signed). The failed-login
counters, keyed with a secret derived from the signing key when
`AUTH_HASH_SALT` is not set, start again.

## Configuration

Secrets (`wrangler pages secret put`):

| Name | Purpose |
|---|---|
| `AUTH_SIGNING_KEY` | **required** — the private JWK from `scripts/auth-keys.mjs` |
| `AUTH_BOOTSTRAP_TOKEN` | creates the first account; delete it afterwards |
| `AUTH_HASH_SALT` | salts the IP / User-Agent hashes stored with sessions (without it, none are stored) and the lock keys |
| `AUTH_TURNSTILE_SECRET` | Turnstile after repeated failures (`TURNSTILE_SECRET` is the fallback) |
| `AUTH_GOOGLE_CLIENT_SECRET` / `AUTH_MICROSOFT_CLIENT_SECRET` / `AUTH_GITHUB_CLIENT_SECRET` / `AUTH_FACEBOOK_CLIENT_SECRET` | a login provider's client secret ([providers](docs/PROVIDERS.md)) |
| `AUTH_APPLE_PRIVATE_KEY` | Sign in with Apple: the `.p8` key's text |
| `AUTH_MFA_KEY` | switches the second factor on and encrypts the authenticator secrets (at least 32 characters) — [second factor](docs/SECOND-FACTOR.md) |
| `AUTH_MFA_KEY_PREVIOUS` | the previous `AUTH_MFA_KEY` during a rotation |
| `AUTH_MFA_RESET_TOKEN` | one-time break-glass for a sole owner who lost every factor (at least 32 characters); delete it afterwards |
| `AUTH_RESEND_API_KEY` / `AUTH_MAILCHANNELS_API_KEY` / `AUTH_SMTP_PASSWORD` | optional mail transport for invitations and reset links ([Mail](docs/PAGES.md#mail)) |

Vars (all optional):

| Name | Default | Meaning |
|---|---|---|
| `AUTH_PREVIOUS_PUBLIC_KEY` | — | the retired public JWK during a key rotation |
| `AUTH_ISSUER` | `https://<host>/api/auth` | token `iss` |
| `AUTH_AUDIENCE` | the host | token `aud` |
| `AUTH_TOKEN_MINUTES` | `10` | access token lifetime |
| `AUTH_SESSION_DAYS` | `14` | session (refresh token) lifetime, from login |
| `AUTH_REFRESH_GRACE_SECONDS` | `30` | two-tab refresh window |
| `AUTH_PBKDF2_ITERATIONS` | `100000` | password hash cost (minimum 10,000) |
| `AUTH_PASSWORD_MIN` / `AUTH_PASSWORD_MAX` | `12` / `256` | password length, in characters |
| `AUTH_LOCK_ACCOUNT_FAILURES` / `AUTH_LOCK_IP_FAILURES` | `5` / `20` | failures before a lock |
| `AUTH_LOCK_BASE_SECONDS` / `AUTH_LOCK_MAX_SECONDS` | `30` / `3600` | first and longest lock |
| `AUTH_TURNSTILE_AFTER` | `3` | failures before Turnstile is required |
| `AUTH_TURNSTILE_SITE_KEY` | — | with the secret: the login page shows the Turnstile widget (its CSP then allows `challenges.cloudflare.com` — [SECURITY](docs/SECURITY.md#lockouts-turnstile-csrf)) |
| `AUTH_GOOGLE_CLIENT_ID` / `AUTH_MICROSOFT_CLIENT_ID` / `AUTH_GITHUB_CLIENT_ID` / `AUTH_FACEBOOK_CLIENT_ID` | — | a login provider's client id; with its secret the provider is on |
| `AUTH_APPLE_CLIENT_ID` / `AUTH_APPLE_TEAM_ID` / `AUTH_APPLE_KEY_ID` | — | Sign in with Apple: Services ID, team, key |
| `AUTH_MICROSOFT_TENANT` | `common` | `organizations`, `consumers` or one tenant id |
| `AUTH_FACEBOOK_API_VERSION` | `v24.0` | Graph API version |
| `AUTH_<PROVIDER>_ALLOWED_DOMAINS` | — | only verified addresses in these domains, for that provider |
| `AUTH_PASSWORD_LOGIN` | on | `off` removes the password as a way in (one method must remain) |
| `AUTH_ROLE_METHODS` | — | JSON `{"<role>": ["<method>", …]}`: a role counts only in sessions started with one of them |
| `AUTH_RECENT_SIGNIN_MINUTES` | `10` | how fresh a sign-in must be to change sign-in methods |
| `AUTH_MFA_ROLES` | — | JSON `{"<role>": true\|false}`: override which roles need a second factor (default: `owner` and every role that can send, erase or administer) |
| `AUTH_STEP_UP_MINUTES` | `10` | how recent a second factor must be for a sensitive change |
| `AUTH_ACCESS_TEAM` / `AUTH_ACCESS_AUD` | — | both: Cloudflare Access is a way to sign in ([Access](docs/ACCESS.md)); one alone is a configuration error (`503`) |
| `AUTH_ACCESS_LINK_BY_EMAIL` | `off` | `on`: an unlinked Access identity enters the existing account with its address (read the risk first) |
| `AUTH_ACCESS_AUTO_CREATE` / `AUTH_ACCESS_DEFAULT_ROLE` | `off` / — | `on`: an unknown Access identity gets an account with that one role (never owner, a wildcard or a sensitive permission) |
| `AUTH_ACCESS_ALLOWED_DOMAINS` | — | only addresses in these domains, for Access |
| `AUTH_LOGIN_URL` | `/auth/login` | where a failed form login is sent back |
| `AUTH_AFTER_LOGIN_URL` | `/` | where a form login goes without `next` |
| `AUTH_KNOWN_PERMISSIONS` | — | JSON array of modules and the permissions they declare ([registry](docs/PERMISSIONS.md#the-registry-of-known-permissions)); `"panel"` and `"title"` on an entry give "Your panels" a link |
| `AUTH_INVITE_HOURS` | `72` | invitation link lifetime |
| `AUTH_RESET_MINUTES` | `60` | password reset link lifetime |
| `AUTH_MAIL_FROM` / `AUTH_MAIL_FROM_NAME` | — | sender of invitations and reset links (required with a transport) |
| `AUTH_SMTP_HOST` / `AUTH_SMTP_PORT` / `AUTH_SMTP_USER` / `AUTH_SMTP_HELO` | — / `587` | your own mail server |
| `AUTH_SERVICE_TOKEN_MAX_DAYS` | `365` | longest expiry of a service token ([service tokens](docs/SERVICE-TOKENS.md)); the default is 90 days |
| `AUTH_SERVICE_ACCESS_MINUTES` | `10` | lifetime of the access token `POST /api/auth/token` gives a script |
| `AUTH_AUDIT_RETENTION_DAYS` | `365` | how long the audit log (`auth_events`) keeps an event; older ones are deleted as new ones are written ([panel](docs/PANEL.md#audit-log)) |

The binding is `AUTH_DB` (D1).

## Data

`users`, `sessions`, `refresh_history`, `login_attempts`, `schema_meta`
(version 1), `roles`, `role_permissions`, `groups`, `group_members`, `grants`
and the views `owner_holders`, `active_owners` (version 2), `invitations` and
`password_resets` (invitations and reset links), `service_accounts` and
`service_tokens` (service tokens), `identities`, `oauth_flows`,
`session_methods` and `auth_events` (login providers and the audit log),
`access_group_map`, `access_synced_members` and `access_seen_groups`
(Cloudflare Access), `mfa_factors`, `mfa_backup_codes`, `mfa_pending`,
`mfa_challenges` and `mfa_reset_tokens` (the second factor; new tables need no
version) — see
`schema.sql`. Nothing is stored in clear that could log someone in: passwords
are PBKDF2 hashes, refresh tokens, one-time link tokens and service-token secrets SHA-256 hashes, IP and User-Agent salted
hashes (only with `AUTH_HASH_SALT`). A test runs a full flow and scans every
table, every answer body and header, and every log line for the private key,
the passwords, the bootstrap token and every refresh token issued.

## Tests

```sh
node --import ./workers/auth/test/resolve.mjs --test 'workers/auth/test/*.test.ts'
```

Node 23.6 or later; real SQL through `node:sqlite`, a pinned clock, no npm
packages. The permission tests cover the `can` table, effective permissions
and provenance on a seeded database, the owner invariant on every path
(concurrent ones included), the token ceiling, refresh picking up changed
grants, revoke-sessions, the escalation rule, and a route table generated from
the files under `functions/` (no session → 401, without the permission → 403,
with it → 2xx, no CSRF header → 403). The invitation and page tests cover the invitation
lifecycle (mail or link, once, two acceptances at once, expired, withdrawn,
sent again, escalation, a role deleted meanwhile), reset links (once, expiry,
sessions ended, lock cleared), "forgot password" (identical answers, limits,
absent without mail), own sessions, `next` against a table of hostile values,
every page in five languages, strict-CSP HTML and `autocomplete`, a scan for
one-time tokens in tables, logs and headers, the mail copies, and their own
route table (`go134-routes.test.ts`). The service-token tests (`service-*.test.ts`)
cover the exchange (one answer for every failure, expiry, revocation and the
10 min 30 s an issued token outlives it, lockouts, cookies ignored), the
intersection table with wildcards, sensitive permissions, escalation, what a
service account can never be (a login, an invitation, a reset, a group member,
an owner), the channel rule on auth's own routes, a scan for secrets, their
own route table, and an end-to-end run through `web-push` and
`republish-trigger`. The provider tests run every provider against a scripted
fake of its endpoints (`test/providers-fake.ts`): success; wrong, replayed,
expired and missing `state`; a wrong `nonce`; a code used twice; a redirect URI
that does not match; cancelled consent; ID tokens with another `aud` or `iss`,
expired, from the future, with an unknown key, `alg: none`, HS256 with the
public key; endpoints that redirect elsewhere, answer too much or too slowly;
the takeover attempts (an unverified or verified address equal to an existing
account's, an invitation accepted with another address); no invitation, no
users row; the last-method invariant under concurrent removals; the per-role
method policy; both Turnstile CSP variants; a scan for secrets and provider
tokens; their own route table. `test/dev-server.mjs` and
`test/auth-shots.mjs` preview the pages ([PAGES.md](docs/PAGES.md#previews-and-checks)).
The admin panel tests (`admin-*.test.ts`) cover its routes through
their own route table, the owner invariant, escalation and step-up on every
sensitive change, the audit log (filters, paging, CSV formula guard,
retention), the panel's logic modules, its strings in five languages, its
colour contrast in both themes and its shell (CSP, meta tags, files);
`test/admin-shots.mjs` and `test/admin-walk.mjs` drive it in a browser
([PANEL.md](docs/PANEL.md#previews-and-checks)).
