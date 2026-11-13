// GO-128: what the panel's Settings view shows about the worker, read-only.
//
//   GET  {base}/api/settings            the configuration as this worker sees it
//   POST {base}/api/settings/smtp-test  open an SMTP session and close it unsent
//
// Never a secret value: an API key, the SMTP password, a webhook secret are
// reported only as "configured" booleans. Everything else returned is a plain
// variable from wrangler.toml [vars], which the admin can read there anyway.
// Configuration stays in wrangler.toml and `wrangler secret put`; the panel
// has no way to change it, so a stolen panel session cannot redirect mail.

import { softLimit } from "./bounce-store";
import { takeTestSend } from "./campaign-limit";
import { trackClicks } from "./click-track";
import { DEFAULT_CONFIRM_BATCH } from "./confirm";
import { engineConfig } from "./engine-rate";
import { DEFAULT_DLQ } from "./engine-status";
import { feedSource } from "./feed-fetch";
import { DEFAULT_MAX_ROWS } from "./import";
import { basePath, intVar, json } from "./lib";
import type { Env } from "./lib";
import { openMailer, transportOf, transportProblem } from "./mail/_transport";
import type { SmtpConnect } from "./mail/_transport";
import type { SmtpSocket } from "./mail/_smtpwire";
import { siteOf } from "./render";
import type { RouteContext } from "./router";
import { schemaVersion } from "./schema";
import { unsubscribeWindowDays } from "./stats-headline";

const set = (v: string | undefined): boolean => !!(v ?? "").trim();
const plain = (v: string | undefined): string | null => (v ?? "").trim() || null;

// settingsOf is the whole answer of GET /api/settings, minus the schema version.
export function settingsOf(env: Env) {
  const cfg = engineConfig(env);
  const feed = feedSource(env);
  const smtpHost = plain(env.NEWSLETTER_SMTP_HOST);
  return {
    transport: transportOf(env),
    // Why the configured transport can never send (SMTP only), or null.
    transportProblem: transportProblem(env),
    sender: { email: plain(env.NEWSLETTER_MAIL_FROM), name: plain(env.NEWSLETTER_MAIL_FROM_NAME) },
    siteUrl: plain(env.NEWSLETTER_SITE_URL),
    siteUrlValid: siteOf(env) !== null,
    keys: { resend: set(env.NEWSLETTER_RESEND_API_KEY), mailchannels: set(env.NEWSLETTER_MAILCHANNELS_API_KEY) },
    smtp: smtpHost
      ? {
          host: smtpHost,
          port: env.NEWSLETTER_SMTP_PORT ? Number(env.NEWSLETTER_SMTP_PORT) : 587,
          user: plain(env.NEWSLETTER_SMTP_USER),
          passwordSet: set(env.NEWSLETTER_SMTP_PASSWORD),
          helo: plain(env.NEWSLETTER_SMTP_HELO),
        }
      : null,
    engine: {
      rate: cfg.rate,
      batchSize: cfg.batchSize,
      staleMinutes: cfg.staleMs / 60_000,
      maxAttempts: cfg.maxAttempts,
      queue: !!env.NEWSLETTER_QUEUE,
      deadLetterQueue: plain(env.NEWSLETTER_DEAD_LETTER_QUEUE) ?? DEFAULT_DLQ,
    },
    clicks: { tracking: trackClicks(env), unsubscribeWindowDays: unsubscribeWindowDays(env) },
    bounces: { softLimit: softLimit(env) },
    webhooks: { resend: set(env.NEWSLETTER_RESEND_WEBHOOK_SECRET), mailchannels: set(env.NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY) },
    feedUrl: "url" in feed ? feed.url.href : null,
    confirmBatch: Math.min(intVar(env.NEWSLETTER_CONFIRM_BATCH, DEFAULT_CONFIRM_BATCH), cfg.rate),
    importMaxRows: intVar(env.NEWSLETTER_IMPORT_MAX_ROWS, DEFAULT_MAX_ROWS),
    auth: env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD ? "access" : "basic",
    base: basePath(env),
  };
}

// GET /api/settings → settingsOf + schemaVersion.
export async function getSettings({ env }: RouteContext): Promise<Response> {
  return json({ ...settingsOf(env), schemaVersion: await schemaVersion(env) });
}

const MAX_TRANSCRIPT = 2000; // characters of server replies returned

// tapConnect wraps `connect` so every chunk the SERVER sends is copied into
// `log`. Only the readable side is tapped — what the client writes (AUTH
// carries the password) is never recorded. The tap wraps getReader instead of
// piping the stream, so the socket's own stream is never locked by us and
// STARTTLS can still swap it out.
export function tapConnect(connect: SmtpConnect, log: string[]): SmtpConnect {
  const decoder = new TextDecoder();
  const wrap = (s: SmtpSocket): SmtpSocket => ({
    readable: {
      getReader() {
        const r = s.readable.getReader();
        return {
          async read() {
            const chunk = await r.read();
            if (chunk.value) log.push(decoder.decode(chunk.value, { stream: true }));
            return chunk;
          },
          releaseLock: () => r.releaseLock(),
        };
      },
    } as unknown as ReadableStream<Uint8Array>,
    writable: s.writable,
    startTls: () => wrap(s.startTls()),
    close: () => s.close(),
  });
  return (address, options) => wrap(connect(address, options));
}

// stageOf names where a failed session stopped: "smtp 535 at AUTH: …" → AUTH;
// a failure with no stage happened while connecting.
export const stageOf = (detail: string): string => /\bat (greeting|EHLO|STARTTLS|AUTH)\b/.exec(detail)?.[1] ?? "connect";

const transcript = (log: string[]): string => {
  const all = log.join("").replace(/\r\n/g, "\n").trimEnd();
  return all.length > MAX_TRANSCRIPT ? all.slice(0, MAX_TRANSCRIPT) + "…" : all;
};

// loadSockets imports the Workers sockets API; null where there is none.
export async function loadSockets(): Promise<SmtpConnect | null> {
  try {
    return ((await import("cloudflare:sockets")) as { connect: SmtpConnect }).connect;
  } catch {
    return null;
  }
}

// POST /api/settings/smtp-test → {ok, transport, stage, tls, authenticated,
// reply, detail?}. Connects, greets, upgrades to TLS and logs in exactly as a
// send would, then closes without sending. It counts against the test-send
// limit (10 per hour, shared with campaign test sends): each test is an
// outbound connection an attacker with the session could otherwise repeat.
export async function smtpTest({ env }: RouteContext, connect?: SmtpConnect | null): Promise<Response> {
  const kind = transportOf(env);
  if (kind !== "smtp") {
    const detail = kind ? `the active transport is ${kind}, an HTTP API: there is no session to open; use a test send` : "no mail transport configured";
    return json({ error: "not_smtp", detail, transport: kind }, 409);
  }
  const problem = transportProblem(env);
  if (problem) return json({ error: "transport_unusable", detail: problem, transport: kind }, 503);
  if (!(await takeTestSend(env))) return json({ error: "test_limit", detail: "at most 10 test sends per hour" }, 429, { "retry-after": "3600" });
  const sockets = connect === undefined ? await loadSockets() : connect;
  if (!sockets) return json({ error: "no_sockets", detail: "cloudflare:sockets is not available in this runtime" }, 503);
  const log: string[] = [];
  const opened = await openMailer(env, tapConnect(sockets, log));
  const port = env.NEWSLETTER_SMTP_PORT ? Number(env.NEWSLETTER_SMTP_PORT) : 587;
  const tls = port === 465 ? "implicit" : "starttls";
  if (!opened.ok) {
    return json({ ok: false, transport: kind, stage: stageOf(opened.detail), tls, authenticated: false, reply: transcript(log), detail: opened.detail });
  }
  await opened.mailer.close();
  const authenticated = set(env.NEWSLETTER_SMTP_USER);
  return json({ ok: true, transport: kind, stage: authenticated ? "AUTH" : "EHLO", tls, authenticated, reply: transcript(log) });
}
