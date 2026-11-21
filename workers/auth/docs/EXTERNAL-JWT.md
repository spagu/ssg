# auth — signing in with a JWT from an existing login system

> **Tested against a scripted issuer only.** The exchange was built and
> tested against a fake issuer in `test/external-fake.ts`: its JWKS endpoint,
> and tokens signed with RS256, PS256, ES256/384/512, EdDSA and HS256/384/512,
> or forged. It has not been run against any particular product. Before you
> rely on it, decode one real token from your system and compare its claims
> with the checks below.

A site that already runs its own login (its own users, roles and token
issuer) can let those people into the `auth` panels without a second set of
passwords. The existing system signs a short JWT for the person and sends it
once to `POST /api/auth/jwt`. `auth` verifies the token and turns it into a
normal `auth` session with the same cookies, the same permissions model and
the same revocation as any other sign-in. The issuer says who the person is,
and `auth` decides what they may do, either from its own roles or from the
token's roles claim through a mapping you write.

Nothing changes until you configure it. With no `AUTH_JWT_*` variable set the
route answers `404`.

**Not implemented:** RFC 8693 token exchange (the `grant_type`,
`subject_token_type` and `actor_token` form). This is one simpler exchange,
from a token to a cookie session. There is also no account-page button to link
an external identity to an existing account: linking happens at sign-in, as
described under [Who gets in](#who-gets-in).

## How it works

1. The person signs in at your existing system.
2. Your system signs a JWT for them (claims below) and gets it to the
   browser, typically with an auto-submitting form that posts `token` (and
   optionally `next`) to `https://example.com/api/auth/jwt`.
3. `auth` checks the token (below), finds or creates the account, applies the
   role mapping, and starts the session through the same entry every sign-in
   uses. If the account's roles need a second factor, the person gets the
   challenge (`/auth/mfa`), not a session. The issuer is never assumed to
   have checked a second factor.
4. The browser lands on `next` (a path on this host only; anything else
   falls back to `AUTH_AFTER_LOGIN_URL`) with the usual `__Host-` cookies.

The token is single use. Its `jti` is recorded until the token could no
longer pass anyway (its expiry, or its age limit, plus the clock skew), and a
second presentation is refused.

## The token

Your system must send:

| Claim | Rule |
|---|---|
| `iss` | exactly `AUTH_JWT_ISSUER` (string compare, no normalisation) |
| `aud` | `AUTH_JWT_AUDIENCE`, or an array that contains it |
| `exp` | **required**, in the future (60 s skew) |
| `iat` | **required**, not in the future, and no older than `AUTH_JWT_MAX_AGE_SECONDS` (default 300) |
| `nbf` | optional; if present, passed |
| `jti` | **required**, unique per token, at most 255 characters |
| `sub` | the person's stable id at your system (with `AUTH_JWT_IDENTITY_CLAIM=sub`, the default) |
| `email` | the address, needed to accept an invitation or create an account, and as the identity with `AUTH_JWT_IDENTITY_CLAIM=email` |
| `email_verified` | `true` (or `"true"`) for the address to count, unless `AUTH_JWT_EMAIL_VERIFIED=off` |
| roles claim | optional, `AUTH_JWT_ROLES_CLAIM` (default `roles`): an array of names or one space-separated string |

The header's `alg` must be on `AUTH_JWT_ALGORITHMS`. `alg: none` is never
accepted. A token with `crit` is refused. The key comes from the
configuration only. The token's `jku`, `x5u`, `jwk` and `x5c` headers are
never read, and its `kid` only chooses among the keys you configured.

A token your system can mint, as a JSON payload:

```json
{
  "iss": "https://id.example.com",
  "aud": "https://example.com",
  "sub": "user-1001",
  "email": "ada@example.com",
  "email_verified": true,
  "roles": ["editor"],
  "iat": 1795000000,
  "exp": 1795000120,
  "jti": "5f0c7d1e-2b7a-4c55-9e44-0c1f6a3e9b10"
}
```

## Presenting it

`POST /api/auth/jwt`, with the token in **one** of:

- `Authorization: Bearer <token>`: a `fetch` from a page on this host, which
  must also send `x-ssg-auth: 1` like every script call to `auth`;
- a form or JSON field `token`.

The optional field `next` is where to go afterwards (a path on this host).

The CSRF defence of every `auth` POST applies. A script needs `x-ssg-auth: 1`
from this origin. A plain form must come from this origin, **or** from an
origin listed in `AUTH_JWT_FORM_ORIGINS`, which is how the existing system's
own page posts the token across origins. Any other origin gets `403`. Without
this rule, a page anywhere could post a token for the attacker's own account
and sign the visitor into it (login CSRF).

The auto-submitting form on your system's page (`https://id.example.com`,
listed in `AUTH_JWT_FORM_ORIGINS`):

```html
<form method="post" action="https://example.com/api/auth/jwt">
  <input type="hidden" name="token" value="eyJ…">
  <input type="hidden" name="next" value="/editor/">
  <noscript><button type="submit">Continue</button></noscript>
</form>
<script>document.forms[0].submit();</script>
```

If that page has a strict Content-Security-Policy, its `form-action` must
allow `https://example.com`.

Answers: `303` to `next` with the session cookies on success; `303` to the
login page with `?error=<code>` on any refusal of the token or the person;
`403` from the CSRF check; `404` when not configured; `503
jwt_misconfigured {hint}` when configured wrongly. Failed tokens count
against a per-IP lockout of their own (`AUTH_LOCK_IP_FAILURES`, like the
password's IP counter, kept apart from it).

## Who gets in

The identity is the pair (`jwt:<issuer>`, subject), stored like a login
provider's. In order:

1. `AUTH_JWT_ALLOWED_DOMAINS`, when set: only verified addresses in those
   domains.
2. The identity is already linked: that account, if it is active. A blocked
   account is refused whatever the token says.
3. Not linked, and the token has no usable address (missing, unverified, or
   outside printable ASCII): refused, `jwt_email_unusable`.
4. An existing account has exactly this address:
   - with `AUTH_JWT_IDENTITY_CLAIM=sub` (default): refused, `jwt_not_linked`.
     An address alone never takes over an existing account;
   - with `AUTH_JWT_IDENTITY_CLAIM=email`: the address **is** the identity
     you chose, so it is linked to that account and the person enters
     (audited as a link `via: "email"`).
5. `AUTH_JWT_UNKNOWN`:
   - `reject` (default): refused, `not_invited`, and no account is written;
   - `invite`: an open invitation for the address is accepted, with its roles
     and groups, and the identity linked; with no invitation, `not_invited`;
   - `create`: an open invitation first, otherwise a new account **with no
     role**. Permissions then come only from the role mapping, or from an
     administrator.

Use `email` as the identity claim only when your system guarantees that an
address belongs to one person and is verified, and never reassigns it. If
your system lets people choose or change an unverified address, keep `sub`.

## Roles and groups from the token

Without `AUTH_JWT_ROLE_MAP`, the roles claim is ignored and the
administrator gives roles, as for any account.

With it, every sign-in makes the person's **synchronised** grants and
memberships match the token:

```json
{
  "editor": { "roles": ["newsletter-editor"], "groups": ["Editors"] },
  "viewer": { "roles": ["newsletter-viewer"] }
}
```

- a mapped external role gives its roles and groups before the session
  starts, so the first token already carries them;
- one the token no longer carries takes back what the mapping gave, and only
  that. A grant or membership an administrator made by hand is never touched;
- a token without the claim, or with nothing that matches, gets nothing from
  the mapping;
- names that are not roles or groups here are skipped and logged;
- the `owner` role cannot be mapped (a configuration error), and a mapped
  group that holds the owner role is skipped and logged.

A mapping that reaches a role holding an administrative permission (`*`,
any `auth:…` such as `auth:admin`, an erase, a wildcard, or a permission a
module declares sensitive), named directly or granted to a mapped group,
**stops the method**: every exchange answers `503 jwt_misconfigured` with a
hint naming the roles, and nobody signs in this way until it is fixed. The
check reads the roles from the database before each exchange, so a role
that gains such a permission later stops the method too. Names that are not
roles or groups here do not trigger it.

To map such roles on purpose, set `AUTH_JWT_ALLOW_ADMIN_ROLES=on`. Do it
only if you trust the issuer with administration of this site: anyone who
can make your system sign a token with that role gets it here. Even then the
owner role is never given, and a role that needs a second factor still needs
one (see [SECOND-FACTOR.md](SECOND-FACTOR.md)).

Each change is an audit event `roles_synced` (`rolesAdded`, `rolesRemoved`,
`groupsAdded`, `groupsRemoved`, `keptForOwner`).

## Keys and algorithms

Set **exactly one** key source:

| Source | Variable | Notes |
|---|---|---|
| JWK Set URL (RFC 7517) | `AUTH_JWT_JWKS_URL` | `https://` only, no credentials in the URL; fetched from that host only, no redirects, 10 s and 256 KiB limits; cached an hour; an unknown `kid` fetches again at most once a minute; when the host is down the last good set works for up to a day, and with none the sign-in is refused (`jwt_unavailable`) |
| Static public key | `AUTH_JWT_PUBLIC_KEY` | one JWK, a JWK Set (`{"keys": […]}`), or a PEM `-----BEGIN PUBLIC KEY-----` (newlines may be written as `\n`) |
| Shared secret | `AUTH_JWT_SECRET` (a **secret**, `wrangler pages secret put`) | at least 32 bytes; only with `HS256`/`HS384`/`HS512` |

`AUTH_JWT_ALGORITHMS` defaults to every asymmetric algorithm this worker
verifies: `RS256 RS384 RS512 PS256 PS384 PS512 ES256 ES384 ES512 EdDSA`
(EdDSA = Ed25519). Narrow it to what your system uses. A key is used only for
its own family (`kty`, `crv`), only when its `alg` and `use`, if present,
agree, and RSA keys must be at least 2048 bits. With several keys the token
must name one by `kid`. A key without a `kid` is used only when it is the
only one.

### The HS256 trade-off

Many existing systems sign with a shared secret, so HS256 is accepted, but
only when you opt in explicitly. You must list it in `AUTH_JWT_ALGORITHMS`
**and** set `AUTH_JWT_SECRET`. What that costs:

- anyone who holds the secret can mint tokens for **anyone**, including the
  roles in your mapping. With a public key, only the issuer's private key can.
  This `auth` deployment then holds a secret that can sign tokens, where with
  a public key it holds only something that can verify them;
- the secret must be long and random (`openssl rand -base64 48`), never
  shared with anything but the issuer, and rotated by replacing it on both
  sides at once (there is no "previous secret" here).

Mixing HS* with a public-key source, or a secret with public-key algorithms,
is refused when the configuration is read. That mix is the classic algorithm
confusion attack, where the public key is used as an HMAC secret (RFC 8725
§2.1, §3.1). Prefer an asymmetric algorithm if your system can do it.

## Sessions afterwards

The session is an ordinary one. It appears in the person's sessions and in
the admin panel. It is revocable there and by "sign out everywhere", it
refreshes and rotates like the others, and it ends when the account is
blocked. The method `jwt:<issuer>` is stored with it:

- the audit log records `signin`, `signin_refused` (with the reason, never
  the token), `invitation_accepted`, `account_created`, `method_linked` and
  `roles_synced` with method `jwt:<issuer>`;
- `AUTH_ROLE_METHODS` can name it, for example
  `{"newsletter-editor": ["jwt:https://id.example.com"]}`;
- the account page lists the linked identity under the issuer's name. It can
  be removed there like any method while another one remains.

Signing out of `auth` does not sign the person out of your system.

## Refusal codes

`?error=` on the login page: `jwt_failed` (the token did not verify, was
replayed, or names no subject; the audit detail `cause` says which check
failed), `jwt_unavailable` (the JWKS could not be fetched), `jwt_email_unusable`,
`jwt_not_linked`, `not_invited`, `domain_not_allowed`, `account_blocked`,
`account_exists`, `invitation_invalid`, `identity_in_use`,
`provider_already_linked`, `too_many_attempts`, `permissions_too_large`.

## Variables

| Name | Default | Meaning |
|---|---|---|
| `AUTH_JWT_ISSUER` | — | **required**: the exact `iss` |
| `AUTH_JWT_AUDIENCE` | — | **required**: the `aud` your system puts in tokens for this site |
| `AUTH_JWT_JWKS_URL` / `AUTH_JWT_PUBLIC_KEY` / `AUTH_JWT_SECRET` | — | exactly one key source (the secret is a wrangler secret) |
| `AUTH_JWT_ALGORITHMS` | every asymmetric one | comma-separated; HS* only with the secret |
| `AUTH_JWT_MAX_AGE_SECONDS` | `300` | the oldest `iat` accepted (1–3600) |
| `AUTH_JWT_IDENTITY_CLAIM` | `sub` | `sub` or `email` |
| `AUTH_JWT_EMAIL_VERIFIED` | `required` | `off`: an address counts without `email_verified` |
| `AUTH_JWT_UNKNOWN` | `reject` | `invite` or `create` |
| `AUTH_JWT_ROLES_CLAIM` | `roles` | the claim name, as written in the token |
| `AUTH_JWT_ROLE_MAP` | — | JSON, see [above](#roles-and-groups-from-the-token) |
| `AUTH_JWT_ALLOW_ADMIN_ROLES` | `off` | `on`: the mapping may give roles with administrative permissions |
| `AUTH_JWT_FORM_ORIGINS` | — | comma-separated `https://` origins whose pages may post the form |
| `AUTH_JWT_ALLOWED_DOMAINS` | — | only verified addresses in these domains |

A configuration problem is named in the `503` answer, in the log, and on
the dashboard summary (`jwt_misconfigured`). The method is never half open.

## Data

`jwt_used_tokens` holds a SHA-256 of issuer and `jti` (never the value) with
its expiry, and expired rows are deleted with every new one. `jwt_synced_grants`
and `jwt_synced_members` mark what the mapping gave. The identity is a row of
`identities` with provider `jwt:<issuer>`. These are new tables, created on
the next request with no schema version and no manual migration (`schema.sql`
has them too).

See also: [THREAT-MODEL.md](THREAT-MODEL.md), [SECURITY.md](SECURITY.md#external-jwt-issuer),
[ACCESS.md](ACCESS.md) (the same idea for Cloudflare Access), [PROVIDERS.md](PROVIDERS.md).
