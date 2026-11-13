// Uncertain sends: the admin's side of the one ambiguity left in the engine.
//
// A campaign_sends row is `uncertain` when the consumer that claimed it died
// while handing its message to the transport (engine-claim.ts): the provider
// may or may not have accepted it. The engine never re-sends such a row by
// itself — a possible duplicate is worse than one visibly missing message —
// so the admin decides:
//   GET  {base}/api/campaigns/:id/uncertain                 who, and since when
//   POST {base}/api/campaigns/:id/uncertain/resend {ids?}   one more attempt
//   POST {base}/api/campaigns/:id/uncertain/dismiss {ids?}  close as `skipped`
// `ids` omitted = every uncertain row of the campaign.
//
// A re-send goes through the normal path: the rows go back to `retry` with
// exactly one attempt left, and the ranges holding them are reopened with
// retry_only = 1 — the job reads those ledger rows instead of the segment, so
// nobody who signed up since is mailed, and the last check (still confirmed,
// not suppressed), the rate budget and the idempotency key all apply. It works
// on a `sent` campaign without reopening it: the status stays `sent` and
// finished_at keeps marking the end of the campaign's run; the counters follow
// the re-sent rows as they finish.

import { loadCampaign, readJson } from "./campaign-model";
import { jobFor } from "./engine-batches";
import { engineConfig } from "./engine-rate";
import type { Env } from "./lib";
import { json } from "./lib";
import { honoursIdempotencyKey, transportOf } from "./mail/_transport";
import type { RouteContext } from "./router";

export const DEFAULT_LIMIT = 50;
export const MAX_LIMIT = 200;
export const MAX_IDS = 1000;
// Resend keeps an idempotency key for 24 hours; after that a re-send is new.
export const KEY_WINDOW_MS = 24 * 3_600_000;

const RESENDABLE = new Set(["sending", "paused", "sent"]);

// GET → {items: [{subscriberId, email, claimedAt, safeResend}], nextCursor,
// transport, safeResend}. email is null for a subscriber erased since.
// safeResend (top level): the active transport honours the idempotency key;
// per item: and the key is still within the provider's 24-hour window.
export async function listUncertain({ env, params, url }: RouteContext): Promise<Response> {
  const raw = url.searchParams.get("limit");
  const limit = raw === null ? DEFAULT_LIMIT : Number(raw);
  if (!Number.isInteger(limit) || limit < 1 || limit > MAX_LIMIT) {
    return json({ error: "invalid_limit", detail: `limit must be an integer from 1 to ${MAX_LIMIT}` }, 400);
  }
  if (!(await loadCampaign(env, params.id))) return json({ error: "not_found" }, 404);
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT cs.subscriber_id, s.email, cs.claimed_at FROM campaign_sends cs LEFT JOIN subscribers s ON s.id = cs.subscriber_id
      WHERE cs.campaign_id = ? AND cs.status = 'uncertain' AND cs.subscriber_id > ? ORDER BY cs.subscriber_id LIMIT ?`,
  )
    .bind(params.id, url.searchParams.get("cursor") ?? "", limit + 1)
    .all<{ subscriber_id: string; email: string | null; claimed_at: string }>();
  const rows = results.slice(0, limit);
  const safe = honoursIdempotencyKey(env);
  const fresh = Date.now() - KEY_WINDOW_MS;
  return json({
    items: rows.map((r) => ({
      subscriberId: r.subscriber_id,
      email: r.email,
      claimedAt: r.claimed_at,
      safeResend: safe && Date.parse(r.claimed_at) > fresh,
    })),
    nextCursor: results.length > limit ? rows[rows.length - 1].subscriber_id : null,
    transport: transportOf(env),
    safeResend: safe,
  });
}

// readIds reads {ids?: string[]}: null = all, or a 400 Response.
async function readIds(request: Request): Promise<string[] | null | Response> {
  const b = (await readJson(request)) ?? {};
  if (b.ids === undefined) return null;
  const ok =
    Array.isArray(b.ids) && b.ids.length >= 1 && b.ids.length <= MAX_IDS && b.ids.every((i) => typeof i === "string" && i.length >= 1 && i.length <= 200);
  return ok ? (b.ids as string[]) : json({ error: "invalid_ids", detail: `ids must be 1 to ${MAX_IDS} subscriber ids` }, 400);
}

// move changes uncertain rows (all, or `ids`) and returns the subscriber ids moved.
async function move(env: Env, campaignId: string, ids: string[] | null, set: string, binds: unknown[]): Promise<string[]> {
  const { results } = await env.NEWSLETTER_DB.prepare(
    `UPDATE campaign_sends SET ${set}
      WHERE campaign_id = ?1 AND status = 'uncertain' AND (?2 IS NULL OR subscriber_id IN (SELECT value FROM json_each(?2)))
      RETURNING subscriber_id`,
  )
    .bind(campaignId, ids === null ? null : JSON.stringify(ids), ...binds)
    .all<{ subscriber_id: string }>();
  return results.map((r) => r.subscriber_id);
}

// POST …/uncertain/resend → {resent, ranges, safeResend}; 404 not_found;
// 409 not_resendable for a cancelled (or not yet started) campaign.
export async function resendUncertain({ env, params, request }: RouteContext): Promise<Response> {
  const ids = await readIds(request);
  if (ids instanceof Response) return ids;
  const c = await loadCampaign(env, params.id);
  if (!c) return json({ error: "not_found" }, 404);
  if (!RESENDABLE.has(c.status)) {
    return json({ error: "not_resendable", detail: `a ${c.status} campaign sends nothing more` }, 409);
  }
  const now = new Date().toISOString();
  const moved = await move(env, c.id, ids, "status = 'retry', attempts = MAX(0, ?3 - 1), error = NULL, claimed_at = ?4", [
    engineConfig(env).maxAttempts,
    now,
  ]);
  if (!moved.length) return json({ resent: 0, ranges: 0, safeResend: honoursIdempotencyKey(env) });
  // A paused campaign's reopened ranges are parked: resume queues them.
  const paused = c.status === "paused";
  const { results: ranges } = await env.NEWSLETTER_DB.prepare(
    `UPDATE send_batches SET status = ?1, retry_only = 1, updated_at = ?2
      WHERE campaign_id = ?3 AND status = 'done'
        AND EXISTS (SELECT 1 FROM json_each(?4) WHERE value > send_batches.after_id AND value <= send_batches.until_id)
      RETURNING after_id, until_id`,
  )
    .bind(paused ? "parked" : "queued", now, c.id, JSON.stringify(moved))
    .all<{ after_id: string; until_id: string }>();
  // A queue batch holds at most 100 messages. A job lost here is not lost
  // work: the cron re-queues a queued range after the stale timeout.
  for (let i = 0; !paused && i < ranges.length; i += 100) {
    await env.NEWSLETTER_QUEUE?.sendBatch(ranges.slice(i, i + 100).map((r) => ({ body: jobFor(c.id, r.after_id, r.until_id) })));
  }
  return json({ resent: moved.length, ranges: ranges.length, safeResend: honoursIdempotencyKey(env) });
}

// POST …/uncertain/dismiss → {dismissed}: the rows become `skipped` with
// error "dismissed" — the admin accepts that the message may be missing (or
// knows it arrived). 404 not_found.
export async function dismissUncertain({ env, params, request }: RouteContext): Promise<Response> {
  const ids = await readIds(request);
  if (ids instanceof Response) return ids;
  if (!(await loadCampaign(env, params.id))) return json({ error: "not_found" }, 404);
  const moved = await move(env, params.id, ids, "status = 'skipped', error = 'dismissed'", []);
  return json({ dismissed: moved.length });
}
