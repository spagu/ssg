# The `auth` contract — for workers that accept its sessions

This is the agreement between the `auth` worker, which signs people in and
issues access tokens, and every other worker (a *consumer*) that wants to
accept them: `comments`, `newsletter`, `newsletter-ui`, `core`, or your own.
A consumer never asks `auth` anything per request. It carries one file —
[`portable/session-verifier.ts`](../portable/session-verifier.ts) — and checks
the token itself with the public key.

**Contract version: 1.** It is the `ver` claim in every token and the
`CONTRACT_VERSION` constant in the verifier.

## The access token

A compact JWS (three base64url parts, no padding), at most 4096 bytes because
it lives in a cookie (the verifier refuses anything over 8192 bytes).

Header — `{"alg": "ES256", "typ": "JWT", "kid": "<key id>"}`. The verifier
never trusts `alg`: ES256 (ECDSA P-256, SHA-256) is fixed in its code, and any
other value — `none`, `HS256`, `RS256` — is refused before a key is touched. A
`crit` member is refused too. `kid` selects one of the keys the consumer
trusts; it is the RFC 7638 thumbprint of the public key.

| Claim | Type | Meaning |
|---|---|---|
| `iss` | string | issuer: `AUTH_ISSUER`, default `https://<host>/api/auth`; must equal the consumer's `<PREFIX>_AUTH_ISSUER` |
| `aud` | string (never an array) | audience: `AUTH_AUDIENCE`, default the site host; must equal `<PREFIX>_AUTH_AUDIENCE` or, unset, the host of the request |
| `sub` | string | the account id (`verifySession` → `user.id`) |
| `sid` | string | the session id (`sessionId`) — for audit logs; a consumer cannot check it is still live |
| `email` | string | the account's address, lower case (`user.email`); may be `""` for a service account |
| `iat` | number | issued at, Unix seconds |
| `exp` | number | expires at, Unix seconds (`expiresAt`, as ISO 8601) |
| `ver` | number | contract version, `1`; anything else is refused (`unsupported_contract`) |
| `perms` | string[] | the account's effective permissions, sorted, wildcards as written (`newsletter:*`, `*`) |
| `kind` | `"user"` \| `"service"`, optional | absent means `user`; `service` marks a machine's token from `POST /api/auth/token` ([SERVICE-TOKENS.md](SERVICE-TOKENS.md)): `sub` is a service account, `sid` the service token's public id. Any other value is refused (`unsupported_contract`) |
| `mfa_at` | number, optional | when this session last passed a second factor ([SECOND-FACTOR.md](SECOND-FACTOR.md)), Unix seconds; absent without one. Informative: the verifier ignores it — a role that needs a second factor is simply absent from `perms` until one is passed |

Lifetimes: the access token lives `AUTH_TOKEN_MINUTES` (default **10
minutes**); the session behind it, renewed through the refresh cookie, lives
`AUTH_SESSION_DAYS` (default 14 days). Clock leeway: **30 seconds** on `exp`
and `iat`, the same in `auth` and in the verifier.

## Cookies and the bearer header

| Cookie | Holds | Attributes | Who reads it |
|---|---|---|---|
| `__Host-ssg-auth` | the access token | `Path=/; Secure; HttpOnly; SameSite=Lax`, `Max-Age` = token lifetime | every consumer on the host |
| `__Secure-ssg-auth-refresh` | the opaque refresh token | `Path=/api/auth/; Secure; HttpOnly; SameSite=Strict` | `auth` only — never sent to a consumer |

One cookie on one host: every worker of the same host sees the same session.
A worker on another host needs its own login; that is out of scope.

`Authorization: Bearer <token>` is the channel for machines. When the header
uses the Bearer scheme, the verifier reads it **and only it** — a bad bearer
token is not rescued by a cookie on the same request. Any other scheme (`Basic`,
used by consumers' password mode) is ignored, and the cookie is read.

**The kind must match the channel**: the cookie carries `user` tokens only, a
bearer header `service` tokens only; otherwise `wrong_kind`. The reasoning:

- SameSite never protected the token *value*: whoever holds it can send it in
  a `Cookie` header from any HTTP client. Refusing a user token as bearer is
  therefore not a defence against a thief, and the contract does not pretend
  it is.
- What it does buy: the access cookie is HttpOnly, so no legitimate client can
  ever put a user token into a header — one arriving there was copied out of a
  browser or a log, and refusing it costs nobody anything while making such
  use visible (`wrong_kind` in the consumer's logs).
- A bearer header is never attached by a browser on its own, so a request
  authenticated by bearer cannot be a cross-site request forgery; a request
  authenticated by the cookie always can. Keeping one kind per channel lets a
  consumer read `kind` and know which case it is in (cookie → apply your CSRF
  check to state-changing requests; bearer → none needed).
- A service token, whose permissions were issued for a script, never rides in
  a browser.

`core` forwards the caller's `Cookie` header to modules as is; it must
not turn a cookie into a bearer header.

## Keys

`GET /api/auth/jwks` answers a JWK Set, cacheable for five minutes:

```json
{"keys": [
  {"kty": "EC", "crv": "P-256", "x": "…", "y": "…", "kid": "<thumbprint>", "alg": "ES256", "use": "sig"},
  {"kty": "EC", "crv": "P-256", "x": "…", "y": "…", "kid": "<thumbprint>", "alg": "ES256", "use": "sig"}
]}
```

The first key is the current one; a second is listed only during a rotation.
The verifier computes each key's id itself (a `kid` written in the JWK is
ignored) and skips anything that is not an ES256 P-256 signing key — and any
JWK holding a private `d`: the signing key must never leave `auth`.

A consumer gets the keys one of two ways (exactly one must be set):

| Source | Variable | Behaviour |
|---|---|---|
| **inline (recommended)** | `<PREFIX>_AUTH_PUBLIC_KEY` = one public JWK (the `AUTH_PUBLIC_KEY=` line `scripts/auth-keys.mjs` prints) or a whole JWKS (the body of `/api/auth/jwks`) | zero requests; works while `auth` is down; rotation means editing the var |
| URL | `<PREFIX>_AUTH_JWKS_URL` = `/api/auth/jwks` or `https://<same host>/api/auth/jwks` | fetched on first use, cached per isolate for 5 minutes; refetched at most **once a minute** per URL, also when an unknown `kid` arrives, so random `kid`s cannot cause a request each; concurrent requests share one fetch; 3 s timeout, 16 KiB cap, redirects not followed; only `https:` on the request's own origin |

When keys cannot be obtained the verifier denies (`keys_unavailable` → 503);
keys fetched earlier keep working until their 5 minutes are up. A malformed
inline key is a configuration error (`not_configured` → 503).

### Rotating the key, seen from a consumer

`auth` starts signing with a new key the moment it is deployed (README,
"Rotating the key"), so a consumer must know the new key **before** that:

- **Inline**: 1) generate the new pair; 2) set every consumer's
  `<PREFIX>_AUTH_PUBLIC_KEY` to a JWKS with **both** public keys —
  `{"keys": [<new>, <old>]}` — and deploy them; 3) rotate `auth`
  (`AUTH_SIGNING_KEY` = new, `AUTH_PREVIOUS_PUBLIC_KEY` = old); 4) after the
  token lifetime plus 30 s, set consumers back to the new key alone, then
  remove `AUTH_PREVIOUS_PUBLIC_KEY` from `auth`.
- **URL**: nothing to do. The first token with the new `kid` triggers a
  refetch; a consumer that fetched less than a minute before the rotation
  refuses new tokens for the rest of that minute (`unknown_key` → 401; the
  panel refreshes and retries). Keep the old key published (step 3 of the
  README procedure) for at least the token lifetime plus five minutes.

## The verifier API

All exported from `session-verifier.ts`:

| Export | Purpose |
|---|---|
| `configFrom(env, prefix)` | the config from `<PREFIX>_AUTH_*`: `null` when none is set (mode off); a config whose `configProblem` names what is wrong when the set is incomplete |
| `verifySession(request, config, deps?)` | `{ok: true, user: {id, email}, perms, sessionId, expiresAt, kind}` or `{ok: false, reason}`; `deps` (`fetch`, `now`) is for tests |
| `can(perms, required)`, `validPermission(s)` | the permission check — byte for byte the code of `functions/api/auth/_can.ts` |
| `denied(request, config, reason)` | the answer to a failed verdict (table below) |
| `loginRedirect(request, config, reason?)` | page → 302 to the login URL with `next`; API → 401 JSON |
| `forbidden(permission)` | 403 `{"error": "forbidden", "permission": "<name>"}` |
| `wantsPage(request)` | a GET/HEAD asking for `text/html` (a browser navigation) |

Reasons, checked in this order (the same order and the same rules as auth's
own `verifyToken`), and what `denied` answers:

| Reason | When | Page request | API request |
|---|---|---|---|
| `not_configured` | half-configured set, unusable inline key, JWKS URL not same-origin `https:` | 503 `{"error": "not_configured", "detail"}` | same |
| `no_session` | no cookie (or an empty bearer) | 302 → login | 401 `{"error": "unauthorized", "reason": "no_session"}` |
| `malformed` | not three base64url parts, not JSON objects, over 8192 bytes, a claim of the wrong type | 302 → login | 401 `reason: "invalid_session"` |
| `bad_signature` | `alg` other than ES256, `crit`, a signature that does not verify | 302 → login | 401 `invalid_session` |
| `unknown_key` | `kid` missing or not among the trusted keys | 302 → login | 401 `invalid_session` |
| `keys_unavailable` | the JWKS URL failed and nothing fresh is cached | 503 `{"error": "auth_unavailable"}`, `Retry-After: 60` | same |
| `unsupported_contract` | `ver` ≠ 1, or an unknown `kind` | 302 → login | 401 `invalid_session` |
| `wrong_issuer` / `wrong_audience` | `iss` / `aud` differ | 302 → login | 401 `invalid_session` |
| `expired` | `exp` ≤ now − 30 s | 302 → login | 401 `reason: "expired"` |
| `not_yet_valid` | `iat` > now + 30 s | 302 → login | 401 `invalid_session` |
| `wrong_kind` | a user token as bearer, a service token as cookie | 302 → login | 401 `invalid_session` |

401 answers carry `WWW-Authenticate: Bearer realm="ssg-auth"` and every answer
`Cache-Control: no-store`. A panel that gets a 401 calls `POST
/api/auth/refresh` once and retries; the browser drops the access cookie when
it expires, so an expired token usually shows up as `no_session`, not
`expired`. `next` is the path and query of the current request, never a host
(leading slashes collapsed; over 512 characters → `/`).

## Admin modes in a consumer

One order, the same in every worker:

1. **Cloudflare Access**, when `<PREFIX>_ACCESS_TEAM` and `_ACCESS_AUD` are set
   — the worker's existing Access check, unchanged.
2. **auth**, when any `<PREFIX>_AUTH_*` variable is set. A complete set turns
   the mode on; an incomplete one is a loud **503** naming what is missing —
   never a silent fall-through to the password.
3. **The shared password** (`<PREFIX>_ADMIN_PASSWORD`). It stays a
   first-class mode: one person and one worker need no accounts, roles or
   second database. Setting up `auth` does not disable it silently; switching
   it off is a separate, documented step in each worker.

Nothing set → 503, never open. In modes 1 and 3 every permission is granted,
as before.

## Variables per consumer

| Variable | Required | Meaning |
|---|---|---|
| `<PREFIX>_AUTH_ISSUER` | yes | the `iss` to accept, e.g. `https://example.com/api/auth` |
| `<PREFIX>_AUTH_LOGIN_URL` | yes | where a page without a session is sent, e.g. `/auth/login` |
| `<PREFIX>_AUTH_PUBLIC_KEY` | one of these two | public JWK or JWKS, as JSON |
| `<PREFIX>_AUTH_JWKS_URL` | one of these two | `/api/auth/jwks` or a same-origin `https://` URL |
| `<PREFIX>_AUTH_AUDIENCE` | no | the `aud` to accept; default the request host (as `auth` defaults) |

Prefixes: `COMMENTS` (comments), `NEWSLETTER` (shared by `newsletter` and
`newsletter-ui`, like their Access variables), `CORE` (core); your own worker
picks its own. Every worker of a Pages project shares one environment, which is
why the names carry a prefix.

## What is compatible, what needs a new version

Compatible within version 1 (consumers keep working unchanged):

- a new optional claim that no consumer has to understand;
- `kind: "service"` tokens — every v1 verifier already handles `kind`;
- more or fewer permissions, new permission names, new keys, a rotation, a
  different token lifetime, a different issuer or audience *value* (consumers
  update their variables);
- new verifier releases that refuse *more* (the file is copied byte for byte,
  so all consumers move together).

A new contract version (`ver: 2`, a new `CONTRACT_VERSION`, and v1 verifiers
refuse it):

- another algorithm or key type; a different cookie name or attributes that
  change who receives it; `aud` as an array; a changed meaning or type of any
  claim above; a required claim added; a different permission syntax or `can`
  semantics.

`auth` must keep issuing the version its consumers understand until every
copy is updated.

## How stale permissions can be

A consumer trusts a token until it expires. A granted or revoked role, a
blocked account, a logout, "log out everywhere" or a password change reaches
a consumer at the latest after **`AUTH_TOKEN_MINUTES` + 30 s — 10 min 30 s by
default**. Logout clears the cookies of *that* browser at once; a copy of the
token held elsewhere keeps working until it expires
(`test/demo-consumer.test.ts` asserts exactly that). With a JWKS URL, removing
a key from `auth` takes up to five more minutes to reach a consumer's cache.

## Make your own worker a consumer

1. Copy `workers/auth/portable/session-verifier.ts` into your template,
   byte for byte. In a Pages template give it a leading underscore, which
   keeps it out of the route table — e.g.
   `functions/api/<worker>/_session-verifier.ts`; in a Worker, anywhere under
   `src/`.
2. Copy `workers/auth/portable/copy-guard.ts` to `test/auth-copy-guard.ts`,
   and add `test/auth-copies.test.ts` (next section).
3. Declare your permissions (`<worker>:<action>`) in your `module.json`.
4. Gate every admin route as below: Access → auth → password.
5. Document `<PREFIX>_AUTH_*` in your README and `wrangler.snippet.toml`.
6. Tests: each route × (no session, session without the permission, session
   with it), and half-configured → 503.

A complete minimal handler (`functions/api/myworker/admin.ts`):

```ts
import { can, configFrom, denied, forbidden, verifySession } from "./_session-verifier";
import { verifyAccess } from "./_access"; // your template's existing Access check
import { passwordOk } from "./_password"; // your template's existing password check

interface Env { MYWORKER_ACCESS_TEAM?: string; MYWORKER_ACCESS_AUD?: string; MYWORKER_ADMIN_PASSWORD?: string }
const json = (body: unknown, status = 200) => Response.json(body, { status, headers: { "cache-control": "no-store" } });

// requireAdmin: who is acting, or the Response to send instead.
async function requireAdmin(request: Request, env: Env, permission: string): Promise<{ who: string } | Response> {
  if (env.MYWORKER_ACCESS_TEAM && env.MYWORKER_ACCESS_AUD) {
    const refused = await verifyAccess(request, env.MYWORKER_ACCESS_TEAM, env.MYWORKER_ACCESS_AUD);
    return refused ?? { who: "access" };
  }
  const auth = configFrom(env, "MYWORKER");
  if (auth) {
    const v = await verifySession(request, auth);
    if (!v.ok) return denied(request, auth, v.reason);
    return can(v.perms, permission) ? { who: v.user.id } : forbidden(permission);
  }
  if (!env.MYWORKER_ADMIN_PASSWORD) return json({ error: "not_configured" }, 503);
  return (await passwordOk(request, env.MYWORKER_ADMIN_PASSWORD)) ? { who: "password" } : json({ error: "unauthorized" }, 401);
}

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const who = await requireAdmin(request, env, "myworker:read");
  if (who instanceof Response) return who;
  return json({ items: [], by: who.who });
};
```

A runnable version of the same gate, exercised end to end against tokens from
the real login flow, is `test/demo-consumer/admin.ts` with
`test/demo-consumer.test.ts`.

## The copy guard

A template is scaffolded alone (`ssg new worker <name>`) and cannot import
from `workers/auth`, so it holds copies, and a test keeps them honest.
`portable/copy-guard.ts` is that test helper; your template carries a copy of
it too, and guards it with itself:

```ts
// test/auth-copies.test.ts
import { guardCopies } from "./auth-copy-guard.ts";

guardCopies(import.meta.url, {
  "../functions/api/myworker/_session-verifier.ts": "session-verifier.ts",
  "./auth-copy-guard.ts": "copy-guard.ts",
});
```

Keys are your copies, relative to the test file; values are file names in
`workers/auth/portable/`, found at `../../auth/portable/` from
`workers/<template>/test/`. In this repository the test fails, listing every
copy that is missing or differs ("copy it again, never edit a copy"). In a
template scaffolded alone it is **skipped**, and the skip reason says so
("workers/auth is not next to this template (scaffolded alone)…"). To check
more than identity, run the verifier's own suites against your copy the way
`test/demo-consumer.test.ts` does (a resolve hook pointing their import at
the copy). Changing the verifier: edit the source, run `workers/auth`'s tests,
copy it to every consumer, run theirs.

## Threat notes — what the verifier does not protect against

- **Script on the same origin.** The cookie is HttpOnly, so script cannot read
  it, but script running on the host (an XSS in any worker or in the site) can
  make requests that carry it and act as the signed-in person, within their
  permissions. Every worker on the host is part of the trusted base; keep the
  panels' strict CSP and never insert data as HTML.
- **A compromised `auth` signing key** lets its holder mint tokens with any
  permissions for every consumer until the key is rotated out everywhere. The
  key exists only as `auth`'s secret; consumers hold public keys only, and the
  verifier refuses a JWK with a private part so one cannot be pasted by
  mistake.
- **Clock skew** beyond 30 s between `auth` and a consumer makes fresh tokens
  `not_yet_valid` or old ones live longer by the skew. Cloudflare's clocks are
  synchronised; a self-hosted runtime must be too.
- **Revocation is not immediate** (above): a consumer does not see a logout or
  a removed grant until the token expires.
- **Replay of a stolen token** works until it expires, from anywhere. The
  short lifetime is the only bound; tokens are not bound to a client.
- **CSRF** on state-changing routes authenticated by the cookie is the
  consumer's job (SameSite=Lax already keeps the cookie off cross-site
  sub-requests and POSTs; check `Origin` as the existing panels do). The same
  holds for Cloudflare Access mode: Access passes any request carrying its
  `CF_Authorization` cookie, so a write behind Access needs the same check
  (security review SR-10).
- **The JWKS URL** is only as trustworthy as the host it is fetched from —
  which is why it must be the request's own origin over `https:`, and why the
  inline key is recommended.

## Where this is tested

`test/verifier-attacks.test.ts` (the attack corpus, channels, cookies),
`test/verifier-keys.test.ts` (inline keys, JWKS URL, cache, cool-down under a
flood of unknown `kid`s, timeouts, size cap, origin rules),
`test/verifier-config.test.ts` (`configFrom`, the answers, `can` identity, no
imports and ≤ 200 lines, the copy guard), `test/verifier-differential.test.ts`
(3000 generated tokens: the same decision and reason from the verifier and
from `verifyToken`), `test/demo-consumer.test.ts` (end to end, and every suite
again against the copy). 100 % lines and branches on the verifier.
