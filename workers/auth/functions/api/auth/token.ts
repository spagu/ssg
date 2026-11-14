// Cloudflare Pages Function: exchange a service token for a short access token (GO-141).
//   POST /api/auth/token   Authorization: Bearer ssgsvc_<id>_<secret>   (no body)
//
// 200 {accessToken, tokenType: "Bearer", expiresIn, expiresAt, permissions}:
// an ES256 access token with `kind: "service"`, `sub` = the service account,
// `sid` = the token's public id, `email` = "", `perms` = the account's
// permissions as they are NOW, narrowed by the token's list, without the
// machine-sensitive ones an owner did not allow (_service-perms.ts: isSensitive,
// plus what roles needing a second factor give — _service-policy.ts). It lives
// AUTH_SERVICE_ACCESS_MINUTES (default 10). No refresh token, no cookies:
// a script exchanges again when it needs a new one.
//
// 401 {error: "invalid_token"} for a missing, malformed, unknown, revoked or
// expired secret and for a disabled account alike — same status, body and
// headers. 429 {error: "too_many_attempts"} + Retry-After while the IP is
// locked after repeated failures (the same policy as the login, separate
// counters; never per token id — SR-08). 500 permissions_too_large when the token would exceed
// what the verifier accepts.
//
// No CSRF check, on purpose: CSRF abuses a credential the browser attaches by
// itself (a cookie). This route reads no cookie at all — only the bearer
// header, which a browser never adds on its own and a page on another origin
// cannot add without a CORS preflight that this worker never answers. A
// person's session cookie sent along is ignored, never used.

import { prepare } from "./_context";
import { CONTRACT_VERSION, signToken } from "./_jwt";
import type { AccessClaims } from "./_jwt";
import { clientIP, json } from "./_lib";
import type { Env } from "./_lib";
import { lockKey, lockState, recordFailure, releaseAttempt } from "./_lockout";
import { tokenPermissions } from "./_service-perms";
import { serviceGrant } from "./_service-policy";
import { parseSecret, serviceSettings, touchToken, usableToken } from "./_service-tokens";

// MAX_TOKEN: the portable verifier refuses tokens over 8192 bytes.
const MAX_TOKEN = 8192;
const INVALID = { "www-authenticate": 'Bearer realm="ssg-auth", error="invalid_token"' };

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const header = request.headers.get("authorization") ?? "";
  const secret = /^bearer\s/i.test(header) ? header.slice(7).trim() : "";
  const parsed = parseSecret(secret);

  // Per IP only (SR-08): a token's id is public (the panel, the audit log, the
  // secret's own prefix) and its 256-bit secret cannot be guessed, so a
  // per-id counter protected nothing and let anyone lock a working token out.
  // Counted before the lookup (SR-02), given back on success.
  const ip = [{ key: await lockKey(env, "service_ip", clientIP(request)), kind: "service_ip" as const, threshold: s.lockIP }];
  const state = await lockState(env, [ip[0].key], now);
  const wait = state.retryAfter || (await recordFailure(env, ip, s, now));
  if (wait > 0) return json({ error: "too_many_attempts" }, 429, { "retry-after": String(wait) });

  const token = parsed ? await usableToken(env, secret, now) : null;
  if (!token) return json({ error: "invalid_token" }, 401, INVALID);
  await releaseAttempt(env, ip);
  const ss = serviceSettings(env);
  const grant = await serviceGrant(env, token.service_account_id);
  const perms = tokenPermissions(
    grant.perms,
    token.permissions === null ? null : (JSON.parse(token.permissions) as string[]),
    JSON.parse(token.allow_sensitive) as string[],
    grant.free,
  );
  const iat = Math.floor(now / 1000);
  const claims: AccessClaims = {
    iss: s.issuer,
    aud: s.audience,
    sub: token.service_account_id,
    sid: token.id,
    email: "",
    iat,
    exp: iat + ss.accessSeconds,
    ver: CONTRACT_VERSION,
    perms,
    kind: "service",
  };
  const accessToken = await signToken(ctx.keys.signing, claims);
  if (accessToken.length > MAX_TOKEN) {
    const detail = `the ${perms.length} permissions of this token make a ${accessToken.length}-byte access token; workers accept at most ${MAX_TOKEN}. Narrow the token's permissions.`;
    console.error(`auth: no access token for service token ${token.id}: ${detail}`);
    return json({ error: "permissions_too_large", detail }, 500);
  }
  await touchToken(env, token, now);
  return json({ accessToken, tokenType: "Bearer", expiresIn: ss.accessSeconds, expiresAt: new Date(claims.exp * 1000).toISOString(), permissions: perms });
};
