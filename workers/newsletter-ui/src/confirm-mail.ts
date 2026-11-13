// The double opt-in confirmation mail as newsletter-ui sends it to pending
// subscribers (an import can create them; nothing else would ever mail them).
// It is the `newsletter` template's mail exactly: the same subject and text
// per language (confirm-strings.ts), the same link to the template's confirm
// endpoint, the same sender defaults, and the same bookkeeping —
// confirm_sent_at is set only for a mail that actually left, and it starts the
// same 15-minute re-send throttle.

import { CONFIRM_STRINGS } from "./confirm-strings";
import type { Env } from "./lib";
import { json } from "./lib";
import type { MailMessage } from "./mail/_transport";
import { transportReady } from "./transport-ready";
import { siteOf } from "./render";
import type { Site } from "./render";

// CONFIRM_RESEND_MS mirrors the template's _store.ts: one confirmation mail
// per address per 15 minutes.
export const CONFIRM_RESEND_MS = 15 * 60 * 1000;

export interface PendingRow {
  id: string;
  email: string;
  language: string | null;
  token: string;
  consent_at: string | null;
}

// confirmLanguage maps a stored language ("pl", "pt-BR", null) to a shipped
// one by its primary subtag, else en — as the template's pickLang does with a
// subscriber's stored language.
export function confirmLanguage(language: string | null): string {
  const primary = (language ?? "").split("-")[0].toLowerCase();
  return Object.hasOwn(CONFIRM_STRINGS, primary) ? primary : "en";
}

// confirmUrl is the template's GET /api/newsletter/confirm on the site.
export const confirmUrl = (site: Site, token: string): string => `${site.url}/api/newsletter/confirm?token=${encodeURIComponent(token)}`;

export function confirmMessage(row: PendingRow, site: Site): MailMessage {
  const t = CONFIRM_STRINGS[confirmLanguage(row.language)];
  return { to: row.email, subject: t.mailSubject, text: t.mailBody.replace("{url}", confirmUrl(site, row.token)) };
}

// confirmReady checks what a confirmation mail needs, else the JSON answer.
export function confirmReady(env: Env): Site | Response {
  const site = siteOf(env);
  if (!site) {
    return json({ error: "no_site_url", detail: "set NEWSLETTER_SITE_URL to the site's https:// origin (the confirm link points there)" }, 503);
  }
  if (!env.NEWSLETTER_MAIL_FROM) return json({ error: "no_sender", detail: "set NEWSLETTER_MAIL_FROM" }, 503);
  return transportReady(env) ?? site;
}

// markConfirmSent records that a mail left; the status guard keeps a
// subscriber who confirmed meanwhile untouched.
export async function markConfirmSent(env: Env, id: string): Promise<void> {
  await env.NEWSLETTER_DB.prepare("UPDATE subscribers SET confirm_sent_at = ? WHERE id = ? AND status = 'pending'")
    .bind(new Date().toISOString(), id)
    .run();
}
