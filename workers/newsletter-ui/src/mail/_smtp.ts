// A small SMTP submission client over Cloudflare TCP sockets. A leading
// underscore keeps this file out of the Pages route table — it is imported,
// never served. Portable: see _mime.ts.
//
// One session = one connection: open() greets, upgrades to TLS and logs in;
// send() can then be called for several messages (a batch) before close().
// TLS is not optional — port 465 starts encrypted, every other port must offer
// STARTTLS — so a password and a subscriber's address never cross the network
// in clear text.

import { buildMime, checkMessage, isAddress, permanent } from "./_mime";
import type { MailFailure, MailResult, Outgoing } from "./_mime";
import { SmtpFailure, Wire } from "./_smtpwire";
import type { SmtpSocket } from "./_smtpwire";

export type { SmtpSocket };

// The shape of `connect` from "cloudflare:sockets", injectable for tests.
export type SmtpConnect = (
  address: { hostname: string; port: number },
  options: { secureTransport: "on" | "starttls"; allowHalfOpen: boolean },
) => SmtpSocket;

export interface SmtpConfig {
  host: string;
  port: number;
  user?: string;
  password?: string;
  helo: string; // the name this client gives in EHLO
  timeoutMs?: number; // per command; the default fits inside a request's budget
}

export interface SmtpSession {
  send(m: Outgoing): Promise<MailResult>;
  close(): Promise<void>;
}

const DEFAULT_TIMEOUT_MS = 10_000;

// checkConfig catches what no server could fix, before a connection is made.
// Exported for transportProblem (_transport.ts), which asks the same question
// without connecting.
export function checkConfig(cfg: SmtpConfig): string | null {
  if (cfg.port === 25) return "smtp: port 25 is blocked by Cloudflare for outbound connections; use 465 (TLS) or 587 (STARTTLS)";
  if (!Number.isInteger(cfg.port) || cfg.port < 1 || cfg.port > 65535) return `smtp: invalid port ${cfg.port}`;
  if (!/^[!-~]+$/.test(cfg.helo)) return "smtp: invalid EHLO name";
  if (cfg.user && !cfg.password) return "smtp: a user is set but the password is not";
  return null;
}

const failure = (e: unknown): MailFailure =>
  e instanceof SmtpFailure ? { ok: false, temporary: e.temporary, detail: e.message } : { ok: false, temporary: true, detail: `smtp: ${e}` };

const b64 = (s: string): string => {
  let bin = "";
  for (const b of new TextEncoder().encode(s)) bin += String.fromCharCode(b);
  return btoa(bin);
};

// ehlo returns the advertised extensions, keyword → parameters, upper-cased.
// "AUTH=PLAIN LOGIN" (an old spelling some servers still send) reads as AUTH.
async function ehlo(wire: Wire, helo: string): Promise<Map<string, string[]>> {
  const reply = await wire.expect("EHLO", `EHLO ${helo}`, [250]);
  const caps = new Map<string, string[]>();
  for (const line of reply.lines.slice(1)) {
    const [keyword, ...params] = line.toUpperCase().split(/[ =]/);
    caps.set(keyword, [...(caps.get(keyword) ?? []), ...params]);
  }
  return caps;
}

// auth logs in with PLAIN when offered, else LOGIN. Called only once the
// connection is encrypted.
async function auth(wire: Wire, caps: Map<string, string[]>, user: string, password: string): Promise<void> {
  const mechanisms = caps.get("AUTH") ?? [];
  if (mechanisms.includes("PLAIN")) {
    await wire.expect("AUTH", `AUTH PLAIN ${b64(`\0${user}\0${password}`)}`, [235]);
  } else if (mechanisms.includes("LOGIN")) {
    await wire.expect("AUTH", "AUTH LOGIN", [334]);
    await wire.expect("AUTH", b64(user), [334]);
    await wire.expect("AUTH", b64(password), [235]);
  } else {
    throw new SmtpFailure(false, "smtp: the server offers neither AUTH PLAIN nor AUTH LOGIN");
  }
}

// openSmtp connects, upgrades and authenticates. It never throws.
export async function openSmtp(
  cfg: SmtpConfig,
  connect: SmtpConnect,
): Promise<{ ok: true; session: SmtpSession } | MailFailure> {
  const problem = checkConfig(cfg);
  if (problem) return permanent(problem);
  const implicitTls = cfg.port === 465;
  let wire: Wire | undefined;
  try {
    const socket = connect({ hostname: cfg.host, port: cfg.port }, { secureTransport: implicitTls ? "on" : "starttls", allowHalfOpen: false });
    wire = new Wire(socket, cfg.timeoutMs ?? DEFAULT_TIMEOUT_MS);
    await wire.expect("greeting", null, [220]);
    let caps = await ehlo(wire, cfg.helo);
    if (!implicitTls) {
      if (!caps.has("STARTTLS")) throw new SmtpFailure(false, "smtp: the server does not offer STARTTLS; refusing to continue in clear text");
      await wire.expect("STARTTLS", "STARTTLS", [220]);
      wire.upgrade();
      // What the server said before TLS cannot be trusted: ask again.
      caps = await ehlo(wire, cfg.helo);
    }
    if (cfg.user) await auth(wire, caps, cfg.user, cfg.password as string);
    return { ok: true, session: new Session(wire) };
  } catch (e) {
    await wire?.close();
    return failure(e);
  }
}

// dotStuff doubles a leading "." on every line (RFC 5321 4.5.2), so no line of
// the message can end the DATA phase early.
export const dotStuff = (data: string): string => data.replace(/(^|\r\n)\./g, "$1..");

class Session implements SmtpSession {
  private closed = false;
  private readonly wire: Wire;

  constructor(wire: Wire) {
    this.wire = wire;
  }

  async send(m: Outgoing): Promise<MailResult> {
    if (this.closed) return { ok: false, temporary: true, detail: "smtp: the session is closed" };
    const problem = checkMessage(m) ?? (isAddress(m.from) && isAddress(m.to) ? null : "smtp: not a plain ASCII address");
    if (problem) return permanent(problem);
    try {
      await this.wire.expect("MAIL FROM", `MAIL FROM:<${m.from}>`, [250]);
      await this.wire.expect("RCPT TO", `RCPT TO:<${m.to}>`, [250, 251]);
      await this.wire.expect("DATA", "DATA", [354]);
      await this.wire.expect("end of data", `${dotStuff(buildMime(m))}.`, [250]);
      return { ok: true };
    } catch (e) {
      // The server refused this message but the connection is fine: reset the
      // transaction so the next message in the batch starts clean. Anything
      // else (a drop, a timeout) leaves the stream in an unknown state.
      if (!(e instanceof SmtpFailure && e.answered && (await this.reset()))) await this.abort();
      return failure(e);
    }
  }

  private async reset(): Promise<boolean> {
    try {
      await this.wire.expect("RSET", "RSET", [250]);
      return true;
    } catch {
      return false;
    }
  }

  private async abort(): Promise<void> {
    this.closed = true;
    await this.wire.close();
  }

  // close says goodbye politely; a server that does not answer is not an error.
  async close(): Promise<void> {
    if (this.closed) return;
    try {
      await this.wire.expect("QUIT", "QUIT", [221]);
    } catch {
      /* the mail has been accepted already; nothing to report */
    }
    await this.abort();
  }
}
