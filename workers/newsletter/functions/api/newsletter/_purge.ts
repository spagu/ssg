// The retention purge (#465). A leading underscore keeps this file out of the
// Pages route table — _store.ts re-exports purgeExpired for the endpoints.
//
// Two kinds of row expire, on different timescales: an unconfirmed `pending`
// sign-up is worthless after days or weeks, while an `unsubscribed` row is the
// record that consent was withdrawn and is often kept for years. Each status
// has its own period:
//
//   NEWSLETTER_PENDING_RETENTION_DAYS       pending, counted from the latest of
//                                           created_at, consent_at, confirm_sent_at
//   NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS  unsubscribed, counted from
//                                           unsubscribed_at (updated_at when NULL)
//
// Unset or blank, a status falls back to NEWSLETTER_RETENTION_DAYS counted
// from updated_at — the rule before #465, kept byte for byte when neither is
// set. Confirmed subscribers are never purged.

import { retentionDays } from "./_lib";
import type { Env } from "./_lib";

// The clock a period counts from, as an SQL expression over one row.
const UPDATED = "updated_at";
// The latest of the three: subscribe() turns an old row back into `pending`
// with a fresh consent_at, and the purge may run (in the background of that
// same request) before markConfirmSent stamps the new mail, so an old
// confirm_sent_at alone could delete a sign-up made seconds ago. ISO 8601
// strings order as text; created_at and consent_at are NOT NULL, and SQLite's
// multi-argument MAX returns NULL if any argument is, hence the COALESCE.
const PENDING_CLOCK = "MAX(created_at, consent_at, COALESCE(confirm_sent_at, created_at))";
// An import stamps updated_at with the import day; unsubscribed_at is the
// moment consent was withdrawn.
const UNSUBSCRIBED_CLOCK = "COALESCE(unsubscribed_at, updated_at)";

// Beyond this many days nothing is old enough to delete; the cap keeps the
// cutoff a valid Date (an absurd value must not make the purge throw).
const MAX_DAYS = 1_000_000;

export interface StatusRetention {
  days: number; // 0 = keep this status forever
  clock: string; // SQL expression the period counts from
}

export interface Retention {
  pending: StatusRetention;
  unsubscribed: StatusRetention;
}

// specificDays reads a per-status variable: null when unset or blank (fall back
// to NEWSLETTER_RETENTION_DAYS), N for a whole number, 0 — keep — for anything
// else. A typo must never delete rows.
export function specificDays(value: string | undefined): number | null {
  const text = (value ?? "").trim();
  if (!text) return null;
  return /^\d+$/.test(text) ? Number(text) : 0;
}

// retentionPolicy: the effective period and clock of each purged status.
export function retentionPolicy(env: Env): Retention {
  const fallback = retentionDays(env);
  const rule = (value: string | undefined, clock: string): StatusRetention => {
    const days = specificDays(value);
    return days === null ? { days: fallback, clock: UPDATED } : { days, clock };
  };
  return {
    pending: rule(env.NEWSLETTER_PENDING_RETENTION_DAYS, PENDING_CLOCK),
    unsubscribed: rule(env.NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS, UNSUBSCRIBED_CLOCK),
  };
}

// Per-isolate timestamp of the last purge: the lazy purge piggybacks on writes
// and runs at most hourly per isolate, so a busy sign-up form does not turn
// into a DELETE per request.
let lastPurge = 0;
const PURGE_EVERY_MS = 60 * 60 * 1000;

async function deleteOlder(env: Env, where: string, days: number): Promise<number> {
  const cutoff = new Date(Date.now() - Math.min(days, MAX_DAYS) * 86400000).toISOString();
  const res = await env.NEWSLETTER_DB.prepare(`DELETE FROM subscribers WHERE ${where} < ?`).bind(cutoff).run();
  return res.meta.changes ?? 0;
}

// purgeExpired deletes pending and unsubscribed rows past their retention.
// `force` skips the hourly throttle (the admin endpoint). Returns the rows
// deleted, both statuses together.
export async function purgeExpired(env: Env, force = false): Promise<number> {
  const { pending, unsubscribed } = retentionPolicy(env);
  if ((!pending.days && !unsubscribed.days) || (!force && Date.now() - lastPurge < PURGE_EVERY_MS)) return 0;
  lastPurge = Date.now();
  // Both on the old rule: the old single statement (one period, then).
  if (pending.clock === UPDATED && unsubscribed.clock === UPDATED) {
    return deleteOlder(env, "status IN ('pending', 'unsubscribed') AND updated_at", pending.days);
  }
  let deleted = 0;
  for (const [status, rule] of [["pending", pending], ["unsubscribed", unsubscribed]] as const) {
    if (rule.days) deleted += await deleteOlder(env, `status = '${status}' AND ${rule.clock}`, rule.days);
  }
  return deleted;
}
