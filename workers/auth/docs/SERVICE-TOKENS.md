# auth — service accounts and tokens for CI, cron and scripts

A script that calls a worker — CI after a deploy, a nightly cron, your own
tool — should not hold a person's password or a key that opens everything.
`auth` gives it a **service account** with roles, like a person's, and one or
more **service tokens**: long secrets that the script exchanges for a short
access token, which the other workers check with the public key as they check
a person's.

```
script ──Bearer ssgsvc_…──▶ POST /api/auth/token ──▶ {accessToken (10 min, kind: "service")}
script ──Bearer <accessToken>──▶ /api/push/notify, /api/republish, … (checked locally)
```

## Service accounts

A service account is a row of its own (`service_accounts`), **not** a person
with a flag. Everything that lets a person in reads `users` — the login form
(by address), invitations (they create a `users` row), reset links (they point
at one), group membership (it checks one exists) — so none of them can reach
a service account. That is by construction, not by a check someone could
forget:

- it can never sign in through `/auth/login` or `POST /api/auth/login`;
- it can never be invited, sent a reset link, or have its sessions revoked
  (it has none);
- it is never in a group (groups take people);
- it **never counts as the active owner**: the owner invariant counts
  `active_owners`, which joins `users`. Granting it `owner` is possible (by a
  holder of `*`), but removing the last person's `owner` grant is still
  refused (`409 owner_required`), whatever service accounts hold.

It takes roles through the existing grants endpoint — its id as `userId`
(ids are UUIDs, one space): `POST /api/auth/admin/grants {roleId, userId}`.
`effectivePermissions` reads its grants exactly as a person's.

A service account is `active` or `disabled`. Disabling it stops every exchange
of its tokens at once.

## Service tokens

`ssgsvc_<id>_<43 characters>` — 256 random bits after a recognisable prefix,
so secret scanners (and you, in a log) can spot a leaked one. `<id>` is a
12-character public id, shown in lists as `ssgsvc_<id>_…`; it identifies the
token and keys its lockout, and is useless without the rest.

| Property | Rule |
|---|---|
| Secret | shown **once**, in the `201` answer that creates the token; stored only as its SHA-256; never listed, logged or returned again (a test scans the tables, logs and answers) |
| Expiry | required: `expiresInDays` 1 … `AUTH_SERVICE_TOKEN_MAX_DAYS` (default **365**); default **90** |
| Permissions | optional narrowing list: the token gets the **intersection** of the account's permissions and the list — never more than either |
| Sensitive | not carried unless an owner allowed them at creation (`allowSensitive`, next section) |
| Last use | `lastUsedAt`, written at most once per 5 minutes per token |
| Revocation | `DELETE …/tokens/:tokenId`; the next exchange fails |

The token's permissions are computed **at each exchange**, from the account's
grants as they are then — a removed role shows in the next access token.

### Intersection, with wildcards on both sides

| Account | Token list | Token gets |
|---|---|---|
| `newsletter:*` | `newsletter:read` | `newsletter:read` |
| `newsletter:read` | `newsletter:*` | `newsletter:read` |
| `*` | `web-push:notify` | `web-push:notify` |
| `*` | `newsletter:*` | `newsletter:*` (sensitive — see below) |
| `newsletter:read` | `newsletter:send` | nothing |

Each entry of the result is an entry of one side that the other side covers,
so the result never allows more than either (`test/service-perms.test.ts`
checks every pair against every concrete permission).

### Sensitive permissions

Defined once, in `functions/api/auth/_service-perms.ts` (`isSensitive`):

| Sensitive | Why |
|---|---|
| `*` | everything, in every worker |
| `auth:…` (`auth:admin`, `auth:invite`, `auth:*`) | who may do what — identity administration |
| `<worker>:erase` | deleting people's data |
| `<worker>:*` | covers that worker's `erase`, and any action it adds later |

A sensitive permission reaches a service token only when the token's
`allowSensitive` covers it, and **only a holder of `*` (an owner) may set
`allowSensitive`** (`403 sensitive_needs_owner` otherwise). Without it, a
script on an account holding `newsletter:*` gets nothing from the wildcard:
name the concrete permissions it needs (`permissions: ["newsletter:settings"]`)
— a wildcard account still yields those.

**Roles that need a second factor give a machine nothing by default.** A
machine never presents a factor, so whatever people may use only after one,
a service token may carry only with an owner's explicit allowance. A role
needs a factor under the policy in [SECOND-FACTOR.md](SECOND-FACTOR.md): by
default any role holding a sensitive permission or any `…:send`, adjusted per
role by `AUTH_MFA_ROLES`. Every permission such a role gives — not only the
sensitive one — reaches a token only when `allowSensitive` covers it. A
permission the account also holds through a role that needs no factor stays
free.

Two consequences worth knowing:

- **This applies whether or not the second factor is switched on**
  (`AUTH_MFA_KEY`). The switch exists so a site without the key does not lock
  its people out; turning it off must not widen what a stolen service secret
  opens. `AUTH_MFA_ROLES` still counts while it is off, and an invalid value
  fails closed: no role's permissions are free.
- **It is computed at every exchange**, from the grants as they are then. A
  token created before a role gained `newsletter:send` loses that role's
  permissions at its next exchange unless the token lists them in
  `allowSensitive`. To give a script exactly one action, grant it a narrow
  role of its own (for example only `web-push:notify`), which needs no factor.

## The exchange — `POST /api/auth/token`

```
POST /api/auth/token
Authorization: Bearer ssgsvc_<id>_<secret>
```

`200 {accessToken, tokenType: "Bearer", expiresIn, expiresAt, permissions}`.
The access token is an ES256 JWT of the contract
([CONTRACT.md](CONTRACT.md)) with `kind: "service"`, `sub` = the service
account's id, `sid` = the token's public id, `email` = `""`, and `perms` as
above. It lives **`AUTH_SERVICE_ACCESS_MINUTES`, default 10 minutes**. There
is no refresh token and no cookie: exchange again when you need a new one.

| Answer | When |
|---|---|
| `401 {error: "invalid_token"}` + `WWW-Authenticate: Bearer realm="ssg-auth", error="invalid_token"` | no header, another scheme, a malformed, unknown, revoked or expired secret, a disabled account — **one answer for all**, byte for byte, after the same single database query |
| `429 {error: "too_many_attempts"}` + `Retry-After` | the IP is locked (after `AUTH_LOCK_IP_FAILURES`, 20) — even for the right secret, as at the login. Counter `service_ip`, apart from the login's. There is no lock per token id (security review SR-08): the id is public and the 256-bit secret cannot be guessed, so such a lock only let anyone lock a working token out |
| `500 {error: "permissions_too_large", detail}` | the token would exceed the 8192 bytes the verifier accepts — narrow its list |
| `503 not_configured` | no `AUTH_DB` or signing key |

The lookup is by the SHA-256 of the presented secret, so no secret is ever
compared, and timing can only tell about a digest nobody can invert.

**No CSRF check, on purpose.** CSRF abuses a credential the browser attaches by
itself — a cookie. This route reads no cookie at all: a person's session sent
along is ignored, never used. The bearer header is never added by a browser on
its own, and a page on another origin cannot add it without a CORS preflight
that this worker never answers.

**Trade-off of the id lock.** The id is public (it is in lists), so someone who
knows it can keep that token locked by sending wrong secrets — as with a known
address at the login. The lock is capped (`AUTH_LOCK_MAX_SECONDS`, 1 hour);
the per-IP counter bounds one source. Guessing a 256-bit secret is not a
threat the lock is needed for; it exists so a leaked id plus a broken script
cannot hammer the database.

## How long a revoked or disabled token keeps working

Revoking a token or disabling its account stops the **next exchange** at once.
An access token already exchanged keeps working in other workers until it
expires — they never ask `auth`: at most **`AUTH_SERVICE_ACCESS_MINUTES` + 30 s
leeway = 10 min 30 s by default**. `test/service-exchange.test.ts` asserts
exactly that (accepted at 10:29, refused from 10:30).

## The channel rule — and why machines do not administer `auth`

Per the contract, a service token is accepted **only** as `Authorization:
Bearer`, a person's token **only** in the cookie. `auth`'s own routes apply it
too: they read the access cookie only, and a service token found there is
refused (`accessClaims`), so a service token never opens auth's admin API —
not as a cookie, not as a bearer header (`test/service-boundaries.test.ts`).

That is deliberate: administering identities is the most powerful thing on the
site, the admin API's CSRF defence assumes a browser session, and `auth:…` is
sensitive anyway. Account and role changes stay with people. If you ever need
scripted administration, that is a new, separately reviewed feature.

## Admin API

All require `auth:admin`, a person's session, and the [CSRF](SECURITY.md#csrf)
defence on non-GET routes.

| Method + path | Body | Success |
|---|---|---|
| `GET /api/auth/admin/service-accounts` | — | `{items: [serviceAccount], nextCursor: null}` |
| `POST /api/auth/admin/service-accounts` | `{name, description?}` | `201 {serviceAccount}`; `409 name_taken` |
| `GET /api/auth/admin/service-accounts/:id` | — | `{serviceAccount, permissions: [{name, known, sources}], tokens: [token]}` |
| `PUT /api/auth/admin/service-accounts/:id` | `{name?, description?, status?}` | `{serviceAccount}` |
| `DELETE /api/auth/admin/service-accounts/:id` | — | `{ok: true}` — its grants and tokens go too |
| `GET /api/auth/admin/service-accounts/:id/tokens` | — | `{items: [token], nextCursor: null}` |
| `POST /api/auth/admin/service-accounts/:id/tokens` | `{name?, expiresInDays?, permissions?, allowSensitive?}` | `201 {token, secret}` |
| `DELETE /api/auth/admin/service-accounts/:id/tokens/:tokenId` | — | `{ok: true}`; revoking twice keeps the first time |

Shapes: a **serviceAccount** is `{id, name, description, status, createdBy,
createdAt, updatedAt}` (`name` a slug `[a-z][a-z0-9-]*`, ≤ 64); a **token**
`{id, serviceAccountId, name, display, permissions (null = the account's),
allowSensitive, createdBy, createdAt, expiresAt, lastUsedAt, revokedAt,
status: "active" | "expired" | "revoked"}`.

**Escalation**: what a token could carry — its list, or without one the
account's current permissions, plus `allowSensitive` — must be covered by the
caller's own permissions (`403 escalation {permissions}`), unless the caller
holds `*`. Granting roles to the account goes through the grants endpoint and
its own escalation rule.

## A complete example

```sh
# 1. As an owner, signed in (cookies in jar.txt): the account, its role, a token.
H='-H content-type:application/json -H x-ssg-auth:1 -b jar.txt'
SA=$(curl -s $H -d '{"name":"ci-deploy"}' https://example.com/api/auth/admin/service-accounts | jq -r .serviceAccount.id)
ROLE=$(curl -s $H -d '{"name":"deploy-hooks","permissions":["web-push:notify","republish-trigger:run"]}' \
       https://example.com/api/auth/admin/roles | jq -r .role.id)
curl -s $H -d "{\"roleId\":\"$ROLE\",\"userId\":\"$SA\"}" https://example.com/api/auth/admin/grants
curl -s $H -d '{"name":"GitHub Actions","expiresInDays":90}' \
     https://example.com/api/auth/admin/service-accounts/$SA/tokens | jq -r .secret
# → ssgsvc_… — store it as the CI secret SSG_SERVICE_TOKEN; it is never shown again.

# 2. In CI: exchange, then call.
ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" https://example.com/api/auth/token | jq -r .accessToken)
curl -sf -X POST -H "authorization: Bearer $ACCESS" https://example.com/api/push/notify
```

The workers that accept service tokens today: `web-push` (`/api/push/notify`,
`web-push:notify`) and `republish-trigger` (`/api/republish`,
`republish-trigger:run`) — see "Calling this from CI" in their READMEs.

## The newsletter purge from cron

With the shared password, the cron in `workers/newsletter/README.md` runs the
retention purge with the admin password (`curl -u :"$NEWSLETTER_ADMIN_PASSWORD" …`),
which opens every admin action. With `NEWSLETTER_AUTH_*` set (the password is
then refused), the cron needs only the permission that guards the purge — `newsletter:settings` ("maintenance: test
the SMTP connection and run the retention purge now", from its `module.json`):

```sh
# once: a service account "newsletter-cron" holding a role with newsletter:settings,
# and a token narrowed to it:
curl -s $H -d '{"name":"nightly purge","permissions":["newsletter:settings"]}' \
     https://example.com/api/auth/admin/service-accounts/$SA/tokens | jq -r .secret

# the cron job:
ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" https://example.com/api/auth/token | jq -r .accessToken)
curl -sf -H "authorization: Bearer $ACCESS" -H 'content-type: application/json' \
     -d '{"action":"purge"}' https://example.com/api/newsletter/admin
```

The purge erases rows, but it is guarded by `newsletter:settings`, not
`newsletter:erase`, so the token needs no `allowSensitive`. `newsletter`
accepts the service token on the bearer channel, where its CSRF rule does not
apply.

## Configuration

| Var | Default | Meaning |
|---|---|---|
| `AUTH_SERVICE_TOKEN_MAX_DAYS` | `365` | longest expiry a service token may be given |
| `AUTH_SERVICE_ACCESS_MINUTES` | `10` | lifetime of the access token from `POST /api/auth/token` |

Lock thresholds are the login's (`AUTH_LOCK_*`), with separate counters.
