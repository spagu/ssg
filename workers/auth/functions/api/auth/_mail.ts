// Mail for the auth worker: invitations and password reset links. A leading
// underscore keeps this file out of the Pages route table.
//
// The sending itself is the newsletter template's portable mail code, carried
// byte for byte in mail/ (test/mail-copies.test.ts keeps it identical). Those
// modules read NEWSLETTER_* names; this worker has its own AUTH_* names (every
// worker of a Pages project shares one environment, so the newsletter's
// settings must never send account mail by accident), translated here.
//
// Mail is optional. Without a transport the API hands the one-time link to the
// administrator instead, and there is no self-service "forgot password". A
// transport that is configured but can never work (no sender, SMTP on port 25,
// a user without a password) is an error up front, never a silent no-op.

import { STRINGS, fill } from "./_i18n";
import type { Lang } from "./_i18n";
import type { Env } from "./_lib";
import { sendMail, transportOf, transportProblem } from "./mail/_transport";
import type { MailEnv, MailResult, SmtpConnect } from "./mail/_transport";

// mailEnv maps this worker's settings onto the names the portable code reads.
export function mailEnv(env: Env): MailEnv {
  return {
    NEWSLETTER_MAIL_FROM: env.AUTH_MAIL_FROM,
    NEWSLETTER_MAIL_FROM_NAME: env.AUTH_MAIL_FROM_NAME,
    NEWSLETTER_RESEND_API_KEY: env.AUTH_RESEND_API_KEY,
    NEWSLETTER_MAILCHANNELS_API_KEY: env.AUTH_MAILCHANNELS_API_KEY,
    NEWSLETTER_SMTP_HOST: env.AUTH_SMTP_HOST,
    NEWSLETTER_SMTP_PORT: env.AUTH_SMTP_PORT,
    NEWSLETTER_SMTP_USER: env.AUTH_SMTP_USER,
    NEWSLETTER_SMTP_PASSWORD: env.AUTH_SMTP_PASSWORD,
    NEWSLETTER_SMTP_HELO: env.AUTH_SMTP_HELO,
  };
}

export type MailState = { kind: "none" } | { kind: "ready"; transport: string } | { kind: "broken"; problem: string };

// mailState: no transport, a usable one, or one that can never send (why).
export function mailState(env: Env): MailState {
  const m = mailEnv(env);
  const transport = transportOf(m);
  if (!transport) return { kind: "none" };
  if (!env.AUTH_MAIL_FROM) return { kind: "broken", problem: "AUTH_MAIL_FROM is not set: a transport needs a verified sender address" };
  const problem = transportProblem(m);
  return problem ? { kind: "broken", problem } : { kind: "ready", transport };
}

// The tests hand the SMTP socket factory in here; nothing else should.
export const mailHooks: { connect?: SmtpConnect } = {};

export interface InviteMail {
  to: string;
  lang: Lang;
  site: string; // the host the link points at
  inviter: string; // the inviter's name, else address
  url: string;
  expires: string; // ISO 8601
  hours: number;
}

export interface ResetMail {
  to: string;
  lang: Lang;
  site: string;
  url: string;
  minutes: number;
}

// The texts say who, where, what the link does, how long it lasts, and what to
// do when the mail was not expected. Plain text only: nothing to render,
// nothing to track, nothing a filter mistakes for a campaign.
export function inviteMessage(m: InviteMail) {
  const t = STRINGS[m.lang];
  const vars = { site: m.site, inviter: m.inviter, url: m.url, expires: m.expires.slice(0, 16).replace("T", " "), hours: String(m.hours) };
  return { to: m.to, subject: fill(t.mailInviteSubject, vars), text: fill(t.mailInviteBody, vars) };
}

export function resetMessage(m: ResetMail) {
  const t = STRINGS[m.lang];
  const vars = { site: m.site, email: m.to, url: m.url, minutes: String(m.minutes) };
  return { to: m.to, subject: fill(t.mailResetSubject, vars), text: fill(t.mailResetBody, vars) };
}

// send delivers one message. The result's detail never holds the message (the
// link is in the body, not in any error the transports produce).
export function send(env: Env, message: { to: string; subject: string; text: string }): Promise<MailResult> {
  return sendMail(mailEnv(env), message, mailHooks.connect);
}
