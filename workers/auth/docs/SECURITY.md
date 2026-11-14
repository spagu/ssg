# auth — security: password hashing, lockouts, CSRF, login providers, one-time links

The security decisions behind the sign-in, with the measurements they rest
on. The guide is the [README](../README.md); the routes are in [API.md](API.md).

## Password hashing

PBKDF2-SHA256 through WebCrypto, stored as
`pbkdf2-sha256$<iterations>$<salt>$<hash>` (16-byte salt, 32-byte hash,
base64url). The format records its own cost, so `AUTH_PBKDF2_ITERATIONS` can be
raised at any time: each account is re-hashed with the new cost at its next
successful login. Passwords: at least `AUTH_PASSWORD_MIN` characters (12),
at most `AUTH_PASSWORD_MAX` (256, enforced before hashing), never the account's
own address.

### Measurement (step 0) and the cost decision

**Measured locally** — `wrangler` 4.161.0, local workerd 2026-11-10, one
`deriveBits` per request, AMD Ryzen 9 7950X (WSL2), median of 7:

| Iterations | In-isolate time | Whole request (curl) |
|---|---|---|
| 10,000 | 1 ms | 3.0 ms |
| 50,000 | 4 ms | 6.2 ms |
| 100,000 | 8–9 ms | 11.2 ms |
| 200,000 | 15 ms | 18.6 ms |
| 600,000 | 46 ms | 50.0 ms |
| 1,000,000 | 77 ms | 80.9 ms |

Local workerd accepted every count up to 10,000,000 (782 ms); 0 is refused, and
the binary carries the message `Pbkdf2 failed: iteration counts above <max>
are not supported`, with `max` configurable.

**Not in the official documentation, reported by Cloudflare engineers:** the
production runtime refuses more than **100,000** iterations. The Web Crypto page
of the Workers docs lists PBKDF2 as supported with no cap mentioned
([developers.cloudflare.com/workers/runtime-apis/web-crypto](https://developers.cloudflare.com/workers/runtime-apis/web-crypto/));
the cap is stated in workerd issue
[#1346](https://github.com/cloudflare/workerd/issues/1346) (made configurable
in workerd, kept in production "for at least some period of time", January
2024; the issue is still open and users reported the error at 210,000 in 2026).

**Documented:** CPU time per request is 10 ms on Workers Free and 30 s by
default (up to 5 min) on Workers Paid; waiting on I/O does not count
([developers.cloudflare.com/workers/platform/limits](https://developers.cloudflare.com/workers/platform/limits/)).
Pages Functions requests count against the Workers plan
([pages/functions/pricing](https://developers.cloudflare.com/pages/functions/pricing/)).

**Unknown here:** the CPU time on Cloudflare's machines, and how strictly the
free plan's 10 ms is applied to a single request. No Cloudflare account was used.

**Decision.** Default `AUTH_PBKDF2_ITERATIONS = 100000`, the most production
accepts. That is below current guidance for PBKDF2-SHA256 (OWASP: 600,000);
the per-account and per-IP locks, the 12-character minimum and the second factor
(once `AUTH_MFA_KEY` is set) make up for it. One login costs one derivation — about 9 ms on the
machine above, close to the whole free-plan budget before the token is signed.
**Use Workers Paid for production.** On the free plan, a login may be stopped
for exceeding CPU time; lowering the cost (e.g. `50000`) makes it fit at the
price of a weaker hash. Setting more than the runtime accepts is safe: new
hashes fail with `503 hashing_unavailable`, logins keep working with the old
hashes, and the dummy derivation falls back to 100,000 so the "no
enumeration" guarantee holds.

## Lockouts, Turnstile, CSRF

**Locks.** Checked before any hashing; a locked request gets `429` even with the
right password. Per address (`AUTH_LOCK_ACCOUNT_FAILURES`, 5) and per IP
(`AUTH_LOCK_IP_FAILURES`, 20 — people share addresses); the lock lasts
`AUTH_LOCK_BASE_SECONDS` (30), doubled for every further failure, at most
`AUTH_LOCK_MAX_SECONDS` (3600). A day without failures resets a counter. Unknown
addresses are counted the same way, so a lock reveals nothing — and anyone can
keep a known address locked for up to the maximum, which is why it is capped.
Keys are salted hashes, never the address or the IP.
Every attempt is **counted before** the password (or code, or token) is
checked, as a reservation: a burst of parallel requests cannot all pass the
lock check before the first failure is written (security review SR-02). Past
the threshold only the attempt that moves an expired lock on is checked; a
success gives its reservation back. The same holds for the second factor,
break-glass, bootstrap, the password change, "forgot password" and the
service-token exchange (per IP only — never per token id, SR-08).

**Turnstile.** With `AUTH_TURNSTILE_SECRET` (or the shared `TURNSTILE_SECRET`),
after `AUTH_TURNSTILE_AFTER` (3) recent failures for the address or the IP the
login needs a valid `cf-turnstile-response`; otherwise `403
turnstile_required`. A failed verification call counts as not verified.

The login page can show the widget only when it may load Cloudflare's script:
set **`AUTH_TURNSTILE_SITE_KEY`** (a var) next to the secret, and
— only then — the login page's password form carries the Turnstile widget and
that page's CSP adds exactly `script-src https://challenges.cloudflare.com`
and `frame-src https://challenges.cloudflare.com`. Nothing else: checked in
Chromium with Cloudflare's test keys, the widget renders, produces its token
and the login after three failures succeeds with no CSP violation and without
`connect-src`, `style-src` or `img-src` additions (its requests run inside its
own frame). Without both the key and the secret the page and its CSP are
exactly as before; the forgot-password page never loads it. A test pins both
CSP variants. (Without the site key, an over-the-threshold login still gets
`403 turnstile_required` and the page says to wait — that is the case the
site key fixes.)

<a id="csrf"></a>**CSRF.** Every `POST` is refused (`403`) unless both hold:
no cross-origin evidence (an `Origin` header, if sent, is this origin;
`Sec-Fetch-Site`, if sent, is `same-origin` — a sibling subdomain is refused),
and positive proof: the header `x-ssg-auth: 1` (another origin cannot send it
without a CORS preflight this worker never answers), or — for a plain HTML form
on login, logout, logout-all, password, invite, reset,
forgot and `sessions/:id`, which cannot set headers — an
`Origin` equal to this origin or `Sec-Fetch-Site: same-origin`. Every current
browser sends `Origin` on a form POST. Together with `SameSite=Lax` /
`Strict` cookies this also stops *login CSRF* (logging a victim into the
attacker's account), which cookies alone cannot. Refresh and bootstrap accept
JSON only.

Every answer carries `cache-control: no-store` (except the JWKS),
`content-security-policy: default-src 'none'…`, `x-frame-options: DENY`,
`referrer-policy: no-referrer`, `x-content-type-options: nosniff` and
`x-robots-tag: noindex, nofollow`.

### The pages' own headers

The pages under `/auth/` carry the same headers with two differences: a CSP
that lets them load their own stylesheet and script —
`default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self';
form-action 'self'; base-uri 'none'; frame-ancestors 'none'` — and
`referrer-policy: strict-origin` instead of `no-referrer`. Under `no-referrer`
a browser sends `Origin: null` with a form POST (Fetch, "serializing a request
origin"), which the CSRF defence above refuses, so no form would work.
`strict-origin` sends the origin alone — never a path or a query, so never a
token from an invitation or reset URL — and nothing to a plain-http page.

Two additions, each only where it is needed (`_pages-csp.ts`): a page with
login-provider buttons (login, invitation, account) adds the enabled
providers' authorization origins to `form-action` — the button is a form
POSTed here, answered with a `303` to the provider, and browsers apply
`form-action` to that redirect; and the Turnstile pair above on the login page.

## Login providers

Set-up and behaviour are in [PROVIDERS.md](PROVIDERS.md); the decisions:

- **Identity is (provider, subject)**, unique in `identities`. An address is
  used only to find an open invitation, and only when the provider asserts it
  verified it (Google `email_verified`, Apple `email_verified`, Microsoft
  `xms_edov`, GitHub's primary verified address; Facebook never). Without an
  invitation nothing is created — no users row (tested). A verified address
  equal to an existing account's does not sign into it (tested).
- **The flow** is authorization code with PKCE S256 (all but Apple, whose
  documented token request has no verifier). `state` and `nonce` are 32
  random bytes, single use: the browser holds only a random handle in
  `__Host-ssg-auth-flow-<provider>` (10 minutes, `SameSite=Lax`); the state's
  hash, nonce, verifier, redirect URI, purpose and `next` live in
  `oauth_flows`, deleted by the first callback that presents the handle
  (`DELETE … RETURNING`) whatever happens next. The redirect URI is one fixed
  path per provider on this host and must equal the flow's.
- **Outbound requests** go only to each provider's hard-coded https hosts,
  with `redirect: "error"`, a 10 s time limit and a 256 KiB cap
  (`_provider-fetch.ts`).
- **ID tokens** are verified locally: `alg` from an allowlist (RS256 — `none`
  and HS256-with-the-public-key refused before any key is used), `kid` in the
  provider's JWKS (cached per isolate for an hour; an unknown `kid` refetches
  at most once a minute), RSA ≥ 2048 bits, signature, `iss` (Microsoft:
  `https://login.microsoftonline.com/<tid>/v2.0` with the token's own `tid`,
  restricted by `AUTH_MICROSOFT_TENANT`), `aud` (`azp` when several), `exp`,
  `iat`, `nonce`, `sub`.
- **Provider tokens** (access, ID) and codes stay in memory for the callback:
  never stored, returned or logged; client secrets and the Apple key never
  leave the token request. A test scans every answer, log line and table
  after flows with all five providers.
- **Linking** a provider to a signed-in account needs a session started in the
  last `AUTH_RECENT_SIGNIN_MINUTES` (10), checked at the start and again at the
  callback (the same session, still live). **The last method** of an account
  cannot be removed: triggers on `identities` and one conditional `UPDATE` for
  the password, so concurrent removals leave one (tested).
- **Per-role methods** (`AUTH_ROLE_METHODS`) are applied at every token issue
  from the grants' provenance; a malformed value grants nothing.
- **Apple's callback** is a cross-site form POST, which carries no
  `SameSite=Lax` cookie; the POST changes nothing and bounces to the GET,
  where every check runs. It has no CSRF check by design — it can do no more
  than a crafted GET link, which the flow cookie and state already defeat.

## Cloudflare Access

The bridge ([ACCESS.md](ACCESS.md)) trusts a token only after checking it
here, whatever Access did in front: RS256 only (`none` and HS256 with the
public key are refused, tested); keys from the configured team's certs
endpoint only — the host is built from `AUTH_ACCESS_TEAM`, never from the
request or the token, fetched through the same guarded fetch as the login
providers (https, that host, no redirect, time and size limits); `aud` must
contain this application's AUD, so a token for another Access application of
the same team opens nothing; `iss` must be the team; `exp` is **required**
(the older copies in other templates accept a token without one), `nbf`/`iat`
not in the future beyond 60 s, `type` `app` when present, `sub` present. The
keys are cached per isolate (one hour); an unknown key id refetches at most
once a minute; a certs outage keeps already-held keys for up to a day, and
with none the sign-in is refused — never opened. One variable of the pair
without the other, or a value that cannot be right, is a `503` naming it.

Identity is the pair (`access`, `sub`), like a provider's. The address only
finds an invitation — compared after the one normalisation (trim,
lower-case) and only in printable ASCII, so case, a Unicode look-alike that
lower-cases to ASCII (the Kelvin sign), an IDN form, a plus-tag or a trailing
dot never reach someone else's account (a table of them is tested). Two
opt-ins widen this, both off by default: `AUTH_ACCESS_LINK_BY_EMAIL` (an
existing account entered by address — it makes the identity provider's
administrators able to sign in as anyone; audited) and
`AUTH_ACCESS_AUTO_CREATE` (refuses to start with a default role that is
owner, a wildcard or a sensitive permission). A blocked account is refused
whatever Access says; `AUTH_ROLE_METHODS` and the second factor apply
to `access` as to any method — Access is never assumed to have checked a
second factor. Group mapping changes only the memberships it added, never one
added by hand, and keeps the last active owner's. Sign-out sends an Access
session to the team's logout URL, built from configuration only (no open
redirect); the pages with a sign-out form allow exactly that origin in
`form-action`. No token, signature or claim value is logged or stored: the
log and `auth_events` carry reasons and addresses (tested by scanning both).

## Second factor

[SECOND-FACTOR.md](SECOND-FACTOR.md) is the whole story; the security points:

- **No session before the second step.** A first factor for an account with
  a factor yields only a pending handle: 32 random bytes in
  `__Host-ssg-auth-mfa` (Secure, HttpOnly, `Path=/`, SameSite=Lax so it
  survives a provider's redirect chain), its SHA-256 in D1 with the account,
  the first factor and `next`; 5 minutes; consumed in the statement that
  checks it is unused. The code is checked against the handle's own account —
  the request never names one, so a handle cannot be carried to another
  account (tested). The page reveals nothing the first factor had not proved.
- **TOTP:** ±1 step; replay refused in one conditional `UPDATE` (the step
  must be later than the last accepted, so two requests with one code cannot
  both pass); comparison over all three candidates. Secrets AES-256-GCM under
  HKDF(`AUTH_MFA_KEY`), bound to row and account by associated data; a table
  scan in the tests finds no secret in clear. A short key fails closed.
- **Backup codes:** 80 random bits, SHA-256 only, single use in one
  conditional `UPDATE`, regenerating deletes the old set in the same batch.
- **Passkeys:** challenge server-side, single use, bound to account, purpose
  and pending handle or session; `clientDataJSON` type, challenge, origin,
  not cross-origin; rpId hash; UP; signature over `authData ‖
  SHA-256(clientDataJSON)` (DER → P1363 for ES256); counter monotonic (a
  stuck-at-zero authenticator allowed), moved in a conditional `UPDATE`. The
  CBOR decoder is strict and bounded (no tags, floats or indefinite lengths,
  depth 8, 512 items, 16 KiB, lengths checked before anything is allocated,
  duplicate keys refused). Attestation is not verified (requested `none`).
- **Lockout:** wrong codes count per account (`AUTH_LOCK_ACCOUNT_FAILURES`)
  and per IP (`AUTH_LOCK_IP_FAILURES`) on their own counters, checked before
  any code, counted before the code is checked (SR-02); break-glass locks per
  IP and per account (5), and its token must be at least 32 characters.
- **Policy at token issue**, so other workers need nothing: a role needing a
  second factor is absent from a session that has not passed one, at sign-in
  and every refresh. Providers and Access are never trusted to have checked
  one. The grace for an account without a factor adds only enrolment, and
  enrolling a first factor needs a recent sign-in.
- **Step-up** for sensitive changes; **recovery** by another owner (never
  oneself) or a once-only break-glass bound to the account that just passed
  its password and holds `owner`.
- Never logged or answered: secrets (except the enrolment answer to its own
  session), codes, keys, assertions.

## One-time links

Invitations and password reset links carry 32 random bytes (base64url); only
their SHA-256 is stored (`invitations.token_hash`, `password_resets.token_hash`).
Each is single use, consumed in one conditional statement (two simultaneous
uses: exactly one wins — tested), and short-lived (`AUTH_INVITE_HOURS`, 72;
`AUTH_RESET_MINUTES`, 60). A new link for the same address or account makes
the older one useless at once. Opening a link changes nothing — mail scanners
fetch every URL — the change happens on the form's POST. A test runs a full
flow (links handed out, mailed, refused by the provider, used) and scans every
table, log line and header for every token: none is there. Using a reset link
ends every session of the account and clears its login lock. An administrator
can get a reset link only for an account whose permissions they hold
themselves (`403 escalation`), so `auth:admin` alone cannot take over an
owner.

`POST /api/auth/forgot` gives the same answer for every address and does its
work (lookup, link, mail) after answering (`waitUntil`), so neither the answer
nor its timing says whether an account exists. It is limited per address (3)
and per IP (10), then one an hour, through the same counters as the login lock
(kinds `forgot_account`, `forgot_ip`, keyed by salted hashes).
