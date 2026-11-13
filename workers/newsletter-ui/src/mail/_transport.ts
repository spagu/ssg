// One way to send mail, three transports behind it. A leading underscore keeps
// this file out of the Pages route table — it is imported, never served.
// Portable: see _mime.ts.
//
// The first transport configured wins: Resend → MailChannels → SMTP. The order
// is deliberate — an install that already has a provider key keeps sending
// through it after SMTP settings appear next to it.

import { checkMessage, permanent, temporary } from "./_mime";
import type { MailFailure, MailMessage, MailResult, Outgoing } from "./_mime";
import { checkConfig, openSmtp } from "./_smtp";
import type { SmtpConfig, SmtpConnect } from "./_smtp";

export type { MailFailure, MailMessage, MailResult, SmtpConnect };

// MailEnv is only the mail settings; a worker's own Env carries them as well.
export interface MailEnv {
  NEWSLETTER_MAIL_FROM?: string; // verified sender, e.g. "news@example.com"
  NEWSLETTER_MAIL_FROM_NAME?: string; // optional display name
  NEWSLETTER_RESEND_API_KEY?: string; // Resend
  NEWSLETTER_MAILCHANNELS_API_KEY?: string; // MailChannels Email API
  NEWSLETTER_SMTP_HOST?: string; // your own mail server
  NEWSLETTER_SMTP_PORT?: string; // 465 = TLS from the start, 587 (default) = STARTTLS
  NEWSLETTER_SMTP_USER?: string;
  NEWSLETTER_SMTP_PASSWORD?: string; // a secret
  NEWSLETTER_SMTP_HELO?: string; // EHLO name; defaults to the sender's domain
}

export type Transport = "resend" | "mailchannels" | "smtp";

export function transportOf(env: MailEnv): Transport | null {
  if (env.NEWSLETTER_RESEND_API_KEY) return "resend";
  if (env.NEWSLETTER_MAILCHANNELS_API_KEY) return "mailchannels";
  if (env.NEWSLETTER_SMTP_HOST) return "smtp";
  return null;
}

// smtpConfig reads the SMTP settings; an unset port is 587 (STARTTLS).
function smtpConfig(env: MailEnv): SmtpConfig {
  const sender = env.NEWSLETTER_MAIL_FROM ?? "";
  return {
    host: env.NEWSLETTER_SMTP_HOST as string,
    port: env.NEWSLETTER_SMTP_PORT ? Number(env.NEWSLETTER_SMTP_PORT) : 587,
    user: env.NEWSLETTER_SMTP_USER,
    password: env.NEWSLETTER_SMTP_PASSWORD,
    helo: env.NEWSLETTER_SMTP_HELO || sender.slice(sender.lastIndexOf("@") + 1) || "localhost",
  };
}

// transportProblem says why the configured transport can never send (SMTP on
// port 25, a port that is not a number, a user without a password, a bad EHLO
// name), without connecting; null when it may work or none is configured. The
// HTTP providers have nothing to check before a request: a wrong key shows only
// in the provider's answer.
export function transportProblem(env: MailEnv): string | null {
  return transportOf(env) === "smtp" ? checkConfig(smtpConfig(env)) : null;
}

// A Mailer sends one message after another. For SMTP that is one connection
// for the whole batch; for the HTTP providers each send is its own request.
export interface Mailer {
  send(m: MailMessage): Promise<MailResult>;
  close(): Promise<void>;
}

function resolve(env: MailEnv, m: MailMessage): Outgoing | MailFailure {
  const from = m.from ?? env.NEWSLETTER_MAIL_FROM;
  if (!from) return permanent("no sender: NEWSLETTER_MAIL_FROM is not set");
  const out: Outgoing = { ...m, from, fromName: m.fromName ?? env.NEWSLETTER_MAIL_FROM_NAME ?? "" };
  const problem = checkMessage(out);
  return problem ? permanent(problem) : out;
}

const hasHeaders = (m: Outgoing): boolean => Object.keys(m.headers ?? {}).length > 0;

// honoursIdempotencyKey says whether the configured transport itself refuses
// to send a message twice for the same idempotencyKey. Only Resend documents
// that (the Idempotency-Key header, keys kept 24 hours); the MailChannels Email
// API documents no such mechanism, and SMTP has none — there the key only
// becomes a deterministic Message-ID, a hint some receivers de-duplicate on.
export const honoursIdempotencyKey = (env: MailEnv): boolean => transportOf(env) === "resend";

// The request bodies keep the exact shape the confirmation mail always had;
// html and headers are added only when a message carries them, and the
// Idempotency-Key header only when it has a key.
function resendRequest(key: string, m: Outgoing): Promise<Response> {
  const idempotency: Record<string, string> = m.idempotencyKey === undefined ? {} : { "Idempotency-Key": m.idempotencyKey };
  return fetch("https://api.resend.com/emails", {
    method: "POST",
    headers: { authorization: `Bearer ${key}`, "content-type": "application/json", ...idempotency },
    body: JSON.stringify({
      from: m.fromName ? `${m.fromName} <${m.from}>` : m.from,
      to: m.to,
      subject: m.subject,
      text: m.text,
      ...(m.html ? { html: m.html } : {}),
      ...(hasHeaders(m) ? { headers: m.headers } : {}),
    }),
  });
}

function mailchannelsRequest(key: string, m: Outgoing): Promise<Response> {
  const content = [{ type: "text/plain", value: m.text }];
  if (m.html) content.push({ type: "text/html", value: m.html });
  return fetch("https://api.mailchannels.net/tx/v1/send", {
    method: "POST",
    headers: { "x-api-key": key, "content-type": "application/json" },
    body: JSON.stringify({
      personalizations: [{ to: [{ email: m.to }] }],
      from: m.fromName ? { email: m.from, name: m.fromName } : { email: m.from },
      subject: m.subject,
      content,
      ...(hasHeaders(m) ? { headers: m.headers } : {}),
    }),
  });
}

// resendConflict reads a 409 to a request that carried an Idempotency-Key.
// Resend's error reference documents two such answers, told apart by the
// error `name` in the JSON body:
//   - `concurrent_idempotent_requests`: a request with this key is still in
//     progress — temporary, try again later;
//   - `invalid_idempotent_request`: the key was used within the last 24 hours
//     with a different payload — a message under this key was already
//     accepted, so for the caller it is sent, not failed.
// Any other 409 (an unreadable body, an unknown name) is temporary: a retry
// costs one request, a permanent failure would drop the recipient for good.
async function resendConflict(res: Response): Promise<MailResult> {
  let name: unknown;
  try {
    name = ((await res.json()) as { name?: unknown } | null)?.name;
  } catch {
    name = undefined;
  }
  if (name === "invalid_idempotent_request") return { ok: true, duplicate: true };
  const shown = typeof name === "string" && /^[a-z_]{1,64}$/.test(name) ? ` ${name}` : "";
  return temporary(`resend returned 409${shown}`);
}

// An HTTP provider's 429 and 5xx pass with time; any other refusal will not,
// except a 409 that `conflict` (Resend with a key) reads first.
async function httpSend(name: string, request: () => Promise<Response>, conflict?: (res: Response) => Promise<MailResult>): Promise<MailResult> {
  let res: Response;
  try {
    res = await request();
  } catch (e) {
    return temporary(`${name}: ${e}`);
  }
  if (res.ok) return { ok: true };
  if (res.status === 409 && conflict) return conflict(res);
  return { ok: false, temporary: res.status === 429 || res.status >= 500, detail: `${name} returned ${res.status}` };
}

// loadConnect imports the sockets API on first use, so a runtime without it
// (and Node in the tests) can still load this module; it warns once.
let socketsWarned = false;
async function loadConnect(): Promise<SmtpConnect | null> {
  try {
    return ((await import("cloudflare:sockets")) as { connect: SmtpConnect }).connect;
  } catch (e) {
    if (!socketsWarned) {
      socketsWarned = true;
      console.warn(`mail: cloudflare:sockets is not available here, SMTP cannot send (${e})`);
    }
    return null;
  }
}

// openMailer prepares the configured transport; for SMTP that means
// connecting and logging in. `connect` replaces the sockets API in tests.
export async function openMailer(env: MailEnv, connect?: SmtpConnect): Promise<{ ok: true; mailer: Mailer } | MailFailure> {
  const kind = transportOf(env);
  if (kind === "resend" || kind === "mailchannels") {
    const key = (kind === "resend" ? env.NEWSLETTER_RESEND_API_KEY : env.NEWSLETTER_MAILCHANNELS_API_KEY) as string;
    const request = kind === "resend" ? resendRequest : mailchannelsRequest;
    const send = async (m: MailMessage): Promise<MailResult> => {
      const out = resolve(env, m);
      if ("ok" in out) return out;
      // A 409 means an idempotency conflict only when a key was sent; without
      // one it stays the permanent refusal it always was.
      const conflict = kind === "resend" && out.idempotencyKey !== undefined ? resendConflict : undefined;
      return httpSend(kind, () => request(key, out), conflict);
    };
    return { ok: true, mailer: { send, close: async () => {} } };
  }
  if (kind === null) return permanent("no mail transport configured");

  const sockets = connect ?? (await loadConnect());
  if (!sockets) return permanent("smtp: cloudflare:sockets is not available in this runtime");
  const opened = await openSmtp(smtpConfig(env), sockets);
  if (!opened.ok) return opened;
  const send = async (m: MailMessage): Promise<MailResult> => {
    const out = resolve(env, m);
    return "ok" in out ? out : opened.session.send(out);
  };
  return { ok: true, mailer: { send, close: () => opened.session.close() } };
}

// sendMail sends one message through whichever transport is configured. It
// never throws: every outcome is a MailResult. A message that cannot be sent
// as written is refused before any connection is made.
export async function sendMail(env: MailEnv, m: MailMessage, connect?: SmtpConnect): Promise<MailResult> {
  const out = resolve(env, m);
  if ("ok" in out) return out;
  const opened = await openMailer(env, connect);
  if (!opened.ok) return opened;
  try {
    return await opened.mailer.send(m);
  } finally {
    await opened.mailer.close();
  }
}
