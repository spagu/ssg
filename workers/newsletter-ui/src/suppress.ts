// The suppression list: addresses that must not be mailed or imported again.
//
// A row is keyed by suppressionKey(email) = sha256(trimmed lower-case address),
// UNSALTED on purpose: the key must survive a change of NEWSLETTER_HASH_SALT and
// must match the same address arriving later in an import. `email` holds the
// address in clear text when it is useful to show (a bounce), and is NULL when
// it must not be kept (a GDPR erasure): the hash alone still blocks the address.
//
// Set-based exclusion. SQLite (and so D1) cannot compute SHA-256, so a query
// over `subscribers` cannot join suppressions by hash. Exclusion is two steps:
//   1. NOT_SUPPRESSED_SQL(alias) in the WHERE clause drops every subscriber
//      whose address is suppressed WITH the address kept — the common case
//      (bounces, complaints), done in SQL with the idx_suppressions_email index;
//   2. dropHashOnly(env, rows) removes the rest — suppressions whose address
//      was erased — by hashing the candidates in the worker. It reads the
//      hash-only keys once (a small set: erasures, whose subscriber rows were
//      deleted with them) and hashes nothing at all when there are none.
// A hash-only suppression can only match a subscriber who signed up again
// after being erased, so step 2 is almost always a no-op.

import { sha256hex } from "./lib";
import type { Env } from "./lib";

export type SuppressionReason = "hard" | "soft" | "complaint" | "manual";

export interface Suppression {
  emailHash: string;
  email: string | null;
  reason: string;
  count: number;
  source: string | null;
  createdAt: string;
  updatedAt: string;
}

export const suppressionKey = (email: string): Promise<string> => sha256hex(email.trim().toLowerCase());

// NOT_SUPPRESSED_SQL is a WHERE fragment for a query over subscribers aliased
// as `alias`. subscribers.email is stored lower-case and trimmed by the sign-up
// form, and suppress() stores the same form, so plain equality is exact.
export const NOT_SUPPRESSED_SQL = (alias: string): string =>
  `NOT EXISTS (SELECT 1 FROM suppressions sx WHERE sx.email = ${alias}.email)`;

// What decides whether a suppression still stops mail to its address.
export interface SuppressionFacts {
  reason: string;
  email: string | null; // null = hash-only (an erasure, or a manual block without the address)
  updatedAt: string;
}

// suppressionApplies is the owner's rule for a person who signed up again: a
// hash-only `manual` suppression (that is what an erasure writes) stops
// applying once the subscriber's consent_at is newer than the suppression —
// the person gave fresh consent after being erased. `hard`, `soft` and
// `complaint` suppressions, and any that kept the address, always apply. An
// unreadable date fails closed: the suppression applies.
export function suppressionApplies(s: SuppressionFacts, consentAt: string | null): boolean {
  if (s.email !== null || s.reason !== "manual" || consentAt === null) return true;
  return !(Date.parse(consentAt) > Date.parse(s.updatedAt));
}

// hashOnly returns the suppressions without a clear-text address, by key.
async function hashOnly(env: Env): Promise<Map<string, SuppressionFacts>> {
  const { results } = await env.NEWSLETTER_DB.prepare("SELECT email_hash, reason, updated_at FROM suppressions WHERE email IS NULL").all<{
    email_hash: string;
    reason: string;
    updated_at: string;
  }>();
  return new Map(results.map((r) => [r.email_hash, { reason: r.reason, email: null, updatedAt: r.updated_at }]));
}

// hashOnlyKeys returns the keys of suppressions without a clear-text address.
export async function hashOnlyKeys(env: Env): Promise<Set<string>> {
  return new Set((await hashOnly(env)).keys());
}

// dropHashOnly removes rows whose address matches a hash-only suppression.
// Call it on the result of a query that already used NOT_SUPPRESSED_SQL.
// With `freshConsentWins` (the send engine) each row must carry its
// consent_at, and suppressionApplies decides: someone who signed up again
// after an erasure is kept.
export async function dropHashOnly<T extends { email: string; consent_at?: string | null }>(
  env: Env,
  rows: T[],
  freshConsentWins = false,
): Promise<T[]> {
  const found = await hashOnly(env);
  if (found.size === 0) return rows;
  const out: T[] = [];
  for (const row of rows) {
    const s = found.get(await suppressionKey(row.email));
    if (!s || (freshConsentWins && !suppressionApplies(s, row.consent_at ?? null))) out.push(row);
  }
  return out;
}

// findSuppression returns the suppression for an address, or null.
export async function findSuppression(env: Env, email: string): Promise<Suppression | null> {
  const row = await env.NEWSLETTER_DB.prepare(
    "SELECT email_hash, email, reason, count, source, created_at, updated_at FROM suppressions WHERE email_hash = ?",
  )
    .bind(await suppressionKey(email))
    .first<Record<string, unknown>>();
  if (!row) return null;
  return {
    emailHash: row.email_hash as string,
    email: row.email as string | null,
    reason: row.reason as string,
    count: row.count as number,
    source: row.source as string | null,
    createdAt: row.created_at as string,
    updatedAt: row.updated_at as string,
  };
}

export const isSuppressed = async (env: Env, email: string): Promise<boolean> => (await findSuppression(env, email)) !== null;

export interface SuppressInput {
  email: string;
  reason: SuppressionReason;
  source: string;
  keepEmail: boolean; // false: store the hash only (erasure)
}

// SUPPRESSION_UPSERT is the ON CONFLICT clause every write to suppressions
// shares (suppressStatement here, the soft-bounce promotion and the CSV import
// in bounce-store.ts / suppression-import.ts), so they all merge one way:
//   - reason: `complaint` outranks `hard`, which outranks `soft` and `manual`;
//     a weaker reason never replaces a stronger one (an erasure or a manual
//     block must not turn an absolute complaint into a liftable `manual`),
//     between equals the newer one wins. `source` follows the reason kept.
//   - email: once dropped (an erasure) it stays dropped; a later bounce for
//     the same hash never brings the clear-text address back.
//   - count grows by one per write (soft bounces in a row), updated_at moves.
const RANK = (r: string) => `(CASE ${r} WHEN 'complaint' THEN 3 WHEN 'hard' THEN 2 ELSE 1 END)`;
// reasonRank is the same order in the worker, for a decision made before the
// write (the import's preview); the write itself still merges by the SQL.
export const reasonRank = (r: string): number => (r === "complaint" ? 3 : r === "hard" ? 2 : 1);
export const SUPPRESSION_UPSERT = `ON CONFLICT(email_hash) DO UPDATE SET
       email = CASE WHEN suppressions.email IS NULL OR excluded.email IS NULL THEN NULL ELSE excluded.email END,
       reason = CASE WHEN ${RANK("suppressions.reason")} > ${RANK("excluded.reason")} THEN suppressions.reason ELSE excluded.reason END,
       source = CASE WHEN ${RANK("suppressions.reason")} > ${RANK("excluded.reason")} THEN suppressions.source ELSE excluded.source END,
       count = suppressions.count + 1, updated_at = excluded.updated_at`;

// suppressStatement builds the upsert, so a caller can put it in a db.batch
// together with other writes (erasure deletes and suppresses atomically).
// Repeats merge as SUPPRESSION_UPSERT says.
export async function suppressStatement(env: Env, s: SuppressInput): Promise<D1PreparedStatement> {
  const email = s.email.trim().toLowerCase();
  const at = new Date().toISOString();
  return env.NEWSLETTER_DB.prepare(
    `INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at)
     VALUES (?, ?, ?, 1, ?, ?, ?)
     ${SUPPRESSION_UPSERT}`,
  ).bind(await suppressionKey(email), s.keepEmail ? email : null, s.reason, s.source, at, at);
}

export async function suppress(env: Env, s: SuppressInput): Promise<void> {
  await (await suppressStatement(env, s)).run();
}

// unsuppress lifts a suppression by its key. Returns whether one existed.
export async function unsuppress(env: Env, emailHash: string): Promise<boolean> {
  const res = await env.NEWSLETTER_DB.prepare("DELETE FROM suppressions WHERE email_hash = ?").bind(emailHash).run();
  return res.meta.changes > 0;
}

// suppressedKeys returns which of the given keys are suppressed, in chunks a
// single D1 statement can take (one JSON parameter per chunk, not one each).
export async function suppressedKeys(env: Env, keys: string[], chunk = 1000): Promise<Set<string>> {
  const found = new Set<string>();
  for (let i = 0; i < keys.length; i += chunk) {
    const { results } = await env.NEWSLETTER_DB.prepare(
      "SELECT email_hash FROM suppressions WHERE email_hash IN (SELECT value FROM json_each(?))",
    )
      .bind(JSON.stringify(keys.slice(i, i + chunk)))
      .all<{ email_hash: string }>();
    for (const r of results) found.add(r.email_hash);
  }
  return found;
}
