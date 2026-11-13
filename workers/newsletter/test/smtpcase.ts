// Shared fixtures for the SMTP tests: a config, a message, and a one-shot
// open → send → close against the in-memory server.
import { openSmtp } from "../functions/api/newsletter/_smtp.ts";
import type { SmtpConfig } from "../functions/api/newsletter/_smtp.ts";
import type { Outgoing } from "../functions/api/newsletter/_mime.ts";
import { smtpServer } from "./fakes.ts";
import type { ServerOptions } from "./fakes.ts";

export const cfg = (over: Partial<SmtpConfig> = {}): SmtpConfig => ({
  host: "smtp.example.com",
  port: 587,
  user: "mailer@example.com",
  password: "s3cret",
  helo: "example.com",
  timeoutMs: 200,
  ...over,
});

export const msg = (over: Partial<Outgoing> = {}): Outgoing => ({
  from: "news@example.com",
  fromName: "Example News",
  to: "reader@example.com",
  subject: "Hello",
  text: "Body",
  ...over,
});

// sendOne opens, sends one message and closes, returning everything.
export async function sendOne(server: ServerOptions = {}, config: Partial<SmtpConfig> = {}, message: Partial<Outgoing> = {}) {
  const s = smtpServer(server);
  const opened = await openSmtp(cfg(config), s.connect);
  if (!opened.ok) return { s, opened, sent: null };
  const sent = await opened.session.send(msg(message));
  await opened.session.close();
  return { s, opened, sent };
}
