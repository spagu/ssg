# auth — security review of the 1.9.9 branch

An independent review of the authentication and administration system:
`workers/auth`, the portable session verifier and its copies, `workers/core`,
the admin gates of comments, newsletter, newsletter-ui, web-push and
republish-trigger, and the Go build steps that feed them (`internal/admintheme`,
`internal/modules`, `internal/generator/modules_registry.go`,
`worker_public.go`). The threat model is [THREAT-MODEL.md](THREAT-MODEL.md).

Method: the docs were read for the intended guarantees only; the code was
judged by attacking it. Every finding below got a test that failed against
the vulnerable code before the fix (the tests named `SR-nn` in
`workers/auth/test/security-review.test.ts`, `workers/comments/test/security-review.test.ts`,
`workers/newsletter/test/security-review.test.ts`, `workers/core/test/security-review.test.ts`,
`workers/newsletter-ui/test/security-review.test.ts`). Races were made
deterministic with read barriers, so the tests do not depend on timing.

## Findings

| ID | Severity | Status | Finding | Test |
|---|---|---|---|---|
| SR-01 | Medium | fixed | A rotated refresh token stayed a session for the whole session lifetime on every route that accepts the refresh cookie (step-up, factor and method changes, password change, logout, OAuth link start) | `SR-01: an old, rotated refresh token no longer authenticates …` |
| SR-02 | Medium | fixed | Every lockout was check-then-write: a burst of parallel requests passed the lock read before any failure was written, so all guesses were checked (second factor, password, password change, break-glass, bootstrap, forgot, service tokens) | `SR-02: …` (5 tests) |
| SR-03 | Low | fixed | `AUTH_MFA_RESET_TOKEN` had no minimum length and break-glass locked per IP only | `SR-03: …` (2 tests) |
| SR-04 | Medium | fixed | Taking permissions away was ungated: an `auth:admin` could revoke co-owners' owner grants (or remove them from an owner group, delete the group, the role, the Access mapping) and then block, reset, sign out the former owners and switch off their service accounts | `SR-04: …` (8 tests) |
| SR-05 | Medium | fixed | No step-up on a reset link for another account, on resending an owner invitation, or on an Access group mapping onto an owner group (no audit for mappings either) — a stale or stolen owner session could take over a co-owner or mint an owner | `SR-05: …` (3 tests) |
| SR-06 | Low | fixed | Provider addresses were lower-cased before the "verified" check: a verified `Kate@example.com` (KELVIN SIGN) accepted kate@example.com's invitation | `SR-06: …` |
| SR-07 | Low | fixed | Linking a provider failed for an account with a factor that had stepped up but signed in long ago (callback demanded a recent sign-in) — availability | `SR-07: …` |
| SR-08 | Low | fixed | A service token's public id was a lockout key: anyone could lock a working token out | `SR-08: …`; `service-exchange.test.ts` "no lockout per token id" |
| SR-09 | Low | fixed | Every anonymous OAuth start ran a full scan-and-delete of `session_methods` (D1 cost amplification) | `SR-09: …` |
| SR-10 | Medium | fixed | comments (and newsletter, low) had no CSRF check in Cloudflare Access mode: Access passes any request carrying `CF_Authorization`, which the browser attaches itself; a same-site page could delete comments or run an import | comments + newsletter `SR-10: …` |
| SR-11 | Low | fixed | A comment URL with a tab or newline (`/\t/evil.example`) became a link to another host in the moderation panel | comments `SR-11: …` |
| SR-12 | Low/Medium | fixed | core's and newsletter-ui's shared-password lockouts had the SR-02 race; newsletter-ui also lost concurrent counts (read, compute, write back) | core + newsletter-ui `SR-12: …` |
| SR-13 | Informational | fixed | Two parallel removals of an owner's two factors both passed the "keep one" check | `SR-13: …` |
| SR-14 | Informational | fixed | Deleting a grant that vanished between read and delete answered 500 after the change | `SR-04: a grant that is gone answers 404, never 500` |
| SR-15 | Informational | documented | An owner without a factor who signs in with the first factor only can enrol within `AUTH_RECENT_SIGNIN_MINUTES` and so gains the owner role: whoever holds the first factor of an un-enrolled owner becomes owner. Owners should enrol at once | — |
| SR-16 | Informational | documented | Lockout abuse: anyone can keep a known address's password or second-factor sign-in locked for up to `AUTH_LOCK_MAX_SECONDS` (capped by design) | — |
| SR-17 | Informational | documented | Cloudflare Access counts every address as verified and maps groups by name; with an IdP that lets users choose either, `AUTH_ACCESS_LINK_BY_EMAIL` / group mappings trust that IdP | — |
| SR-18 | Informational | documented | Apple's `form_post` bounce puts `code` and Apple's `user` JSON in a GET query (edge logs, history); the code is single-use and flow-bound | — |
| SR-19 | Informational | documented | comments and newsletter password modes have no brute-force limit (core and newsletter-ui do) | — |
| SR-20 | Informational | documented | core forwards the caller's cookie and Authorization (core's own Basic password in password mode) to any GET path a module declares under its base; same host only, no redirects, answers schema-checked | — |
| SR-21 | Informational | documented | The Access token checks (core, comments, newsletter, newsletter-ui) ignore `nbf`, and an unknown kid waits for the hourly key refresh (availability after an Access key rotation) | — |
| SR-22 | Informational | documented | `readBody` (auth) reads the whole body before its 16 KiB check when no Content-Length is sent; the platform's request size limit bounds it | — |
| SR-23 | Informational | documented | Audit gaps: group create/rename, service-account create/disable/delete, invitation resend are not audited (group delete and Access mappings now are) | — |
| SR-24 | Informational | documented | A service token without a permission list follows the account's grants as they grow later (documented "null = the account's") | — |
| SR-25 | Informational | documented | Any `auth:invite` holder can replace or withdraw another administrator's pending invitation (denial of service only) | — |
| SR-26 | Informational | documented | The auth panel builds some action URLs from the raw hash id (not exploitable: ids are server-generated, a crafted id 404s before any button renders) | — |
| SR-27 | Informational | documented | `GET /auth/mfa` and `/auth/mfa/enrol` write a challenge row per load (authenticated or pending callers only; swept) | — |

Nothing found blocks the release after these fixes.

## The fixes, briefly

- **SR-01** `_sessions.ts` `sessionByRefresh` applies `rotateRefresh`'s rules:
  the current token, or the previous one inside `AUTH_REFRESH_GRACE_SECONDS`
  with the session's current token as its successor; any other rotated token
  revokes the session (`refresh_reuse`) and finds nothing.
- **SR-02** `_lockout.ts` `recordFailure` is now a reservation called before
  the check. It returns the wait when the attempt must not be checked: past
  the threshold while the lock is set (or not yet written), or when another
  attempt won the compare-and-swap that moves an expired lock on. A refused
  attempt gives its counts back; a success gives the IP's back
  (`releaseAttempt`) and clears the account's. Applied in login, password,
  `_mfa-verify`, break-glass, bootstrap, forgot, token.
- **SR-03** break-glass: a token under 32 characters is treated as unset
  (404 + an error log); a per-account counter `mfa_reset_account` beside the
  IP's.
- **SR-04** `_admin-sensitive.ts` `mayTakeAway` (taking `*` / `auth:*` away
  needs it, plus a step-up) and `mayActOn` (sessions, status, service
  accounts and tokens of an account holding `*` / `auth:*` need it). Taking
  other permissions away stays an ordinary `auth:admin` change — the existing
  "narrowing is fine" rule. The owner invariant itself was never broken.
- **SR-05** `stepUpWhen` in reset-link (another account), resend (`*` /
  `auth:*`), access-groups POST (`*` / `auth:*`); new audit kinds
  `access_mapping_added`, `access_mapping_removed`, `group_deleted` (panel
  vocabulary and strings in five languages).
- **SR-06** providers hand the address over as given; `_provider-callback.ts`
  `trusted` uses `matchableEmail` (the rule Access already had): printable
  ASCII only, normalised once, after the check.
- **SR-07** `linkWith` re-checks the start route's rule (`freshness`).
- **SR-08** token exchange: counters per IP only.
- **SR-09** the `session_methods` sweep moved to `purgeStale` (after a sign-in).
- **SR-10** comments `requireAdmin` runs `csrfProblem` on writes in Access
  mode and the panel sends `x-ssg-auth: 1` in every mode; newsletter applies
  `sameOrigin` to Access-mode writes.
- **SR-11** `normaliseURL` refuses control characters and spaces.
- **SR-12** core and newsletter-ui decide after the comparison: a wrong guess
  counted beyond the limit gets 429 like a locked one, the right password is
  accepted only while the address is still unlocked; newsletter-ui's count is
  one atomic upsert. No extra D1 write for a success (every request in these
  modes carries the password).
- **SR-13** `removeFactor(…, keepOne)` deletes only while another confirmed
  factor exists, in the same statement.

## Existing assertions changed, and why

- `logout.test.ts`: a rotated refresh token used 20 minutes after rotation was
  expected to be found and the session ended as `logout`. That is SR-01's
  vulnerable behaviour; it now ends as `refresh_reuse` (inside the grace
  window it is still `logout`, asserted in `security-review.test.ts`).
- `service-exchange.test.ts`: "lockout per token id: after 5 failures even the
  right secret waits" asserted SR-08's vulnerable behaviour; replaced by the
  opposite rule.
- `service-admin.test.ts`: the caller of the broken-invariant 409 test now
  holds `*` (through a non-owner role): an `auth:admin` deleting an account
  holding `*` is refused with 403 escalation first (SR-04).
- `mfa-enrol.test.ts`: the race simulation turned the delete into a no-op
  while the factor stayed; it now really removes the factor in between, since
  a refused "keep one" delete with the factor still present is 409 (SR-13).
- `comments/test/csrf-actors.test.ts`: Access-mode writes now carry the
  panel's CSRF header (SR-10).

## Attempts that held

**Tokens and keys.** `verifyToken` and the portable verifier (source and the
copies in core, comments, newsletter, newsletter-ui, web-push,
republish-trigger, byte-identical, guarded): `alg` fixed to ES256 in code;
`none`, HS256 with the public key as secret, RS256, `crit` refused before any
key is touched; signature length exactly 64; kid looked up in our own map
(`__proto__` and friends are Map keys); `ver`, `iss`, `aud`, `exp`/`iat` with
30 s leeway, claim types, `kind` vs channel (cookie = person, bearer =
machine). JWKS: same-origin https only, redirects not followed, 16 KiB cap,
3 s timeout, one refetch per URL per minute whatever kids arrive; a private
key (`d`) in a JWKS is skipped. The signing key's kid is the RFC 7638
thumbprint computed here; the private half is proven against the public one.

**OIDC / OAuth.** Flow bound to the browser by a `__Host-` cookie handle,
consumed once (`DELETE … RETURNING`); state, nonce (constant time), PKCE S256
(Apple: nonce); per-provider callback paths (no mix-up); redirect URI
compared; ID tokens RS256 only, ≥ 2048-bit RSA keys from fixed hosts, `iss`
per provider (Microsoft tenant `tid` and tenant restriction), `aud`/`azp`,
`exp`, `iat`. Identity = (provider, subject); an address only opens an
invitation, never an existing account; Facebook never verified, GitHub only
primary+verified, Microsoft only with `xms_edov`. Apple's cross-site
`form_post` only redirects.

**Cloudflare Access bridge.** RS256 only from the configured team host; `aud`
of another application refused; `iss`, `exp` (required), `nbf`/`iat`, type
`app`; link-by-email and auto-create off by default; auto-create never with
owner, `*` or a sensitive default role; ASCII-only address matching; group
sync never removes a hand-made membership and keeps the owner invariant.

**Sessions.** Refresh rotation with an atomic swap; two-tab grace only for the
direct predecessor; reuse revokes; access cookie `__Host-`, HttpOnly, Lax;
refresh cookie `__Secure-`, Path=/api/auth/, Strict; a new session per
sign-in (no fixation: no session id is ever taken from the request);
`/api/auth/resume` refuses cross-site navigations.

**CSRF.** Every state-changing route in `auth` (enumerated from the file
tree: 26 admin handlers, login, logout(-all), refresh, password, reset,
forgot, invite, bootstrap, methods, mfa/*, oauth start, access link) requires
no cross-origin evidence plus `x-ssg-auth: 1` or a same-origin form where a
form is allowed; `Sec-Fetch-Site: same-site` is refused. core requires
`x-core-admin: 1` on every non-GET in every mode; newsletter-ui its header
plus Origin rule; web-push and republish-trigger read only the bearer header.

**Open redirects.** `safeNext` refuses schemes, `//`, backslashes, control
characters and dot-segment results that start with `//`; logout's Access URL
comes from configuration only; the portable verifier's login redirect carries
only a path.

**Enumeration.** Login: same status, body and PBKDF2 work for unknown,
blocked and wrong (dummy derivation at the configured cost, with fallback);
forgot answers before any lookup (`waitUntil`); invite: one 410 for every bad
token; token exchange: one 401.

**Privilege escalation.** `can` matches the exact name, `worker:*` or `*`
only; `beyond` on every hand-out (grant, member, role create/edit, invitation
create/resend, Access mapping); `allowSensitive` needs `*` + step-up; token
narrowing never widens; `worker:*` counts as sensitive; auth's admin API
refuses service tokens. Owner invariant: SQL triggers on grants, members,
groups, roles, owner role permissions, user status and deletes; service
accounts never count as owners.

**Second factor.** Pending handles bound to their account, single use, 5 min;
every first factor goes through `startSession`; TOTP step monotonic in one
UPDATE (no replay across sign-in and step-up); backup codes single use under
concurrency; WebAuthn challenge single use, 300 s, bound to account, purpose
and session/handle; exact origin, rpId hash, UP flag, counter
compare-and-swap; CBOR decoder fuzzed (200 000 random inputs, deep nesting,
2^64 lengths, indefinite lengths, tags, duplicate keys): only `CborError`;
factor reset revokes every session.

**Injection.** SQL: every template literal interpolates constants only
(column and table names from fixed tuples); all input bound. HTML: every
server-rendered value through `esc`; the admin config in a JSON block with
`<`, `>`, `&`, U+2028/9 escaped; no `innerHTML` in the auth, core,
newsletter-ui or comments panels; links through `setAttribute` and local-path
checks; CSP `script-src 'self'` everywhere. CSV export prefixes formula
characters. Mail headers are built by the shared MIME module (CR/LF refused).

**core.** Probes stay on the host and under the module's base (`..`,
`%2e%2e`, `//`, backslash, `?`, `#` refused by comparing before/after
parsing); redirects not followed; 256 KiB cap on the stream; cache keyed per
URL, reader and a hash of the caller's credentials, 1000 entries.

**Denial of service.** Regexes tested with 50–60 k-character adversarial
input (base-path, path, link, file, font-stack patterns): ~1 ms each; Go uses
RE2. Bodies capped at 16 KiB (auth), passwords capped before hashing, locks
before hashing.

**Admin theme and build paths (Go).** Every value written to the CSS is a
colour re-formatted by `ParseHex`, a font name of letters, digits, spaces,
dots, dashes and underscores, a file path matching a strict per-segment
pattern, or a value from a fixed list; the JSON goes through
`encoding/json`. Worker files come from the project's own config; remote
sources are unpacked as regular files (no symlinks); asset clashes and
output collisions refused; registry bases cannot start a segment with `.`.

**Secrets at rest and in logs.** Only hashes of tokens and links; TOTP
secrets AES-GCM sealed and bound to factor and account; IP/UA hashes salted;
audit details are ids and names, never a link or a secret; logs name account
ids and reasons only.

## Not tested, and why

- **Real D1 concurrency.** The fakes are node:sqlite; D1 serialises
  statements in one database, which is what the fixes rely on (single-statement
  upserts, compare-and-swap updates).
- **Real identity providers and Cloudflare Access** (whether any issues a
  verified non-ASCII address; Access's real `SameSite` on `CF_Authorization`).
  Only the fakes were exercised.
- **Edge rate limiting** and the platform's request-size limit.
- **The panels in a real browser** for the panel-only hardening (SR-26).

## Test results at the end of the review

```
auth            720 tests, 720 pass, 0 fail
core            199 tests, 199 pass, 0 fail
comments        103 tests, 103 pass, 0 fail
newsletter      154 tests, 154 pass, 0 fail
newsletter-ui   633 tests, 633 pass, 0 fail
web-push         62 tests,  62 pass, 0 fail
republish-trigger 36 tests, 36 pass, 0 fail
workers/*.test.ts 107 tests, 89 pass, 0 fail, 18 skipped
```

Every file changed by the review is at 100 % line coverage and at least 98 %
branch coverage in its template's suite (node `--experimental-test-coverage`).
