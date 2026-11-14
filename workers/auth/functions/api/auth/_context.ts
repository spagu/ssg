// What every route needs before it does anything: a configured worker (D1 and
// a valid signing key — otherwise 503 with a readable hint), the schema, the
// settings, the clock; for a POST also the CSRF defence. Then the helpers that
// turn a login into cookies and a request back into a session. A leading
// underscore keeps this file out of the Pages route table.

import { noteKey } from "./_admin-keys";
import { settings } from "./_config";
import type { Settings } from "./_config";
import { ACCESS_COOKIE, REFRESH_COOKIE, accessCookie, readCookie, refreshCookie } from "./_cookies";
import { CONTRACT_VERSION, signToken, verifyToken } from "./_jwt";
import type { AccessClaims } from "./_jwt";
import { loadKeys } from "./_keys";
import type { KeySet } from "./_keys";
import { clock, json } from "./_lib";
import type { Env } from "./_lib";
import { csrfProblem } from "./_request";
import { NO_METHOD_HINT, signInMethods } from "./_provider-methods";
import { sessionGrant } from "./_provider-policy";
import { ensureSchema } from "./_schema";
import { getSession, live, sessionByRefresh } from "./_sessions";
import type { Session } from "./_sessions";
import { findUserById } from "./_users";
import type { User } from "./_users";

export interface Ctx {
  request: Request;
  env: Env;
  keys: KeySet;
  s: Settings;
  now: number;
}

const HINT_DB = "bind a D1 database as AUTH_DB (see wrangler.snippet.toml)";
const HINT_KEY =
  "set the AUTH_SIGNING_KEY secret to the private JWK printed by `node scripts/auth-keys.mjs` (wrangler pages secret put AUTH_SIGNING_KEY)";

// prepare returns the context, or the 503 to send. With `db: false` (JWKS) the
// database is not required.
export async function prepare(request: Request, env: Env, db = true): Promise<Ctx | Response> {
  if (db && !env.AUTH_DB) return json({ error: "not_configured", hint: HINT_DB }, 503);
  const keys = await loadKeys(env);
  if (!keys) return json({ error: "not_configured", hint: HINT_KEY }, 503);
  if (db) await ensureSchema(env);
  // Every way in turned off is a configuration error, named (GO-151).
  if (db && signInMethods(env).length === 0) return json({ error: "no_sign_in_method", hint: NO_METHOD_HINT }, 503);
  return { request, env, keys, s: settings(env, request), now: clock.now() };
}

// hashingUnavailable: the runtime refused to derive at AUTH_PBKDF2_ITERATIONS
// (Cloudflare's production runtime caps PBKDF2 at 100,000), so no new hash
// can be stored. A configuration error, reported as one.
export const hashingUnavailable = (): Response =>
  json({ error: "hashing_unavailable", hint: "the runtime refused AUTH_PBKDF2_ITERATIONS; lower it (production cap: 100000)" }, 503);

// preparePost is prepare plus the CSRF check (see _request.ts). `form` lets a
// plain HTML form through on the routes that must work without JavaScript.
export async function preparePost(request: Request, env: Env, form: boolean): Promise<Ctx | Response> {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const problem = csrfProblem(request, form);
  return problem ? json({ error: problem }, 403) : ctx;
}

// The access cookie must fit in a browser. RFC 6265 (6.1) asks browsers to
// keep cookies of at least 4096 bytes counting the name, the value and the
// attributes, and the major browsers stop there (Chrome and Firefox count name
// + value against 4096). The whole Set-Cookie value — `__Host-ssg-auth=<JWT>`
// plus its attributes — is held to ACCESS_COOKIE_MAX, 296 bytes under that,
// for a proxy or a browser that counts a little differently. The README says
// how many permissions that leaves room for.
export const COOKIE_LIMIT = 4096;
export const ACCESS_COOKIE_MAX = 3800;

export interface Issued {
  cookies: string[];
  exp: number;
  // tooLarge explains, for an administrator, why no access token was issued:
  // the account's permissions do not fit in a cookie. Then `cookies` holds the
  // refresh cookie only (when there is a new one), so the session survives
  // until an administrator narrows the account's roles.
  tooLarge: string | null;
}

// issueCookies signs an access token for the session and returns the cookies
// to set: always the access cookie; the refresh cookie when there is a new
// refresh token (not in the two-tab grace case). `perms` is read from the
// grants every time — a login, a refresh — so a changed grant shows in the
// next token. A list that does not fit is refused whole, never cut short.
export async function issueCookies(ctx: Ctx, user: User, session: Session, refreshToken: string | null): Promise<Issued> {
  const iat = Math.floor(ctx.now / 1000);
  const grant = await sessionGrant(ctx.env, user.id, session);
  const claims: AccessClaims = {
    iss: ctx.s.issuer,
    aud: ctx.s.audience,
    sub: user.id,
    sid: session.id,
    email: user.email,
    iat,
    exp: iat + ctx.s.tokenSeconds,
    ver: CONTRACT_VERSION,
    // The grants, narrowed by the per-role method policy (GO-151) and the
    // second-factor policy (GO-140).
    perms: grant.perms,
    // GO-140: when this session last passed a second factor — an optional
    // claim no consumer has to read (docs/CONTRACT.md); absent without one.
    ...(grant.mfaAt === null ? {} : { mfa_at: grant.mfaAt }),
  };
  const access = accessCookie(await signToken(ctx.keys.signing, claims), ctx.s.tokenSeconds);
  // GO-138: the key's first use, for its age on the admin panel (once per isolate).
  await noteKey(ctx.env, ctx.keys.signing.kid, ctx.now);
  const cookies: string[] = [];
  if (refreshToken) {
    const left = Math.max(0, Math.floor((Date.parse(session.expires_at) - ctx.now) / 1000));
    cookies.push(refreshCookie(refreshToken, left));
  }
  if (access.length > ACCESS_COOKIE_MAX) {
    const detail =
      `the ${claims.perms.length} permissions of this account make a ${access.length}-byte access cookie; ` +
      `this worker issues at most ${ACCESS_COOKIE_MAX} bytes (browsers keep ${COOKIE_LIMIT}). ` +
      "An administrator must grant this account fewer roles, or roles with worker:* wildcards instead of long lists.";
    console.error(`auth: no access token for account ${user.id}: ${detail}`);
    return { cookies, exp: 0, tooLarge: detail };
  }
  return { cookies: [access, ...cookies], exp: claims.exp, tooLarge: null };
}

// permissionsTooLarge is the answer when issueCookies refused: a server-side
// configuration problem the account holder cannot fix, named for whoever can.
export const permissionsTooLarge = (detail: string, cookies: string[] = []): Response =>
  json({ error: "permissions_too_large", detail }, 500, {}, cookies);

// accessClaims verifies the access cookie against our own keys and settings.
// The channel rule of the contract applies here too: the cookie carries a
// person's token only, so a service token (GO-141) found in it is refused —
// and the bearer header is never read by auth's own routes. Machines do not
// administer `auth`: its admin API is for people, behind the cookie and the
// CSRF defence (docs/SERVICE-TOKENS.md says why).
export async function accessClaims(ctx: Ctx): Promise<AccessClaims | null> {
  const token = readCookie(ctx.request, ACCESS_COOKIE);
  if (!token) return null;
  const v = await verifyToken(token, { keys: ctx.keys.verify, issuer: ctx.s.issuer, audience: ctx.s.audience, nowSeconds: Math.floor(ctx.now / 1000) });
  return v.ok && (v.claims.kind ?? "user") === "user" ? v.claims : null;
}

export interface Who {
  user: User;
  session: Session;
  claims: AccessClaims | null; // null when found through the refresh cookie
}

// whoAmI finds the live session and active user behind the request: through
// a valid access token, or — when `viaRefresh` and the access token is gone or
// expired — through the refresh cookie (logout must work after a long pause).
// A token alone is not enough: the session must still be live in D1 and the
// account active, so a revoked session is refused here at once.
export async function whoAmI(ctx: Ctx, viaRefresh: boolean): Promise<Who | null> {
  const claims = await accessClaims(ctx);
  let session: Session | null = null;
  if (claims) {
    const s = await getSession(ctx.env, claims.sid);
    session = s && live(s, ctx.now) && s.user_id === claims.sub ? s : null;
  } else if (viaRefresh) {
    const token = readCookie(ctx.request, REFRESH_COOKIE);
    session = token ? await sessionByRefresh(ctx.env, token, ctx.s.graceSeconds, ctx.now) : null;
  }
  if (!session) return null;
  const user = await findUserById(ctx.env, session.user_id);
  return user && user.status === "active" ? { user, session, claims } : null;
}
