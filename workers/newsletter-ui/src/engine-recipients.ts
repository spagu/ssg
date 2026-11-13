// Who a batch goes to, and the last check before each message.
//
// A batch is a keyset range of subscribers.id, (after, until]. Selecting it
// applies the campaign's segment (segmentWhere: confirmed + languages + tags)
// and both steps of the suppression exclusion: NOT_SUPPRESSED_SQL in SQL for
// suppressions that kept the address, then dropHashOnly in the worker for
// hash-only ones — with the owner's rule that a person who gave fresh consent
// after an erasure is mailed again (suppressionApplies).
//
// The last check (CHECKS, verdictOf) runs immediately before each message,
// because minutes can pass between selecting a batch and reaching a row: an
// unsubscribe, an erasure or a new suppression in that time takes effect for
// the very next message. It reads the campaign's status too, so a pause or
// cancel stops the batch before the next message rather than after the batch.

import type { Env } from "./lib";
import type { Segment } from "./segment";
import { segmentWhere } from "./segment";
import { NOT_SUPPRESSED_SQL, dropHashOnly, suppressionApplies } from "./suppress";

export interface Candidate {
  id: string;
  email: string;
  language: string | null;
  token: string;
  consent_at: string | null;
}

// selectRange returns up to `limit` recipients in (from, until], ordered by
// id, and whether SQL returned the full `limit` (then the range may hold more
// rows than were read, and the caller continues after the last one).
export async function selectRange(
  env: Env,
  segment: Segment,
  from: string,
  until: string,
  limit: number,
): Promise<{ rows: Candidate[]; full: boolean; lastId: string | null }> {
  const { sql, binds } = segmentWhere(segment);
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT id, email, language, token, consent_at FROM subscribers s
      WHERE ${sql} AND id > ? AND id <= ? AND ${NOT_SUPPRESSED_SQL("s")}
      ORDER BY id LIMIT ?`,
  )
    .bind(...binds, from, until, limit)
    .all<Candidate>();
  const lastId = results.length ? results[results.length - 1].id : null;
  return { rows: await dropHashOnly(env, results, true), full: results.length === limit, lastId };
}

// countRecipients is total_count when a campaign starts: the segment minus
// suppressions that kept the address. Hash-only suppressions are not hashed
// here (that would mean hashing every address in the cron); they are erased
// people, almost never on the list again, and the batch drops them.
export async function countRecipients(env: Env, segment: Segment): Promise<number> {
  const { sql, binds } = segmentWhere(segment);
  const row = await env.NEWSLETTER_DB.prepare(`SELECT COUNT(*) AS n FROM subscribers s WHERE ${sql} AND ${NOT_SUPPRESSED_SQL("s")}`)
    .bind(...binds)
    .first<{ n: number }>();
  return Number((row as { n: number }).n);
}

// selectRetries is the read of a range reopened for re-sends only
// (send_batches.retry_only): the ledger rows in `retry`, whatever the segment
// says now — the last check (CHECKS below) decides whether each still gets a
// message. A subscriber erased since comes back with an empty address and is
// skipped by that check before anything is rendered.
export async function selectRetries(
  env: Env,
  campaignId: string,
  from: string,
  until: string,
  limit: number,
): Promise<{ rows: Candidate[]; full: boolean; lastId: string | null }> {
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT cs.subscriber_id AS id, COALESCE(s.email, '') AS email, s.language, COALESCE(s.token, '') AS token, s.consent_at
       FROM campaign_sends cs LEFT JOIN subscribers s ON s.id = cs.subscriber_id
      WHERE cs.campaign_id = ? AND cs.status = 'retry' AND cs.subscriber_id > ? AND cs.subscriber_id <= ?
      ORDER BY cs.subscriber_id LIMIT ?`,
  )
    .bind(campaignId, from, until, limit)
    .all<Candidate>();
  const lastId = results.length ? results[results.length - 1].id : null;
  return { rows: results, full: results.length === limit, lastId };
}

// What the last check found: go ahead, skip this person (with the reason
// stored on their send row), or stop the batch (the campaign left `live`).
export type Verdict = { go: true } | { go: false; skip: string } | { go: false; stop: string };

// CHECKS are the facts of the last check as scalar subqueries, so the claim of
// a row can return them in the same statement (engine-claim.ts claimOne): one
// D1 query per message for both. Parameters: ?1 campaign id, ?2 subscriber
// id, ?5 the address's suppression key.
export const CHECKS = `(SELECT status FROM campaigns WHERE id = ?1) AS campaign_status,
    (SELECT status FROM subscribers WHERE id = ?2) AS sub_status,
    (SELECT consent_at FROM subscribers WHERE id = ?2) AS consent_at,
    (SELECT reason FROM suppressions WHERE email_hash = ?5) AS reason,
    (SELECT email FROM suppressions WHERE email_hash = ?5) AS x_email,
    (SELECT updated_at FROM suppressions WHERE email_hash = ?5) AS x_updated`;

export type Facts = Record<"campaign_status" | "sub_status" | "consent_at" | "reason" | "x_email" | "x_updated", string | null>;

// verdictOf reads the facts. `live` is the campaign status the batch runs
// under: `sending`, or `sent` for re-sends of a finished campaign.
export function verdictOf(row: Facts, live: string): Verdict {
  if (row.campaign_status === null) return { go: false, stop: "deleted" };
  if (row.campaign_status !== live) return { go: false, stop: row.campaign_status };
  if (row.sub_status === null) return { go: false, skip: "subscriber_gone" };
  if (row.sub_status !== "confirmed") return { go: false, skip: "not_confirmed" };
  if (row.reason !== null) {
    const facts = { reason: row.reason, email: row.x_email, updatedAt: row.x_updated as string };
    if (suppressionApplies(facts, row.consent_at)) return { go: false, skip: "suppressed" };
  }
  return { go: true };
}
