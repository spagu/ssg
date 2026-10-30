// Cloudflare Pages Function: contact / job-application form handler.
// POST /api/contact — validates a Turnstile token, then sends the message via
// Resend or the MailChannels Email API (see _mail.ts).
//
// Secrets (wrangler pages secret put <NAME>):
//   CONTACT_TURNSTILE_SECRET      Cloudflare Turnstile secret key for this form
//   TURNSTILE_SECRET              shared fallback, read only when the prefixed
//                                 one is unset (kept so existing projects keep working)
//   CONTACT_TO                    destination inbox, e.g. "team@example.com"
//   CONTACT_FROM                  verified sender, e.g. "noreply@example.com"
//   CONTACT_RESEND_API_KEY        Resend key — or:
//   CONTACT_MAILCHANNELS_API_KEY  MailChannels key (one of the two is required)
//   RESEND_API_KEY                legacy Resend fallback, read only when neither
//                                 prefixed key is set
// Without a mail key the form answers 503 instead of failing at the provider.

import { type MailEnv, missingMailConfig, sendMail } from "./_mail";

interface Env extends MailEnv {
  CONTACT_TURNSTILE_SECRET?: string;
  TURNSTILE_SECRET?: string;
}

const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });

async function verifyTurnstile(secret: string, token: string, ip: string | null): Promise<boolean> {
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  if (ip) body.append("remoteip", ip);
  const res = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", { method: "POST", body });
  const out = (await res.json()) as { success: boolean };
  return out.success === true;
}

// One Pages project runs every worker it carries with one shared environment,
// so an unprefixed secret set for one template is visible to all of them
// (#325). The prefixed name is this form's own; the shared one is a fallback
// for projects configured before the prefix existed.
const turnstileSecret = (env: Env): string | undefined =>
  env.CONTACT_TURNSTILE_SECRET || env.TURNSTILE_SECRET;

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let form: Record<string, string>;
  try {
    const data = await request.formData();
    form = Object.fromEntries([...data.entries()].map(([k, v]) => [k, String(v)]));
  } catch {
    return json({ error: "invalid form data" }, 400);
  }

  const { name, email, message } = form;
  if (!name || !email || !message) return json({ error: "name, email and message are required" }, 422);

  const secret = turnstileSecret(env);
  if (!secret) return json({ error: "contact form not configured" }, 503);
  // Checked before the captcha so a misconfigured form never spends the token.
  const missing = missingMailConfig(env);
  if (missing) {
    console.error(`contact form: mail not configured: ${missing}`);
    return json({ error: `contact form mail not configured: ${missing}` }, 503);
  }
  const token = form["cf-turnstile-response"];
  const ip = request.headers.get("cf-connecting-ip");
  if (!token || !(await verifyTurnstile(secret, token, ip))) {
    return json({ error: "captcha verification failed" }, 403);
  }

  const sent = await sendMail(env, {
    replyTo: { email, name },
    subject: `Contact form: ${name}`,
    text: `From: ${name} <${email}>\n\n${message}`,
  });
  return sent ? json({ ok: true }) : json({ error: "send failed" }, 502);
};
