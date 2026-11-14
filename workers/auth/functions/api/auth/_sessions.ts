// Sessions and refresh-token rotation. A leading underscore keeps this file
// out of the Pages route table.
//
// A refresh token is 32 random bytes; only its SHA-256 is stored (a random
// 256-bit value needs no salt or slow hash). Every refresh replaces it:
//
//   current token  → rotated: a new token, the old hash moves to
//                    refresh_history with its successor and the time;
//   previous token → if the session's current token is its direct successor
//                    and it was rotated less than AUTH_REFRESH_GRACE_SECONDS
//                    ago, this is two tabs refreshing at once: a new access
//                    token, NO new refresh token (the browser already holds the
//                    successor from the other tab's answer);
//   any other rotated token → someone holds a copy of an old token: the whole
//                    session is revoked (theft detection), for both parties;
//   unknown token  → refused, nothing revoked (a guess must not log anyone out).
//
// The grace window is the trade-off: a stolen token replayed within seconds of
// the real rotation gets one access token instead of revoking the session.

import { iso, randomToken, saltedHash, sha256hex } from "./_lib";
import type { Env } from "./_lib";

export interface Session {
  id: string;
  user_id: string;
  refresh_hash: string;
  created_at: string;
  last_used_at: string;
  expires_at: string;
  revoked_at: string | null;
  revoke_reason: string | null;
  ip_hash: string | null;
  ua_hash: string | null;
}

export const live = (s: Session, now: number): boolean => s.revoked_at === null && Date.parse(s.expires_at) > now;

export async function createSession(
  env: Env,
  userId: string,
  request: Request,
  sessionSeconds: number,
  now: number,
  method = "password",
): Promise<{ session: Session; refreshToken: string }> {
  const refreshToken = randomToken();
  const session: Session = {
    id: crypto.randomUUID(),
    user_id: userId,
    refresh_hash: await sha256hex(refreshToken),
    created_at: iso(now),
    last_used_at: iso(now),
    expires_at: iso(now + sessionSeconds * 1000),
    revoked_at: null,
    revoke_reason: null,
    ip_hash: await saltedHash(env, request.headers.get("cf-connecting-ip")),
    ua_hash: await saltedHash(env, request.headers.get("user-agent")),
  };
  const s = session;
  // The way in (GO-151) goes with the session, in the same transaction: the
  // per-role method policy reads it at every token issue (session_methods).
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare(
      `INSERT INTO sessions (id, user_id, refresh_hash, created_at, last_used_at, expires_at, ip_hash, ua_hash)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    ).bind(s.id, s.user_id, s.refresh_hash, s.created_at, s.last_used_at, s.expires_at, s.ip_hash, s.ua_hash),
    env.AUTH_DB.prepare("INSERT INTO session_methods (session_id, method) VALUES (?, ?)").bind(s.id, method),
  ]);
  return { session, refreshToken };
}

export function getSession(env: Env, id: string): Promise<Session | null> {
  return env.AUTH_DB.prepare("SELECT * FROM sessions WHERE id = ?").bind(id).first<Session>();
}

export type Rotation =
  | { kind: "rotated"; session: Session; refreshToken: string }
  | { kind: "grace"; session: Session }
  | { kind: "reused"; sessionId: string }
  | { kind: "invalid" };

export async function rotateRefresh(env: Env, token: string, graceSeconds: number, now: number): Promise<Rotation> {
  const db = env.AUTH_DB;
  const hash = await sha256hex(token);
  const current = await db.prepare("SELECT * FROM sessions WHERE refresh_hash = ?").bind(hash).first<Session>();
  if (current) {
    if (!live(current, now)) return { kind: "invalid" };
    const next = randomToken();
    const nextHash = await sha256hex(next);
    // One transaction: the swap succeeds only if the token is still current
    // (another request may have rotated it a moment ago), and the history row
    // is written only if the swap happened.
    const [swap] = await db.batch([
      db
        .prepare("UPDATE sessions SET refresh_hash = ?, last_used_at = ? WHERE id = ? AND refresh_hash = ? AND revoked_at IS NULL")
        .bind(nextHash, iso(now), current.id, hash),
      db
        .prepare(
          `INSERT INTO refresh_history (hash, session_id, successor_hash, rotated_at)
           SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM sessions WHERE id = ? AND refresh_hash = ?)`,
        )
        .bind(hash, current.id, nextHash, iso(now), current.id, nextHash),
    ]);
    if (swap.meta.changes === 1) {
      return { kind: "rotated", session: { ...current, refresh_hash: nextHash, last_used_at: iso(now) }, refreshToken: next };
    }
    // Lost the race: the token was rotated by the other request — see below.
  }
  const old = await db
    .prepare("SELECT session_id, successor_hash, rotated_at FROM refresh_history WHERE hash = ?")
    .bind(hash)
    .first<{ session_id: string; successor_hash: string; rotated_at: string }>();
  if (!old) return { kind: "invalid" };
  const session = await getSession(env, old.session_id);
  if (!session || !live(session, now)) return { kind: "invalid" };
  if (old.successor_hash === session.refresh_hash && now - Date.parse(old.rotated_at) <= graceSeconds * 1000) {
    return { kind: "grace", session };
  }
  await revokeSession(env, session.id, "refresh_reuse", now);
  return { kind: "reused", sessionId: session.id };
}

// sessionByRefresh finds the live session a refresh token belongs to, without
// rotating anything — what logout and the routes that accept the refresh
// cookie need. The same rules as rotateRefresh (security review SR-01): the
// current token, or the previous one inside the two-tab grace window; any
// other rotated token is a copy someone kept, so the session is revoked
// (theft detection) and nothing is found. Before, a rotated token stayed a
// session for the whole session lifetime on every such route.
export async function sessionByRefresh(env: Env, token: string, graceSeconds: number, now: number): Promise<Session | null> {
  const hash = await sha256hex(token);
  const current = await env.AUTH_DB.prepare("SELECT * FROM sessions WHERE refresh_hash = ?").bind(hash).first<Session>();
  if (current) return live(current, now) ? current : null;
  const old = await env.AUTH_DB.prepare("SELECT session_id, successor_hash, rotated_at FROM refresh_history WHERE hash = ?")
    .bind(hash)
    .first<{ session_id: string; successor_hash: string; rotated_at: string }>();
  const s = old ? await getSession(env, old.session_id) : null;
  if (!old || !s || !live(s, now)) return null;
  if (old.successor_hash === s.refresh_hash && now - Date.parse(old.rotated_at) <= graceSeconds * 1000) return s;
  await revokeSession(env, s.id, "refresh_reuse", now);
  return null;
}

// revokeSession ends one session. Its rotated-token history goes with it: a
// revoked session refuses every token anyway.
export async function revokeSession(env: Env, id: string, reason: string, now: number): Promise<void> {
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("UPDATE sessions SET revoked_at = ?, revoke_reason = ? WHERE id = ? AND revoked_at IS NULL").bind(iso(now), reason, id),
    env.AUTH_DB.prepare("DELETE FROM refresh_history WHERE session_id = ?").bind(id),
  ]);
}

// revokeUserSessions ends every live session of a user except `keep` (the one
// making the request, when it should survive — a password change).
export async function revokeUserSessions(env: Env, userId: string, reason: string, now: number, keep = ""): Promise<number> {
  const [res] = await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("UPDATE sessions SET revoked_at = ?, revoke_reason = ? WHERE user_id = ? AND revoked_at IS NULL AND id != ?").bind(
      iso(now),
      reason,
      userId,
      keep,
    ),
    env.AUTH_DB.prepare(
      "DELETE FROM refresh_history WHERE session_id IN (SELECT id FROM sessions WHERE user_id = ? AND revoked_at IS NOT NULL)",
    ).bind(userId),
  ]);
  return res.meta.changes;
}

// purgeStale drops what can no longer matter: history of ended sessions,
// sessions ended more than 30 days ago (kept that long for the session list),
// failure counters that are neither recent nor locking anything, and the
// sign-in methods of sessions no longer kept.
export async function purgeStale(env: Env, now: number): Promise<void> {
  const db = env.AUTH_DB;
  const at = iso(now);
  const month = iso(now - 30 * 86_400_000);
  await db.batch([
    db.prepare("DELETE FROM refresh_history WHERE session_id IN (SELECT id FROM sessions WHERE expires_at <= ? OR revoked_at IS NOT NULL)").bind(at),
    db.prepare("DELETE FROM sessions WHERE expires_at <= ? OR revoked_at <= ?").bind(month, month),
    db.prepare("DELETE FROM login_attempts WHERE last_failure_at <= ? AND (locked_until IS NULL OR locked_until <= ?)").bind(iso(now - 86_400_000), at),
    // The ways in of sessions dropped above. Swept here, after a sign-in,
    // rather than on every anonymous OAuth start (SR-09).
    db.prepare("DELETE FROM session_methods WHERE session_id NOT IN (SELECT id FROM sessions)"),
  ]);
}
