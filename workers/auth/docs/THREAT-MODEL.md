# auth — threat model

What the `auth` worker, the `core` worker and the modules that accept `auth`
sessions (comments, newsletter, newsletter-ui, web-push, republish-trigger)
protect, from whom, and where the lines between them run. Written for the
security review of the 1.9.9 branch; the findings are in
[SECURITY-REVIEW.md](SECURITY-REVIEW.md). The intended mechanisms are in
[SECURITY.md](SECURITY.md) and [CONTRACT.md](CONTRACT.md); this file judges
them by what an attacker can reach.

## Deployment shape

One Cloudflare Pages project (or several Workers) on **one host**, e.g.
`https://example.com`. `auth` lives under `/auth/` and `/api/auth/`, `core`
under `/admin/` and `/api/core/`, each module under its own prefix. All share
the origin, the cookies scoped to it, and — in a Pages project — the
environment. Data lives in D1 databases (`AUTH_DB`, `CORE_DB`,
`COMMENTS_DB`, `NEWSLETTER_DB`, …). Each module verifies `auth`'s tokens
locally with the portable verifier; none asks `auth` per request.

## Assets

| Asset | Where | Why it matters |
|---|---|---|
| Accounts (address, name, status) | `AUTH_DB.users` | identity of every administrator |
| Passwords | `users.password_hash` (PBKDF2-SHA256, 100 000 iterations, per-user salt) | offline cracking after a database leak |
| Sessions | `sessions`, `refresh_history` (SHA-256 of refresh tokens only) | a live session is the account until it ends |
| Access tokens | the `__Host-ssg-auth` cookie (ES256 JWT, 10 min) | accepted by every module until `exp`, no revocation check |
| The signing key | `AUTH_SIGNING_KEY` secret (P-256 private JWK) | whoever holds it is everyone, in every module |
| Permissions | grants, roles, groups, Access group mappings, service-token lists | the authority every module enforces |
| Second factors | `mfa_factors` (TOTP secrets sealed with `AUTH_MFA_KEY`), passkeys, backup-code hashes | the only thing between a stolen password and an owner session |
| One-time links | invitations, reset links (SHA-256 only, single use) | each one is an account |
| Break-glass token | `AUTH_MFA_RESET_TOKEN` secret | removes an owner's factors |
| Service tokens | `ssgsvc_<id>_<secret>` (SHA-256 of the secret only) | machine access, possibly sensitive permissions |
| Subscriber data | newsletter / newsletter-ui D1 | personal data; export permission |
| The ability to mail a list | newsletter-ui `newsletter:send` | mass mail in the site's name |
| Comments and moderation | comments D1 | content published on the site |
| Module settings | `CORE_DB` | hide modules, rename them, the admin home |
| Audit trail | `auth_events`, `comment_events`, core change log | the record of who did what |

## Actors

1. **Anonymous internet attacker** — can send any request to any route, from
   many IPs, in parallel; no account.
2. **Malicious invitee** — holds one invitation link (and so one inbox); may
   try to widen what it grants, accept it for another address, or replay it.
3. **Low-privilege administrator** — holds `auth:admin` or `auth:invite`
   (or a module permission) but not `*`; wants owner, or to remove the owners.
4. **Compromised module on the same host** — code running on the same origin
   (a third-party worker template with a bug, a module the site owner added).
   Sees the `__Host-ssg-auth` cookie (Path=/), can call `/api/auth/*` with the
   victim's cookies from the victim's browser, and can serve pages under its
   prefix; does NOT see the refresh cookie (Path=/api/auth/) unless it is
   served under `/api/auth/`.
5. **Malicious identity-provider account** — controls an account at Google,
   Microsoft, Apple, GitHub or Facebook (or a tenant at Microsoft, or a
   Cloudflare Access identity provider) and can choose its address, name and
   (for a tenant administrator) its claims.
6. **Read access to logs or the database** — a support engineer, a leaked D1
   export, a log drain.
7. **Cross-site attacker** — controls another site (or a sibling subdomain,
   which is *same-site*) that an administrator visits while signed in.

## Trust boundaries

```
 browser ──(cookies, Origin, Sec-Fetch-Site)──▶ auth ──▶ AUTH_DB
    │                                              │
    │                                              ├──▶ identity providers (https, fixed hosts)
    │                                              ├──▶ Cloudflare Access certs (fixed team host)
    │                                              └──▶ mail transport (Resend / MailChannels / SMTP)
    ├──(same cookie)──▶ core ──(caller's credentials, same host only)──▶ modules
    └──(same cookie)──▶ modules ──(portable verifier, JWKS same-origin or inline key)
 machines ──(Authorization: Bearer service token)──▶ /api/auth/token ──▶ modules
 build machine ──(ssg config)──▶ Go generator ──▶ registry JSON, admin theme CSS, worker files
```

- **Browser ↔ worker**: everything from the browser is hostile until checked.
  State-changing requests must prove same origin (`x-ssg-auth: 1` or a
  same-origin form + Origin / Sec-Fetch-Site).
- **auth ↔ modules**: a module trusts a token only after the portable
  verifier checks signature (ES256 only, kid from the key set), `iss`, `aud`,
  `exp`/`iat`, `ver`, and the channel (cookie = person, bearer = machine).
- **auth ↔ identity providers**: a provider's answer is trusted only through
  a flow bound to the browser (cookie handle, state, nonce, PKCE) and only
  for the identity (provider, subject); an address is a hint, never an
  account.
- **core ↔ modules**: core forwards the caller's credentials only to paths on
  the same host under the module's registered base, follows no redirects and
  caches per caller.
- **Build time ↔ runtime**: values from the site's config become CSS and JSON
  served to administrators; paths from the config select files to publish.

## Out of scope

- **A compromised Cloudflare account** (or wrangler credentials): it can read
  every secret, replace the code and the databases. Nothing here defends
  against it.
- **The identity providers and Cloudflare Access themselves** being wrong
  (e.g. a provider that marks an unverified address verified, an Access IdP
  that lets users choose their address). Documented where we rely on them.
- **XSS from site content outside these workers.** The site is static content
  on the same origin. Script running on the origin (a hostile post, a
  compromised third-party script in a theme) runs with the administrator's
  origin: it cannot read the `HttpOnly` cookies, but it can make
  same-origin `fetch` calls that carry them and set `x-ssg-auth: 1` — so it
  can do everything the signed-in administrator can through the JSON API,
  including with the refresh cookie (the browser attaches it to
  `/api/auth/*`). CSRF defences do not stop same-origin script. The defences
  that remain: step-up for sensitive changes (the script does not have the
  second factor), and that the pages of these workers send
  `script-src 'self'` with no inline script. Keep untrusted script off the
  origin; host the admin on its own origin if the site runs third-party code.
- **Denial of service by volume** at the network level (Cloudflare's job);
  application-level cost amplification is in scope.
- **The `workers/ecommerce` template and `examples/ebook-shop`.**

## Threats by actor, and the defence that answers each

| Actor | Threat | Defence (verified in SECURITY-REVIEW.md) |
|---|---|---|
| 1 | Guess passwords / codes / tokens | per-account and per-IP locks, counted **before** the check (SR-02, SR-12); PBKDF2; 256-bit link and service secrets |
| 1 | Forge a token (alg none / HS256 / kid tricks) | ES256 fixed in code, kid from our own set, `crit` refused |
| 1 | Enumerate accounts | same answers and same hashing work for unknown, blocked and wrong; forgot works after the answer |
| 1 | Lock other people out | locks are capped (`AUTH_LOCK_MAX_SECONDS`); no lock keyed on a public id (SR-08) |
| 1 | Cost amplification (D1 writes, PBKDF2, JWKS fetches) | bounded bodies, locks before hashing, JWKS cooldown; OAuth start no longer scans sessions (SR-09) |
| 2 | Accept an invitation for another address, widen its roles | the link is the invitation; roles fixed at creation; provider addresses must be verified ASCII (SR-06) |
| 3 | Escalate through grants, groups, roles, invitations, mappings | `beyond()` on every hand-out; step-up for `*` / `auth:*` (SR-05) |
| 3 | Remove the owners, then act on them | taking away `*` / `auth:*` needs it (SR-04); owner invariant in SQL triggers |
| 4 | Read the access cookie and replay it | short lifetime; tokens not bound to a client — documented in CONTRACT.md |
| 4 | Read the refresh cookie | scoped to `/api/auth/`; a module served there is part of `auth` |
| 5 | Take over an account by address | identity = (provider, subject); an address only opens an invitation; Facebook never verified, Microsoft only with `xms_edov`; non-ASCII refused (SR-06) |
| 6 | Use tokens or links from the database | only hashes stored; TOTP secrets sealed; IP/UA hashed with a salt |
| 6 | Use a refresh token kept from earlier | rotated tokens past the two-tab window revoke the session everywhere (SR-01) |
| 7 | CSRF on any write, in any mode | custom header + Origin rule in auth mode, now also in Access mode (SR-10) |
| 7 | Login CSRF, open redirects | Origin proof on forms; `safeNext` on every `next` |
