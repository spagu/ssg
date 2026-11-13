// Applying classified bounces to the database (GO-127). Every source — the
// send engine, the two webhooks — ends here, so a bounce means the same thing
// whoever reported it:
//
//   hard, complaint  suppression (address kept) at once; the soft count goes;
//                    a complaint also unsubscribes the subscriber, with the
//                    same field effects as the `newsletter` unsubscribe link
//   soft             soft_bounces.count + 1; at NEWSLETTER_SOFT_BOUNCE_LIMIT
//                    the address moves to suppressions (reason `soft`)
//   delivered        the soft count goes (it counts bounces IN A ROW)
//
// One call = one db.batch = one transaction, including the hook_events rows
// that make a redelivered webhook event count once: either the event and its
// effects are written, or neither is.

import type { BounceKind } from "./bounce-classify";
import { normaliseEmail } from "./input";
import { intVar } from "./lib";
import type { Env } from "./lib";
import { SUPPRESSION_UPSERT, suppressionKey, suppressStatement } from "./suppress";

export const DEFAULT_SOFT_LIMIT = 3;
export const EVENT_RETENTION_MS = 7 * 86_400_000; // longer than any provider retries a webhook

export const softLimit = (env: Env): number => intVar(env.NEWSLETTER_SOFT_BOUNCE_LIMIT, DEFAULT_SOFT_LIMIT);

export interface Bounce {
  id: string | null; // a webhook event id to deduplicate on, or null
  kind: BounceKind;
  emails: string[];
  source: string; // smtp | resend | mailchannels
  detail: string;
}

export interface Applied {
  applied: number; // events written
  duplicate: number; // events seen before, skipped
}

type Stmt = D1PreparedStatement;

// unsubscribeByEmail: the `newsletter` template's unsubscribe, by address. An
// already unsubscribed row keeps its original unsubscribed_at.
export const unsubscribeByEmail = (env: Env, email: string, at: string): Stmt =>
  env.NEWSLETTER_DB.prepare(
    "UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = ?, updated_at = ? WHERE email = ? AND status != 'unsubscribed'",
  ).bind(at, at, email);

// softStatements counts one soft bounce and, at the limit, promotes the
// address to suppressions and drops the counter — all in SQL, so two bounces
// racing cannot both read "2" and miss the third.
async function softStatements(env: Env, b: Bounce, email: string, key: string, at: string): Promise<Stmt[]> {
  const db = env.NEWSLETTER_DB;
  const limit = softLimit(env);
  return [
    db
      .prepare(
        `INSERT INTO soft_bounces (email_hash, email, count, source, detail, updated_at) VALUES (?, ?, 1, ?, ?, ?)
         ON CONFLICT(email_hash) DO UPDATE SET count = soft_bounces.count + 1, source = excluded.source,
           detail = excluded.detail, updated_at = excluded.updated_at`,
      )
      .bind(key, email, b.source, b.detail.slice(0, 500), at),
    db
      .prepare(
        `INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at)
         SELECT email_hash, email, 'soft', count, ?, ?, ? FROM soft_bounces WHERE email_hash = ? AND count >= ?
         ${SUPPRESSION_UPSERT}`,
      )
      .bind(b.source, at, at, key, limit),
    db.prepare("DELETE FROM soft_bounces WHERE email_hash = ? AND count >= ?").bind(key, limit),
  ];
}

// statementsFor turns one bounce into its writes. Addresses that do not
// validate are ignored: they cannot be on the list.
export async function statementsFor(env: Env, b: Bounce, at: string): Promise<Stmt[]> {
  const db = env.NEWSLETTER_DB;
  const out: Stmt[] = [];
  const emails = b.emails.map(normaliseEmail).filter((e): e is string => e !== null);
  if (b.kind === "delivered") {
    if (!emails.length) return out;
    const keys = await Promise.all(emails.map(suppressionKey));
    return [db.prepare("DELETE FROM soft_bounces WHERE email_hash IN (SELECT value FROM json_each(?))").bind(JSON.stringify(keys))];
  }
  for (const email of emails) {
    const key = await suppressionKey(email);
    if (b.kind === "soft") {
      out.push(...(await softStatements(env, b, email, key, at)));
      continue;
    }
    out.push(await suppressStatement(env, { email, reason: b.kind, source: b.source, keepEmail: true }));
    out.push(db.prepare("DELETE FROM soft_bounces WHERE email_hash = ?").bind(key));
    if (b.kind === "complaint") out.push(unsubscribeByEmail(env, email, at));
  }
  return out;
}

// applyBounces writes the bounces of one report (a webhook request, one
// engine failure). Events with an id seen before are skipped; the new ids are
// recorded in the same transaction as their effects. If a concurrent delivery
// of the same event wins the race, the UNIQUE id makes this batch fail and roll
// back — the caller answers 500 and the provider's retry finds it applied.
export async function applyBounces(env: Env, provider: string, bounces: Bounce[]): Promise<Applied> {
  const db = env.NEWSLETTER_DB;
  const at = new Date().toISOString();
  const ids = bounces.filter((b) => b.id !== null).map((b) => `${provider}:${b.id}`);
  const seen = new Set<string>();
  if (ids.length) {
    const { results } = await db
      .prepare("SELECT id FROM hook_events WHERE id IN (SELECT value FROM json_each(?))")
      .bind(JSON.stringify(ids))
      .all<{ id: string }>();
    for (const r of results) seen.add(r.id);
  }
  const out: Applied = { applied: 0, duplicate: 0 };
  const writes: Stmt[] = [];
  for (const b of bounces) {
    if (b.id !== null) {
      const id = `${provider}:${b.id}`;
      if (seen.has(id)) {
        out.duplicate++;
        continue;
      }
      seen.add(id); // the same id twice in one request counts once too
      writes.push(db.prepare("INSERT INTO hook_events (id, received_at) VALUES (?, ?)").bind(id, at));
    }
    writes.push(...(await statementsFor(env, b, at)));
    out.applied++;
  }
  if (ids.length) {
    writes.push(db.prepare("DELETE FROM hook_events WHERE received_at < ?").bind(new Date(Date.now() - EVENT_RETENTION_MS).toISOString()));
  }
  if (writes.length) await db.batch(writes);
  return out;
}

// resetSmtpSoft is the engine's "a later message was accepted": one statement
// per batch for every address the batch sent to. Only counts the SMTP client
// recorded are reset here — a provider accepts every message first and reports
// the bounce later, so its "sent" proves nothing; its `delivered` event does.
export async function resetSmtpSoft(env: Env, emails: string[]): Promise<void> {
  if (!emails.length) return;
  await env.NEWSLETTER_DB.prepare("DELETE FROM soft_bounces WHERE source = 'smtp' AND email IN (SELECT value FROM json_each(?))")
    .bind(JSON.stringify(emails))
    .run();
}
