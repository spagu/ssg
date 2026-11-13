// campaign_sends, the engine's ledger: one row per campaign and subscriber,
// UNIQUE(campaign_id, subscriber_id). Idempotence lives here, not in hoping a
// queue job is delivered once.
//
//   (no row) ──claim──▶ claimed ──▶ sent | failed                 final
//                         │    ├──▶ retry (temporary failure, attempts < max)
//                         │    │       └──claim──▶ claimed …
//                         │    └──▶ skipped (unsubscribed / suppressed at the last check)
//                         └──(stale)──▶ uncertain ──admin──▶ retry (resend) | skipped (dismiss)
//
// A row is claimed immediately before ITS message goes out — one row at a
// time, never a whole batch ahead — so at any moment a consumer holds at most
// one claim. "No row" always means "never handed to the transport", and a
// consumer that dies, however it dies (even killed by the platform without
// running a `finally`), leaves at most that one row `claimed`. A `claimed`
// row younger than the stale timeout belongs to a consumer that may be
// mid-send and is left alone; an older one becomes `uncertain`: the provider
// may or may not have accepted it, and the engine never decides that by
// sending again. The admin does (src/engine-uncertain.ts). Every claim counts
// an attempt, so a temporary failure ends `failed` after max attempts.
//
// D1 cost per message: 2 statements — claimOne (claim + the last check in one
// statement) and record. It used to be 2 per message plus 1 claim per batch.

import type { Env } from "./lib";
import type { Candidate, Facts, Verdict } from "./engine-recipients";
import { CHECKS, verdictOf } from "./engine-recipients";
import { suppressionKey } from "./suppress";

const FINAL = new Set(["sent", "failed", "skipped", "uncertain"]);

export const UNCERTAIN_DETAIL = "no result: the consumer stopped while this message was being handed to the transport";

// staleBefore is the claimed_at below which a claim is abandoned. ISO 8601
// strings in UTC compare correctly as text.
const staleBefore = (stamp: string, staleMs: number): string => new Date(Date.parse(stamp) - staleMs).toISOString();

interface SendRow {
  subscriber_id: string;
  status: string;
  attempts: number;
  claimed_at: string;
}

export interface Sorted {
  todo: Candidate[]; // no row, or `retry` with attempts left
  busy: Candidate[]; // claimed by a consumer that may still be sending
  abandoned: Candidate[]; // stale claims (→ uncertain) and `retry` rows out of attempts (→ failed)
}

// sortRows reads the ledger for the candidates and sorts them.
export async function sortRows(env: Env, campaignId: string, rows: Candidate[], staleMs: number, maxAttempts: number): Promise<Sorted> {
  const out: Sorted = { todo: [], busy: [], abandoned: [] };
  if (!rows.length) return out;
  const { results } = await env.NEWSLETTER_DB.prepare(
    "SELECT subscriber_id, status, attempts, claimed_at FROM campaign_sends WHERE campaign_id = ? AND subscriber_id IN (SELECT value FROM json_each(?))",
  )
    .bind(campaignId, JSON.stringify(rows.map((r) => r.id)))
    .all<SendRow>();
  const ledger = new Map(results.map((r) => [r.subscriber_id, r]));
  const cutoff = Date.now() - staleMs;
  for (const r of rows) {
    const s = ledger.get(r.id);
    if (!s) out.todo.push(r);
    else if (FINAL.has(s.status)) continue;
    else if (s.status === "retry") (s.attempts < maxAttempts ? out.todo : out.abandoned).push(r);
    else if (Date.parse(s.claimed_at) >= cutoff) out.busy.push(r);
    else out.abandoned.push(r);
  }
  return out;
}

// settleAbandoned closes what sortRows found abandoned, in one statement: a
// stale claim becomes `uncertain` (never re-sent automatically), a `retry`
// row without attempts left (the limit was lowered meanwhile) `failed`. The
// WHERE repeats both conditions, so a claim taken a moment ago is untouched.
export async function settleAbandoned(env: Env, campaignId: string, ids: string[], staleMs: number, maxAttempts: number): Promise<void> {
  if (!ids.length) return;
  await env.NEWSLETTER_DB.prepare(
    `UPDATE campaign_sends
        SET status = CASE status WHEN 'claimed' THEN 'uncertain' ELSE 'failed' END,
            error = CASE status WHEN 'claimed' THEN ?1 ELSE ?2 END
      WHERE campaign_id = ?3 AND subscriber_id IN (SELECT value FROM json_each(?4))
        AND ((status = 'claimed' AND claimed_at < ?5) OR (status = 'retry' AND attempts >= ?6))`,
  )
    .bind(UNCERTAIN_DETAIL, `no result after ${maxAttempts} attempts`, campaignId, JSON.stringify(ids), staleBefore(new Date().toISOString(), staleMs), maxAttempts)
    .run();
}

export interface Claimed {
  attempts: number;
  verdict: Verdict;
}

// claimOne claims one row and runs the last check in the same statement. null
// when the row is not this consumer's to send: final, claimed by another
// consumer, or out of attempts. A claim whose verdict is not `go` must be
// handed back (release) or recorded `skipped` by the caller.
export async function claimOne(env: Env, campaignId: string, r: Candidate, stamp: string, maxAttempts: number, live: string): Promise<Claimed | null> {
  const row = await env.NEWSLETTER_DB.prepare(
    `INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?1, ?2, 'claimed', 1, ?3)
     ON CONFLICT (campaign_id, subscriber_id) DO UPDATE SET
       status = 'claimed', attempts = campaign_sends.attempts + 1, claimed_at = excluded.claimed_at
     WHERE campaign_sends.status = 'retry' AND campaign_sends.attempts < ?4
     RETURNING attempts, ${CHECKS}`,
  )
    .bind(campaignId, r.id, stamp, maxAttempts, await suppressionKey(r.email))
    .first<Facts & { attempts: number }>();
  return row ? { attempts: Number(row.attempts), verdict: verdictOf(row, live) } : null;
}

// release hands claimed rows back untouched (the attempt is not counted): the
// message was never handed to the transport — a pause, a cancel, a render
// error, a database error before the send, or a tripped bounce breaker.
// `stamp` limits it to this consumer's own claim; null releases `failed` rows
// (the breaker's hard bounces) instead.
export async function release(env: Env, campaignId: string, ids: string[], stamp: string | null): Promise<void> {
  await env.NEWSLETTER_DB.prepare(
    `UPDATE campaign_sends SET status = 'retry', attempts = attempts - 1, error = NULL
      WHERE campaign_id = ?1 AND subscriber_id IN (SELECT value FROM json_each(?2))
        AND (CASE WHEN ?3 IS NULL THEN status = 'failed' ELSE status = 'claimed' AND claimed_at = ?3 END)`,
  )
    .bind(campaignId, JSON.stringify(ids), stamp)
    .run();
}

export type Outcome = "sent" | "failed" | "retry" | "skipped";

// record writes a row's outcome. `skipped` does not count as an attempt: no
// message was handed to the transport.
export async function record(env: Env, campaignId: string, subscriberId: string, outcome: Outcome, detail: string | null): Promise<void> {
  await env.NEWSLETTER_DB.prepare(
    `UPDATE campaign_sends SET status = ?1, error = ?2, sent_at = CASE WHEN ?1 = 'sent' THEN ?3 ELSE sent_at END,
       attempts = attempts - (CASE WHEN ?1 = 'skipped' THEN 1 ELSE 0 END)
      WHERE campaign_id = ?4 AND subscriber_id = ?5`,
  )
    .bind(outcome, detail, new Date().toISOString(), campaignId, subscriberId)
    .run();
}
