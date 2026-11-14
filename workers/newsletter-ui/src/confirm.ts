// Confirmation mail for pending subscribers (GO-130 follow-up). An import
// without proof of consent creates `pending` rows; the `newsletter` template
// mails a confirmation only at sign-up, so these two routes send it:
//   POST /api/subscribers/confirmations     a bounded batch per call, to the
//        pending subscribers never mailed (confirm_sent_at IS NULL);
//   POST /api/subscribers/:id/confirmation  one re-send, 15-minute throttle.
// Both take from the send engine's per-minute budget (takeBudget/giveBack),
// so confirmations and campaigns share one rate. Suppressed addresses are
// skipped. confirm_sent_at is set only for a mail that left.

import { readJson } from "./campaign-model";
import { CONFIRM_RESEND_MS, confirmMessage, confirmReady, markConfirmSent } from "./confirm-mail";
import type { PendingRow } from "./confirm-mail";
import { defaultOpener } from "./engine-deliver";
import type { Opener } from "./engine-deliver";
import { engineConfig, giveBack, secondsToNextMinute, takeBudget } from "./engine-rate";
import type { Env } from "./lib";
import { intVar, json } from "./lib";
import { sendMail, transportOf } from "./mail/_transport";
import type { RouteContext } from "./router";
import { NOT_SUPPRESSED_SQL, dropHashOnly, findSuppression, suppressionApplies } from "./suppress";

// Per call: about 1 fetch + 1 D1 write per mail plus ~6 D1 queries, so 10
// stays well inside the Workers Free limit of 50 subrequests per invocation.
export const DEFAULT_CONFIRM_BATCH = 10;

// ELIGIBLE: pending subscribers never mailed a confirmation, after the id
// bound to `?` ("" for all), suppressed ones left out. The module summary
// counts them with it.
export const ELIGIBLE = `s.status = 'pending' AND s.confirm_sent_at IS NULL AND s.id > ? AND ${NOT_SUPPRESSED_SQL("s")}`;

const rateLimited = () => json({ error: "rate_limited", detail: "the send rate for this minute is used up" }, 429, { "retry-after": String(secondsToNextMinute()) });

async function remainingAfter(env: Env, after: string): Promise<number> {
  const row = await env.NEWSLETTER_DB.prepare(`SELECT COUNT(*) AS n FROM subscribers s WHERE ${ELIGIBLE}`).bind(after).first<{ n: number }>();
  return Number((row as { n: number }).n);
}

// POST /api/subscribers/confirmations {after?} →
//   {sent, failed, skipped, failures: [{id, detail, temporary}], remaining, next}
// Call again with {after: next} while next is not null; `next` moves past
// failed and skipped rows, so a bad address never blocks the queue.
export async function sendConfirmations({ request, env }: RouteContext): Promise<Response> {
  const b = (await readJson(request)) ?? {};
  if (b.after !== undefined && (typeof b.after !== "string" || b.after.length > 200)) {
    return json({ error: "invalid_input", detail: "after must be the next value of the previous answer" }, 400);
  }
  return confirmBatch(env, (b.after as string | undefined) ?? "");
}

// confirmBatch is the batch itself; `open` replaces the transport in tests,
// as in the send engine.
export async function confirmBatch(env: Env, after: string, open: Opener = defaultOpener): Promise<Response> {
  const site = confirmReady(env);
  if (site instanceof Response) return site;
  const cfg = engineConfig(env);
  const size = Math.min(intVar(env.NEWSLETTER_CONFIRM_BATCH, DEFAULT_CONFIRM_BATCH), cfg.rate);
  const { results: picked } = await env.NEWSLETTER_DB.prepare(
    `SELECT id, email, language, token, consent_at FROM subscribers s WHERE ${ELIGIBLE} ORDER BY id LIMIT ?`,
  )
    .bind(after, size)
    .all<PendingRow>();
  const out = { sent: 0, failed: 0, skipped: 0, failures: [] as { id: string; detail: string; temporary: boolean }[] };
  if (!picked.length) return json({ ...out, remaining: 0, next: null });

  const kept = await dropHashOnly(env, picked, true);
  const grant = await takeBudget(env, kept.length, cfg.rate);
  if (kept.length && !grant.granted) return rateLimited();
  const go = kept.slice(0, grant.granted);
  // The cursor ends at the last row handled: all picked rows when the budget
  // covered them, else the last one mailed (the rest come next call).
  const next = go.length < kept.length ? go[go.length - 1].id : picked[picked.length - 1].id;
  const keptIds = new Set(kept.map((r) => r.id));
  out.skipped = picked.filter((r) => r.id <= next && !keptIds.has(r.id)).length;

  let handed = 0;
  try {
    if (go.length) {
      const opened = await open(env);
      if (!opened.ok) return json({ error: "send_failed", detail: opened.detail, temporary: opened.temporary }, 502);
      try {
        for (const row of go) {
          handed++;
          const result = await opened.mailer.send(confirmMessage(row, site));
          if (result.ok) {
            await markConfirmSent(env, row.id);
            out.sent++;
          } else {
            out.failed++;
            out.failures.push({ id: row.id, detail: result.detail, temporary: result.temporary });
          }
        }
      } finally {
        await opened.mailer.close().catch((e) => console.warn("newsletter-ui: closing the mailer", e));
      }
    }
  } finally {
    await giveBack(env, grant, grant.granted - handed);
  }
  const remaining = await remainingAfter(env, next);
  return json({ ...out, remaining, next: remaining ? next : null });
}

// POST /api/subscribers/:id/confirmation → {ok: true, transport}
export async function resendConfirmation({ env, params }: RouteContext): Promise<Response> {
  const row = await env.NEWSLETTER_DB.prepare(
    "SELECT id, email, language, token, consent_at, status, confirm_sent_at FROM subscribers WHERE id = ?",
  )
    .bind(params.id)
    .first<PendingRow & { status: string; confirm_sent_at: string | null }>();
  if (!row) return json({ error: "not_found" }, 404);
  if (row.status !== "pending") return json({ error: "not_pending", detail: `a ${row.status} subscriber gets no confirmation mail` }, 409);
  const s = await findSuppression(env, row.email);
  if (s && suppressionApplies(s, row.consent_at)) return json({ error: "suppressed", detail: `the address is suppressed (${s.reason})` }, 409);
  const wait = (row.confirm_sent_at ? Date.parse(row.confirm_sent_at) || 0 : 0) + CONFIRM_RESEND_MS - Date.now();
  if (wait > 0) {
    return json({ error: "confirm_throttled", detail: "one confirmation mail per address per 15 minutes" }, 429, {
      "retry-after": String(Math.ceil(wait / 1000)),
    });
  }
  const site = confirmReady(env);
  if (site instanceof Response) return site;
  const grant = await takeBudget(env, 1, engineConfig(env).rate);
  if (!grant.granted) return rateLimited();
  const result = await sendMail(env, confirmMessage(row, site));
  if (!result.ok) return json({ error: "send_failed", detail: result.detail, temporary: result.temporary }, 502);
  await markConfirmSent(env, row.id);
  return json({ ok: true, transport: transportOf(env) });
}
