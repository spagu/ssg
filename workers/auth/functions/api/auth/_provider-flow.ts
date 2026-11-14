// A sign-in at a login provider in progress (GO-151): created by
// POST /api/auth/oauth/<provider>/start, consumed by its callback. A leading
// underscore keeps this file out of the Pages route table.
//
// The browser holds ONE thing: a random handle in a short-lived cookie,
// __Host-ssg-auth-flow-<provider> (Secure, HttpOnly, Path=/, no Domain,
// SameSite=Lax — the callback is a top-level GET; Apple's cross-site POST is
// bounced to a GET first). Everything else lives in oauth_flows under the
// handle's SHA-256: the state (hashed), the nonce, the PKCE verifier, the
// exact redirect URI, where to go next and why. The callback DELETEs the row
// with RETURNING — one statement, so a replayed or raced callback finds
// nothing — and only then compares the state, so a flow is used up by its
// first answer, right or wrong. One cookie per provider: starting Google does
// not cancel a GitHub sign-in in another tab.

import { readCookie } from "./_cookies";
import type { Ctx } from "./_context";
import { b64urlEncode, iso, randomToken, secretEqual, sha256, sha256hex } from "./_lib";
import type { Provider } from "./_provider";
import { redirectUriOf } from "./_provider";

export const FLOW_SECONDS = 600;
export type Purpose = "signin" | "invite" | "link";

export interface Flow {
  provider: string;
  nonce: string;
  verifier: string | null;
  redirect_uri: string;
  purpose: Purpose;
  next: string;
  invite_hash: string | null;
  user_id: string | null;
  session_id: string | null;
  expires_at: string;
}

export interface Started {
  state: string;
  nonce: string;
  challenge: string | null;
  redirectUri: string;
  cookie: string;
}

export const flowCookieName = (providerId: string): string => `__Host-ssg-auth-flow-${providerId}`;
const flowCookie = (providerId: string, value: string, maxAge: number): string =>
  `${flowCookieName(providerId)}=${value}; Max-Age=${maxAge}; Path=/; Secure; HttpOnly; SameSite=Lax`;
export const clearFlowCookie = (providerId: string): string => flowCookie(providerId, "", 0);

// startFlow stores a new flow and returns what the authorization request and
// the browser need. Expired flows of anyone are swept on the way.
export async function startFlow(
  ctx: Ctx,
  p: Provider,
  f: { purpose: Purpose; next: string; inviteHash?: string; userId?: string; sessionId?: string },
): Promise<Started> {
  const handle = randomToken();
  const state = randomToken();
  const nonce = randomToken();
  const verifier = p.pkce ? randomToken() : null;
  const challenge = verifier ? b64urlEncode(await sha256(verifier)) : null;
  const redirectUri = redirectUriOf(ctx.request, p.id);
  const db = ctx.env.AUTH_DB;
  await db.batch([
    db.prepare("DELETE FROM oauth_flows WHERE expires_at <= ?").bind(iso(ctx.now)),
    db
      .prepare(
        `INSERT INTO oauth_flows (handle_hash, provider, state_hash, nonce, verifier, redirect_uri, purpose, next, invite_hash, user_id, session_id, created_at, expires_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .bind(
        await sha256hex(handle),
        p.id,
        await sha256hex(state),
        nonce,
        verifier,
        redirectUri,
        f.purpose,
        f.next,
        f.inviteHash ?? null,
        f.userId ?? null,
        f.sessionId ?? null,
        iso(ctx.now),
        iso(ctx.now + FLOW_SECONDS * 1000),
      ),
  ]);
  return { state, nonce, challenge, redirectUri, cookie: flowCookie(p.id, handle, FLOW_SECONDS) };
}

// consumeFlow returns the flow this browser started at this provider with
// this state — or null for a missing cookie, an unknown or used handle, an
// expired flow, another provider's flow or a wrong state. The row is gone
// either way.
export async function consumeFlow(ctx: Ctx, providerId: string, state: string): Promise<Flow | null> {
  const handle = readCookie(ctx.request, flowCookieName(providerId));
  if (!handle) return null;
  const row = await ctx.env.AUTH_DB.prepare("DELETE FROM oauth_flows WHERE handle_hash = ? RETURNING *")
    .bind(await sha256hex(handle))
    .first<Flow & { state_hash: string }>();
  if (!row || row.provider !== providerId || Date.parse(row.expires_at) <= ctx.now) return null;
  if (!state || !(await secretEqual(await sha256hex(state), row.state_hash))) return null;
  return row;
}
