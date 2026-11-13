// The sending loop of one batch: one mailer session (one SMTP connection, or
// the HTTP provider's client), then for each recipient: claim the row and run
// the last check (one statement), render, send, record the outcome.
//
// Claiming one row right before its send — not the whole batch up front — is
// what keeps a crash to one ambiguous message: whatever kills the consumer,
// only the row being handed to the transport can be left `claimed` (and later
// `uncertain`, engine-claim.ts); the rows after it have no claim to clean up.

import type { Campaign } from "./campaign-model";
import { pauseFor, stopBatch } from "./engine-batches";
import type { SendJob } from "./engine-batches";
import { breakerFor, breakerReason, notePermanent } from "./engine-breaker";
import type { Breaker } from "./engine-breaker";
import { claimOne, record, release } from "./engine-claim";
import { engineHooks } from "./engine-hooks";
import type { PermanentFailure } from "./engine-hooks";
import type { EngineConfig } from "./engine-rate";
import type { Candidate } from "./engine-recipients";
import type { Env } from "./lib";
import { openMailer } from "./mail/_transport";
import type { Mailer, MailFailure } from "./mail/_transport";
import { rendererFor } from "./click-track";
import type { Site } from "./render";

export type Opener = (env: Env) => Promise<{ ok: true; mailer: Mailer } | MailFailure>;

export const defaultOpener: Opener = (env) => openMailer(env);

export interface DeliverInput {
  env: Env;
  job: SendJob;
  campaign: Campaign;
  live: string; // the campaign status this batch runs under: `sending`, or `sent` for re-sends
  rows: Candidate[]; // already within the rate grant
  cfg: EngineConfig;
  site: Site;
  sender: { from: string; fromName: string };
  open: Opener;
  // Messages handed to the transport so far: the rate budget actually used.
  // A counter the caller owns, so it is right even when deliver throws.
  tally: { handed: number };
}

// temporary: recipients left in `retry`; stopped: the batch must end now.
export interface Delivered {
  temporary: number;
  stopped: boolean;
}

// idempotencyKey is the same for every attempt at one campaign's message to
// one person, so a provider that honours keys never sends it twice.
export const idempotencyKey = (campaignId: string, subscriberId: string): string => `${campaignId}:${subscriberId}`;

// The note on a `sent` row the provider refused as a repeat of its key.
export const DUPLICATE_DETAIL = "already accepted earlier under this idempotency key";

export async function deliver(d: DeliverInput): Promise<Delivered> {
  const { env, job, campaign: c, cfg } = d;
  const out: Delivered = { temporary: 0, stopped: false };
  // Click links (GO-129, when on) are registered before the mailer opens: a
  // database error here retries the job with nothing claimed or sent.
  const render = await rendererFor(env, c, d.site, d.rows, true);
  const opened = await d.open(env);
  if (!opened.ok) {
    if (opened.temporary) throw new Error(`mail transport unavailable: ${opened.detail}`);
    await pauseFor(env, job, `transport: ${opened.detail}`);
    return { ...out, stopped: true };
  }
  const accepted: string[] = []; // for one soft-bounce reset per batch, not one per message
  const breaker = breakerFor(env);
  let tripped = false;
  // The claim this consumer holds, and whether its message reached the transport.
  let current: { id: string; stamp: string; handed: boolean } | null = null;
  try {
    for (const r of d.rows) {
      const stamp = new Date().toISOString();
      current = { id: r.id, stamp, handed: false };
      const got = await claimOne(env, c.id, r, stamp, cfg.maxAttempts, d.live);
      if (!got) continue; // final, or another consumer has it
      const v = got.verdict;
      if ("stop" in v) {
        await release(env, c.id, [r.id], stamp);
        await stopBatch(env, job, v.stop);
        return { ...out, stopped: true };
      }
      if ("skip" in v) {
        await record(env, c.id, r.id, "skipped", v.skip);
        continue;
      }
      let m;
      try {
        m = render({ email: r.email, language: r.language, token: r.token });
      } catch (e) {
        await release(env, c.id, [r.id], stamp);
        await pauseFor(env, job, `render: ${e instanceof Error ? e.message : String(e)}`);
        return { ...out, stopped: true };
      }
      d.tally.handed++;
      breaker.attempted++;
      current.handed = true;
      const message = { to: r.email, subject: m.subject, text: m.text, html: m.html, headers: m.headers, ...d.sender };
      const result = await opened.mailer.send({ ...message, idempotencyKey: idempotencyKey(c.id, r.id) });
      if (result.ok) {
        // duplicate: the provider already accepted a message under this key
        // (an earlier attempt whose answer was lost) — sent, with a note why.
        await record(env, c.id, r.id, "sent", result.duplicate ? DUPLICATE_DETAIL : null);
        accepted.push(r.email);
      } else if (!result.temporary) {
        await record(env, c.id, r.id, "failed", result.detail);
        if (notePermanent(breaker, { email: r.email, subscriberId: r.id, campaignId: c.id, detail: result.detail })) {
          tripped = true;
          await release(env, c.id, breaker.hard, null);
          await pauseFor(env, job, breakerReason(breaker));
          return { ...out, stopped: true };
        }
      } else if (got.attempts >= cfg.maxAttempts) {
        await record(env, c.id, r.id, "failed", `${result.detail} (gave up after ${cfg.maxAttempts} attempts)`);
      } else {
        await record(env, c.id, r.id, "retry", result.detail);
        out.temporary++;
      }
      current = null;
    }
  } catch (e) {
    // The transport or the database threw. A claim whose message never
    // reached the transport is handed back now; one that did is left
    // `claimed` — whether it went out is unknown, so the stale timeout makes
    // it `uncertain` rather than sending it again.
    if (current && !current.handed) await release(env, c.id, [current.id], current.stamp).catch(() => {});
    throw e;
  } finally {
    if (!tripped) await permanentHooks(env, breaker);
    await deliveredHook(env, accepted);
    await opened.mailer.close().catch((e) => console.warn("newsletter-ui: closing the mailer", e));
  }
  return out;
}

// permanentHooks runs onPermanentFailure for the batch's refusals, at its end
// (engine-breaker.ts says why); a failing hook is logged, the rest still run.
async function permanentHooks(env: Env, b: Breaker): Promise<void> {
  for (const f of b.failures) await permanentHook(env, f);
}

async function permanentHook(env: Env, failure: PermanentFailure): Promise<void> {
  try {
    await engineHooks.onPermanentFailure(env, failure);
  } catch (e) {
    console.error("newsletter-ui: onPermanentFailure hook failed", e);
  }
}

async function deliveredHook(env: Env, emails: string[]): Promise<void> {
  try {
    await engineHooks.onDelivered(env, emails);
  } catch (e) {
    console.error("newsletter-ui: onDelivered hook failed", e);
  }
}
