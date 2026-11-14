# auth — the second factor (TOTP, backup codes, passkeys)

A password hashed within the Workers runtime's limits (PBKDF2-SHA256, 100 000
iterations — [SECURITY.md](SECURITY.md#password-hashing)) is a weaker defence
than a panel that sends mail to a whole list deserves. A second factor closes
that gap: something the person **has** — a phone with an authenticator app, a
passkey or security key — asked for after the password or the login provider.

## Switching it on

Nothing changes until you set one secret:

```sh
openssl rand -base64 32 | npx wrangler pages secret put AUTH_MFA_KEY
```

`AUTH_MFA_KEY` (at least 32 characters) encrypts the authenticator apps'
secrets in D1 and, by being set, turns the second factor on: enrolment, the
step in sign-in, the policy below and step-up. A value shorter than 32
characters is a configuration error that **fails closed**: the policy still
applies, but no app code can be enrolled or checked (503 `mfa_unavailable`);
backup codes and passkeys keep working. Removing the secret turns the whole
feature off again — every factor is then ignored (and the app secrets cannot
be read).

## What a person sees

| Step | What happens |
|---|---|
| Set up | **Your account → Second factor → Add a second factor** (`/auth/mfa/enrol`): an authenticator app (scan the QR code drawn on the page, or type the key; confirm with the first code) or a passkey. The first factor comes with **ten backup codes**, shown once. |
| Sign in | Password (or provider, or Cloudflare Access), then `/auth/mfa`: the app's six-digit code, a passkey, or a backup code. No session exists before that step. |
| Sensitive change | `/auth/mfa/step-up`: the second factor again, if the last one in this session is older than `AUTH_STEP_UP_MINUTES` (10). |
| Lost the phone | A backup code; otherwise another owner resets the factors; a sole owner uses the break-glass secret (below). |

Everything except a passkey works with JavaScript off (plain forms). The
code fields accept paste and autofill (`autocomplete="one-time-code"`, digit
keyboard, spaces and dashes allowed) — nothing to remember or work out
(WCAG 2.2, 3.3.8).

### Authenticator apps (TOTP)

RFC 6238 as the apps implement it: **HMAC-SHA-1, six digits, 30-second
steps**, a 160-bit secret shown as base32. SHA-256/512 and eight digits are
in the RFC, but many apps ignore those URI parameters and would then show
codes that never match, so they are not offered. A code from the step before
or after the current one is accepted (clock drift); a code whose step is not
**later** than the last accepted one for that factor is refused — the same
code never works twice, an older one never after a newer one. The QR code is
drawn by this worker (`_mfa-qr.ts`, byte mode, error correction M) — the
secret never goes to an outside service.

### Backup codes

Ten codes of 16 characters (Crockford base32, 80 random bits), shown as
`XXXX-XXXX-XXXX-XXXX` exactly once — after the first factor and after **New
backup codes**, which replaces the whole set. Only a SHA-256 of each is
stored; each works once; they go with the account's last factor.

### Passkeys — what they do and do not do here

A passkey (WebAuthn) is a **second factor** in this release: it is asked for
*after* a password, a provider or Access. **Passwordless sign-in is not
built** — a passkey alone opens nothing. Details:

- Algorithms ES256 (P-256, nearly every authenticator), RS256 (Windows
  Hello) and EdDSA (Ed25519) — all three were verified against the Workers
  runtime's WebCrypto before this was written.
- Attestation is requested as `none` and **not verified**: the worker does
  not check which make of authenticator it is, only that the person holds the
  key that was registered.
- User presence is required; user verification (PIN, biometrics) is
  preferred but not required — it is a second factor.
- The signature counter must grow; an authenticator that always reports 0
  (synced passkeys do) is accepted at 0, as the WebAuthn spec allows.
- A passkey belongs to one host (the relying party id is the request's host
  name): a site served under several host names registers one per host.
- It needs JavaScript; without it the page offers the app code and the backup
  code.

## Which roles need it — the policy

Default: a role needs a second factor when it holds a permission that
`isSensitive()` flags (`*` — so `owner` —, any `auth:…`, any `…:erase`, any
worker wildcard `…:*`) or one that sends (`…:send`, e.g. the built-in
`newsletter-editor`). `comments-moderator` and `newsletter-viewer` do not.

Change it per role with a variable, in the style of `AUTH_ROLE_METHODS`:

```toml
AUTH_MFA_ROLES = '{"newsletter-editor": false, "comments-moderator": true}'
```

`true` = needs one, `false` = does not, unlisted = the default. A value that
is not a JSON object of booleans fails closed: every role needs a second
factor until it is fixed (logged).

**Where it is enforced:** at every token issue (sign-in, refresh, step-up),
next to the per-role method policy (`_provider-policy.ts`). A session that
has not passed a second factor gets a token **without** those roles'
permissions — the account still signs in with the rest. Nothing changes for
other workers: the permission is simply not in the token. The token also
carries an optional `mfa_at` claim (when the session last passed a second
factor, Unix seconds) that verifiers ignore ([CONTRACT.md](CONTRACT.md)).
`GET /api/auth/me/permissions`, "Your panels" and the account page say which
roles are waiting and link to enrolment (or to step-up).

**Login providers and Access** are never assumed to have checked a second
factor: every first factor goes through the same step (`startSession`).

### An account that must have one but has none (the first owner)

It signs in with its password (or provider) and gets a session **without**
the roles that need a second factor; a browser goes straight to
`/auth/mfa/enrol`. Enrolling proves the new factor, so that session then
counts as having passed one and gets a new token with everything. This is the
only extra capability of such a session, and it is not a bypass:

- the password alone never opens the protected roles — it opens exactly what
  it opened before a factor existed;
- enrolling a first factor needs a sign-in from the last
  `AUTH_RECENT_SIGNIN_MINUTES`, so an old (stolen) session cannot add its own
  authenticator;
- once the account has a factor, the password alone gives no session at all,
  and adding or removing a factor needs a step-up.

## Step-up

These need a second factor from the last `AUTH_STEP_UP_MINUTES` (default 10)
when the account has one, otherwise 403 `step_up_required {stepUpUrl,
minutes}` (a plain form goes to `/auth/mfa/step-up?next=…`):

adding or removing a sign-in method (it replaces the "recent sign-in" rule
for accounts with a factor), adding or removing a factor, new backup codes,
a service token with `allowSensitive`, handing out `*` or `auth:*` (a grant,
a member of a group holding it, an invitation carrying it, a role edited to
hold it), ending another account's sessions (one, all, or everyone's),
blocking an owner, deleting an account, resetting someone's factors.
`requireStepUp(ctx, who)` (`_mfa-step-up.ts`) is the one call for a route;
the admin panel's routes ([PANEL.md](PANEL.md#step-up)) use it the
same way.

## Recovery

- **Another owner:** `POST /api/auth/mfa/reset {userId}` (an owner with a
  recent second factor; never one's own account). Every factor, enrolment
  and backup code of that account goes and its sessions end; it enrols again
  at its next sign-in. Audit event `mfa_reset`.
- **A sole owner (break-glass):** set a one-time secret, then sign in with
  the password and use "Lost your phone or key?" on the challenge page:

  ```sh
  openssl rand -base64 32 | npx wrangler pages secret put AUTH_MFA_RESET_TOKEN
  ```

  It works only in the pending step of a sign-in — after that account's own
  password — only for an account holding `owner`, and **once per value** (the
  worker keeps its hash). It cannot reset a different account: the account is
  the one that just proved its password, never one the request names. Wrong
  values count towards a per-IP lock. The owner is then signed in without
  `*` and sent to enrol. Delete the secret afterwards.

## Key management (`AUTH_MFA_KEY`)

The app secrets are AES-256-GCM ciphertext under a key derived with
HKDF-SHA-256 from `AUTH_MFA_KEY`, bound (as associated data) to their own row
and account. A copy of D1 alone yields no code. Backup codes and passkeys do
not depend on the key.

**Rotation:**

1. `wrangler pages secret put AUTH_MFA_KEY_PREVIOUS` with the current value.
2. `wrangler pages secret put AUTH_MFA_KEY` with a new value.
3. Each app secret is re-encrypted under the new key the next time its code
   is accepted. Rows still under the old key show which key wrote them
   (`mfa_factors.secret` starts `v1.<key id>.`).
4. Remove `AUTH_MFA_KEY_PREVIOUS` once no row uses the old key id. An app
   whose secret is still under a removed key stops matching; its owner uses a
   backup code or a passkey and enrols the app again.

Losing `AUTH_MFA_KEY` makes every app factor unusable (backup codes and
passkeys still work); it never makes one pass.

## Variables

| Name | Kind | Default | Meaning |
|---|---|---|---|
| `AUTH_MFA_KEY` | secret | — | turns the second factor on; encrypts app secrets (≥ 32 characters) |
| `AUTH_MFA_KEY_PREVIOUS` | secret | — | the key before a rotation, still read |
| `AUTH_MFA_ROLES` | var | — | JSON `{"role": true\|false}` overriding the default policy |
| `AUTH_MFA_RESET_TOKEN` | secret | — | one-time break-glass for a sole owner; at least 32 characters — a shorter value counts as unset (security review SR-03). Wrong tokens lock per IP and per account (5, then the growing delay) |
| `AUTH_STEP_UP_MINUTES` | var | 10 | how recent a second factor must be for sensitive changes |

Lockouts reuse `AUTH_LOCK_ACCOUNT_FAILURES` (5) and `AUTH_LOCK_IP_FAILURES`
(20) with their own counters (`mfa_account`, `mfa_ip`); break-glass has its
own per-IP lock (5).

## Audit events

`mfa_enrolled`, `mfa_removed`, `mfa_passed`, `mfa_failed` (sign-in and
step-up; only after a first factor or within a session, so the lockout bounds
them), `mfa_backup_code_used` (with the number left),
`mfa_backup_codes_regenerated`, `mfa_reset` (`via: owner` with `by`, or
`via: break_glass`); `signin` records `secondFactor` when one was passed.
Never a code, a secret or a key.

## Routes

See [API.md](API.md#second-factor-go-140) for the requests and answers, and
[PAGES.md](PAGES.md#second-factor-go-140) for the pages.
