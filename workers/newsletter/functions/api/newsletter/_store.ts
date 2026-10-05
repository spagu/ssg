// D1 access for the newsletter worker. A leading underscore keeps this file out
// of the Pages route table — it is imported, never served.
//
// Status moves one way per event:
//   (new) ──sign-up──▶ pending ──confirm link──▶ confirmed     (double opt-in)
//   (new) ──sign-up──▶ confirmed                               (single opt-in)
//   pending | confirmed ──unsubscribe──▶ unsubscribed ──sign-up──▶ as (new)

import { Env, newToken, retentionDays } from "./_lib";

export interface Signup {
  email: string;
  language: string | null;
  sourcePage: string | null;
  tags: string;
  consentText: string;
  ipHash: string | null;
  uaHash: string | null;
}

export interface Subscriber {
  email: string;
  status: string;
  language: string | null;
  token: string;
  confirm_sent_at: string | null;
}

// A confirmation mail is not re-sent to the same pending address within this
// window: the sign-up endpoint is public, and without it anyone could use it to
// flood a stranger's inbox one Turnstile solve at a time.
const CONFIRM_RESEND_MS = 15 * 60 * 1000;

const now = (): string => new Date().toISOString();

// subscribe is the idempotent upsert behind POST /api/newsletter. It returns the
// row's token and whether a confirmation mail should go out. Callers answer
// every outcome identically, so the response never tells a visitor whether an
// address was already on the list.
export async function subscribe(env: Env, s: Signup, doubleOptIn: boolean): Promise<{ sendConfirmation: boolean; subscriber: Subscriber }> {
  const db = env.NEWSLETTER_DB;
  const at = now();
  const status = doubleOptIn ? "pending" : "confirmed";
  const existing = await findBy(env, "email", s.email);

  if (!existing) {
    const token = newToken();
    // ON CONFLICT DO NOTHING: two simultaneous first sign-ups for one address
    // both pass the SELECT above; the second insert is simply dropped.
    await db.prepare(
      `INSERT INTO subscribers
         (id, email, status, language, source_page, tags, consent_text, consent_at, token,
          created_at, updated_at, confirmed_at, ip_hash, ua_hash)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(email) DO NOTHING`,
    ).bind(
      crypto.randomUUID(), s.email, status, s.language, s.sourcePage, s.tags, s.consentText, at, token,
      at, at, doubleOptIn ? null : at, s.ipHash, s.uaHash,
    ).run();
    return { sendConfirmation: doubleOptIn, subscriber: { email: s.email, status, language: s.language, token, confirm_sent_at: null } };
  }

  // A confirmed subscriber is left exactly as they are. The request is
  // unauthenticated: letting it rewrite someone's language, tags or consent
  // record would let anyone who knows an address edit that person's entry.
  if (existing.status === "confirmed") return { sendConfirmation: false, subscriber: existing };

  // Pending or unsubscribed: this is a fresh sign-up with a fresh consent.
  // Re-subscribing clears unsubscribed_at; the token is kept, so unsubscribe
  // links in mail already sent keep working.
  await db.prepare(
    `UPDATE subscribers
        SET status = ?, language = ?, source_page = ?, tags = ?, consent_text = ?, consent_at = ?,
            updated_at = ?, confirmed_at = ?, unsubscribed_at = NULL, ip_hash = ?, ua_hash = ?
      WHERE email = ?`,
  ).bind(
    status, s.language, s.sourcePage, s.tags, s.consentText, at,
    at, doubleOptIn ? null : at, s.ipHash, s.uaHash, s.email,
  ).run();

  const sent = existing.confirm_sent_at ? Date.parse(existing.confirm_sent_at) : 0;
  const recentlySent = existing.status === "pending" && Date.now() - sent < CONFIRM_RESEND_MS;
  return {
    sendConfirmation: doubleOptIn && !recentlySent,
    subscriber: { ...existing, status, language: s.language },
  };
}

// findBy looks a subscriber up by email or by link token.
export async function findBy(env: Env, column: "email" | "token", value: string): Promise<Subscriber | null> {
  return env.NEWSLETTER_DB.prepare(
    `SELECT email, status, language, token, confirm_sent_at FROM subscribers WHERE ${column} = ?`,
  ).bind(value).first<Subscriber>();
}

// confirm moves a pending row to confirmed. Returns false when nothing changed
// (unknown token, already confirmed, or unsubscribed — an old confirmation link
// must not undo an unsubscribe).
export async function confirm(env: Env, token: string): Promise<boolean> {
  const at = now();
  const res = await env.NEWSLETTER_DB.prepare(
    `UPDATE subscribers SET status = 'confirmed', confirmed_at = ?, updated_at = ?
      WHERE token = ? AND status = 'pending'`,
  ).bind(at, at, token).run();
  return (res.meta.changes ?? 0) > 0;
}

// unsubscribe marks a row unsubscribed. Idempotent: repeating it changes
// nothing, and the original unsubscribed_at is kept.
export async function unsubscribe(env: Env, token: string): Promise<void> {
  const at = now();
  await env.NEWSLETTER_DB.prepare(
    `UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = ?, updated_at = ?
      WHERE token = ? AND status != 'unsubscribed'`,
  ).bind(at, at, token).run();
}

// markConfirmSent records when a confirmation mail went out (see CONFIRM_RESEND_MS).
export async function markConfirmSent(env: Env, email: string): Promise<void> {
  await env.NEWSLETTER_DB.prepare("UPDATE subscribers SET confirm_sent_at = ? WHERE email = ?").bind(now(), email).run();
}

// Per-isolate timestamp of the last purge: the lazy purge piggybacks on writes
// and runs at most hourly per isolate, so a busy sign-up form does not turn
// into a DELETE per request.
let lastPurge = 0;
const PURGE_EVERY_MS = 60 * 60 * 1000;

// purgeExpired deletes pending and unsubscribed rows untouched for
// NEWSLETTER_RETENTION_DAYS. Confirmed subscribers are never purged. `force`
// skips the hourly throttle (the admin endpoint). Returns the rows deleted.
export async function purgeExpired(env: Env, force = false): Promise<number> {
  const days = retentionDays(env);
  if (!days || (!force && Date.now() - lastPurge < PURGE_EVERY_MS)) return 0;
  lastPurge = Date.now();
  const cutoff = new Date(Date.now() - days * 86400000).toISOString();
  const res = await env.NEWSLETTER_DB.prepare(
    `DELETE FROM subscribers WHERE status IN ('pending', 'unsubscribed') AND updated_at < ?`,
  ).bind(cutoff).run();
  return res.meta.changes ?? 0;
}
