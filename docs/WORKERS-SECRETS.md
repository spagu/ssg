# Worker secrets and variables

Where the secrets and variables of the worker templates are set, and which
names each template reads. Secrets never go in `.ssg.yaml`; the build only
wires the code in ([WORKERS-CONFIGURATION.md](WORKERS-CONFIGURATION.md)).
Which template does what is in [WORKERS.md](WORKERS.md).

## Where they are set

Secrets never live in `.ssg.yaml` or in the Function source — set them per Pages
project with wrangler:

```sh
wrangler pages secret put STRIPE_SECRET_KEY
wrangler pages secret put CONTACT_TURNSTILE_SECRET
```

The Function reads them from its `env` binding at runtime.

## One project, one environment: prefix your secrets

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
| `newsletter-ui` | `NEWSLETTER_ASSETS` (R2 bucket binding, optional) + `NEWSLETTER_ASSETS_URL` (var) | campaign images uploaded in the panel; the var is the `https:` address they are served from, the worker's own route when unset |
| `newsletter-ui` | `NEWSLETTER_IMAGES_REQUIRE`, `NEWSLETTER_BANNED_PHRASES`, `NEWSLETTER_BLOCKED_IMAGE_PATTERNS`, `NEWSLETTER_UTM_STRICT` (vars, optional) | rules a campaign must pass before it can be scheduled: image alt text, store and loading; banned phrases; blocked image addresses; hand-written `utm_*` |

The full list of `newsletter-ui` variables (send rate, batch size, click
counting, UTM tagging, campaign images, rules before scheduling, feed URL, …) is the "Configuration" table in
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
| `auth` | `AUTH_JWT_ISSUER` + `AUTH_JWT_AUDIENCE` with `AUTH_JWT_JWKS_URL` or `AUTH_JWT_PUBLIC_KEY` (vars), or `AUTH_JWT_SECRET` (secret, at least 32 bytes, HS256 only) | sign-in with a JWT from a login system the site already runs, exchanged at `POST /api/auth/jwt` ([EXTERNAL-JWT.md](../workers/auth/docs/EXTERNAL-JWT.md)) |
| `auth` | `AUTH_RESEND_API_KEY`, `AUTH_MAILCHANNELS_API_KEY` or `AUTH_SMTP_PASSWORD` (secrets) with `AUTH_MAIL_FROM` | invitation and reset mail; without a transport the link is shown to the administrator |

Every `auth` variable is in the "Configuration" section of
[its README](../workers/auth/README.md#configuration) and in its
`wrangler.snippet.toml`; `core`'s in [its README](../workers/core/README.md#variables).
