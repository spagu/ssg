// Everyone's live sessions, for the admin panel (GO-138): who, how they came
// in, whether a second factor was passed, when. A leading underscore keeps
// this file out of the Pages route table. Ending one session is
// _sessions.ts's revokeSession; ending every account's is here.
//
// Nothing that could resume a session is read: no refresh hash, no IP or
// User-Agent hash — only ids, the way in and times.

import { iso } from "./_lib";
import type { Env } from "./_lib";

export interface LiveSession {
  id: string;
  userId: string;
  email: string;
  name: string;
  method: string | null; // password | a provider | access (null: made before GO-151)
  secondFactor: string | null; // totp | backup | passkey — passed in this session; null: none
  createdAt: string;
  lastUsedAt: string;
  expiresAt: string;
}

interface Row {
  id: string;
  user_id: string;
  email: string;
  display_name: string;
  method: string | null;
  factor: string | null;
  created_at: string;
  last_used_at: string;
  expires_at: string;
}

// The cursor is "<lastUsedAt>|<id>" of the previous page's last session:
// newest use first, the id breaking ties.
const splitCursor = (c: string): [string, string] => {
  const at = c.indexOf("|");
  return at < 0 ? ["\uffff", ""] : [c.slice(0, at), c.slice(at + 1)];
};

// liveSessions: one page of live sessions, all accounts or one (`userId`).
export async function liveSessions(env: Env, now: number, userId: string, cursor: string, limit: number): Promise<{ items: LiveSession[]; nextCursor: string | null }> {
  const [at, id] = cursor ? splitCursor(cursor) : ["\uffff", ""];
  const { results } = await env.AUTH_DB.prepare(
    `SELECT s.id, s.user_id, u.email, u.display_name, sm.method, sf.factor, s.created_at, s.last_used_at, s.expires_at
       FROM sessions s
       JOIN users u ON u.id = s.user_id
       LEFT JOIN session_methods sm ON sm.session_id = s.id
       LEFT JOIN session_factors sf ON sf.session_id = s.id
      WHERE s.revoked_at IS NULL AND s.expires_at > ?
        AND (? = '' OR s.user_id = ?)
        AND (s.last_used_at < ? OR (s.last_used_at = ? AND s.id < ?))
      ORDER BY s.last_used_at DESC, s.id DESC LIMIT ?`,
  )
    .bind(iso(now), userId, userId, at, at, id === "" ? "\uffff" : id, limit + 1)
    .all<Row>();
  const items = results.slice(0, limit).map((r) => ({
    id: r.id,
    userId: r.user_id,
    email: r.email,
    name: r.display_name,
    method: r.method,
    secondFactor: r.factor,
    createdAt: r.created_at,
    lastUsedAt: r.last_used_at,
    expiresAt: r.expires_at,
  }));
  const last = items[items.length - 1];
  return { items, nextCursor: results.length > limit ? `${last.lastUsedAt}|${last.id}` : null };
}

// revokeEveryone ends every live session of every account except `keep` (the
// administrator's own, so the break-glass button does not lock them out of
// the panel that pressed it). Returns how many ended.
export async function revokeEveryone(env: Env, keep: string, now: number): Promise<number> {
  const db = env.AUTH_DB;
  const [res] = await db.batch([
    db.prepare("UPDATE sessions SET revoked_at = ?, revoke_reason = 'admin_revoke_all' WHERE revoked_at IS NULL AND id != ?").bind(iso(now), keep),
    db.prepare("DELETE FROM refresh_history WHERE session_id IN (SELECT id FROM sessions WHERE revoked_at IS NOT NULL)"),
  ]);
  return res.meta.changes;
}
