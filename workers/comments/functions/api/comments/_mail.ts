// Email-on-new-comment (#1.8.16). When a non-spam comment lands, POST a
// moderation notice to a configurable HTTP email API — the JSON shape
// {from, to, subject, text} works with providers like Resend or your own relay.
// A no-op unless MAIL_URL + MAIL_FROM + MAIL_TO are configured, so the feature is
// opt-in and delivery never blocks the visitor's response (call via waitUntil).

import type { Env } from "./_lib";

interface CommentNotice {
  url: string;
  author: string;
  body: string;
}

// notifyByEmail sends the moderation notice; failures are logged, never thrown,
// so a mail-gateway problem can't break comment submission.
export async function notifyByEmail(env: Env, c: CommentNotice): Promise<void> {
  const url = env.COMMENTS_MAIL_URL;
  const from = env.COMMENTS_MAIL_FROM;
  const to = (env.COMMENTS_MAIL_TO || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (!url || !from || to.length === 0) return; // not configured

  const subject = `${env.COMMENTS_MAIL_SUBJECT || "New comment awaiting review"} — ${c.url}`;
  const text =
    `A new comment is awaiting moderation.\n\n` +
    `Page:   ${c.url}\n` +
    `Author: ${c.author}\n\n` +
    `${c.body}\n`;

  const headers: Record<string, string> = { "content-type": "application/json" };
  if (env.COMMENTS_MAIL_KEY) headers.authorization = `Bearer ${env.COMMENTS_MAIL_KEY}`;

  try {
    const res = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify({ from, to, subject, text }),
    });
    if (!res.ok) console.warn(`comment mail: gateway returned ${res.status}`);
  } catch (e) {
    console.warn(`comment mail: ${e}`);
  }
}
