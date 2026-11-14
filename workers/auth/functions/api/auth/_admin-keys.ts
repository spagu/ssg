// How old the signing keys are (GO-138). A JWK carries no date, so the worker
// writes down when it FIRST signed with each key id: the admin panel's Status
// view shows that age next to the rotation advice, and the audit log gets a
// `signing_key_seen` line whenever a new key id appears — a rotation. A
// leading underscore keeps this file out of the Pages route table.
//
//   signing_keys  kid (the RFC 7638 thumbprint, public) → first_used_at.
//                 A key used before this table existed counts from the first
//                 token signed after the upgrade: an age "at least".
//
// noteKey runs at every token issue but writes at most once per key id per
// isolate and database (a module-level memo), and never fails the sign-in it
// rides on.

import { emitEvent } from "./_audit";
import { iso } from "./_lib";
import type { Env } from "./_lib";

export const KEY_TABLES = [
  `CREATE TABLE IF NOT EXISTS signing_keys (
     kid           TEXT PRIMARY KEY,
     first_used_at TEXT NOT NULL
   )`,
];

// ROTATION_DAYS: the README's advice — a new signing key at least yearly, and
// at once when the old one may have leaked.
export const ROTATION_DAYS = 365;

const noted = new WeakMap<D1Database, Set<string>>();

// noteKey records the first use of a key id (once per isolate and database).
export async function noteKey(env: Env, kid: string, now: number): Promise<void> {
  const seen = noted.get(env.AUTH_DB) ?? new Set<string>();
  if (seen.has(kid)) return;
  try {
    const res = await env.AUTH_DB.prepare("INSERT OR IGNORE INTO signing_keys (kid, first_used_at) VALUES (?, ?)").bind(kid, iso(now)).run();
    noted.set(env.AUTH_DB, seen.add(kid));
    if (res.meta.changes === 1) await emitEvent(env, { kind: "signing_key_seen", detail: { kid } }, now);
  } catch {
    console.error("auth: signing key first use not recorded");
  }
}

export interface KeyAge {
  kid: string;
  current: boolean; // signs new tokens; otherwise only still verified (AUTH_PREVIOUS_PUBLIC_KEY)
  firstUsedAt: string | null; // null: never signed here (a previous key) or not recorded yet
  ageDays: number | null;
  rotate: boolean; // older than ROTATION_DAYS
}

// keyAges: each published key id with what is known of its age.
export async function keyAges(env: Env, kids: readonly string[], current: string, now: number): Promise<KeyAge[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT kid, first_used_at FROM signing_keys WHERE kid IN (SELECT value FROM json_each(?))")
    .bind(JSON.stringify(kids))
    .all<{ kid: string; first_used_at: string }>();
  const seen = new Map(results.map((r) => [r.kid, r.first_used_at]));
  return kids.map((kid) => {
    const at = seen.get(kid) ?? null;
    const ageDays = at === null ? null : Math.floor((now - Date.parse(at)) / 86_400_000);
    return { kid, current: kid === current, firstUsedAt: at, ageDays, rotate: kid === current && ageDays !== null && ageDays >= ROTATION_DAYS };
  });
}
