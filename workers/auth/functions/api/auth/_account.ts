// The signed-in person's own sessions, as the account page and
// GET/DELETE /api/auth/sessions see them. A leading underscore keeps this
// file out of the Pages route table. Never a hash, an IP or a User-Agent: the
// list says when, not from where (the worker stores only salted hashes of
// those, and only with AUTH_HASH_SALT).

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { revokeSession } from "./_sessions";

export interface OwnSession {
  id: string;
  createdAt: string;
  lastUsedAt: string;
  expiresAt: string;
  current: boolean;
}

// listOwnSessions: the live sessions of userId, most recently used first;
// `current` marks the one making the request.
export async function listOwnSessions(env: Env, userId: string, currentId: string, now: number): Promise<OwnSession[]> {
  const { results } = await env.AUTH_DB.prepare(
    `SELECT id, created_at, last_used_at, expires_at FROM sessions
     WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ? ORDER BY last_used_at DESC, created_at DESC, id`,
  )
    .bind(userId, iso(now))
    .all<{ id: string; created_at: string; last_used_at: string; expires_at: string }>();
  return results.map((r) => ({ id: r.id, createdAt: r.created_at, lastUsedAt: r.last_used_at, expiresAt: r.expires_at, current: r.id === currentId }));
}

// endOwnSession ends one live session of userId; someone else's session, an
// ended one and an unknown id are all "not_found" — the answer never tells
// whether an id exists for another account.
export async function endOwnSession(env: Env, userId: string, id: string, now: number): Promise<"ok" | "not_found"> {
  const row = await env.AUTH_DB.prepare("SELECT id FROM sessions WHERE id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?")
    .bind(id, userId, iso(now))
    .first<{ id: string }>();
  if (!row) return "not_found";
  await revokeSession(env, id, "user_signout", now);
  return "ok";
}
