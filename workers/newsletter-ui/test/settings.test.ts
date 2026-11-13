// GO-128: GET /api/settings (never a secret) and POST /api/settings/smtp-test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";
import { loadSockets, smtpTest, stageOf, tapConnect } from "../src/settings.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";

const B = "/newsletter-admin";
const SECRETS = {
  NEWSLETTER_RESEND_API_KEY: "re_secret_value",
  NEWSLETTER_SMTP_PASSWORD: "smtp-secret-value",
  NEWSLETTER_RESEND_WEBHOOK_SECRET: "whsec_secret_value",
  NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY: "pem-secret-value",
  NEWSLETTER_HASH_SALT: "salt-secret-value",
};

async function call(env: Record<string, unknown>, method: string, path: string) {
  await ensureSchema(env as never);
  return handleFetch(req(B + path, { method, auth: PASSWORD }), env as never, fakeCtx() as never);
}

test("settings: reports configuration, never a secret value", async () => {
  const env = makeEnv({
    ...SECRETS,
    NEWSLETTER_SITE_URL: "https://example.com",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    NEWSLETTER_MAIL_FROM_NAME: "Example News",
    NEWSLETTER_SMTP_HOST: "smtp.example.com",
    NEWSLETTER_SMTP_PORT: "465",
    NEWSLETTER_SMTP_USER: "mailer@example.com",
    NEWSLETTER_SEND_RATE: "120",
    NEWSLETTER_SEND_BATCH: "20",
    NEWSLETTER_TRACK_CLICKS: "true",
    NEWSLETTER_CONFIRM_BATCH: "500",
    NEWSLETTER_DEAD_LETTER_QUEUE: "my-dlq",
  });
  const res = await call(env, "GET", "/api/settings");
  assert.equal(res.status, 200);
  const text = await res.text();
  for (const v of Object.values(SECRETS)) assert.ok(!text.includes(v), `leaked ${v}`);
  const s = JSON.parse(text);
  assert.equal(s.transport, "resend");
  assert.deepEqual(s.sender, { email: "news@example.com", name: "Example News" });
  assert.equal(s.siteUrlValid, true);
  assert.deepEqual(s.keys, { resend: true, mailchannels: false });
  assert.deepEqual(s.smtp, { host: "smtp.example.com", port: 465, user: "mailer@example.com", passwordSet: true, helo: null });
  assert.deepEqual(s.engine, { rate: 120, batchSize: 20, staleMinutes: 30, maxAttempts: 3, queue: true, deadLetterQueue: "my-dlq" });
  assert.deepEqual(s.clicks, { tracking: true, unsubscribeWindowDays: 14 });
  assert.deepEqual(s.webhooks, { resend: true, mailchannels: true });
  assert.equal(s.feedUrl, "https://example.com/feed.xml");
  assert.equal(s.confirmBatch, 120, "never above the rate");
  assert.equal(s.importMaxRows, 20000);
  assert.equal(s.auth, "basic");
  assert.equal(s.base, B);
  assert.equal(typeof s.schemaVersion, "number");
});

test("settings: an empty configuration reads as nulls and defaults", async () => {
  const env = makeEnv({ NEWSLETTER_QUEUE: undefined, NEWSLETTER_ACCESS_TEAM: "t", NEWSLETTER_ACCESS_AUD: "a" });
  delete env.NEWSLETTER_QUEUE;
  const s = (await import("../src/settings.ts")).settingsOf(env as never);
  assert.equal(s.transport, null);
  assert.deepEqual(s.sender, { email: null, name: null });
  assert.equal(s.siteUrl, null);
  assert.equal(s.siteUrlValid, false);
  assert.equal(s.smtp, null);
  assert.equal(s.engine.queue, false);
  assert.equal(s.engine.deadLetterQueue, "ssg-newsletter-send-dlq");
  assert.equal(s.feedUrl, null);
  assert.equal(s.auth, "access");
  const smtp = (await import("../src/settings.ts")).settingsOf({ NEWSLETTER_SMTP_HOST: "smtp.example.com" } as never).smtp;
  assert.deepEqual(smtp, { host: "smtp.example.com", port: 587, user: null, passwordSet: false, helo: null });
});

// fakeServer is a scripted SMTP server: `replies` answers each command by its
// verb; the client's writes are recorded so the test can prove they are not
// in the transcript.
function fakeServer(o: { auth?: string; starttls?: boolean; greeting?: string } = {}) {
  const written: string[] = [];
  const enc = new TextEncoder();
  const dec = new TextDecoder();
  let closed = 0;
  const socket = (greeting: string | null) => {
    let push: (s: string) => void = () => {};
    const readable = new ReadableStream<Uint8Array>({
      start(c) {
        push = (s) => c.enqueue(enc.encode(s));
        if (greeting) push(greeting);
      },
    });
    const writable = new WritableStream<Uint8Array>({
      write(chunk) {
        const line = dec.decode(chunk).trim();
        written.push(line);
        const verb = line.split(" ")[0];
        if (verb === "EHLO") push(`250-smtp.example.com\r\n${o.starttls === false ? "" : "250-STARTTLS\r\n"}250 AUTH PLAIN\r\n`);
        else if (verb === "STARTTLS") push("220 2.0.0 go ahead\r\n");
        else if (verb === "AUTH") push(o.auth ?? "235 2.7.0 Authentication successful\r\n");
        else push("221 bye\r\n");
      },
    });
    const s: Record<string, unknown> = { readable, writable, startTls: () => socket(null), close: async () => void closed++ };
    return s;
  };
  const connect = () => socket(o.greeting ?? "220 smtp.example.com ESMTP ready\r\n");
  return { connect: connect as never, written, closed: () => closed };
}

const smtpEnv = (over: Record<string, unknown> = {}) =>
  makeEnv({
    NEWSLETTER_SMTP_HOST: "smtp.example.com",
    NEWSLETTER_SMTP_USER: "mailer@example.com",
    NEWSLETTER_SMTP_PASSWORD: "s3cret-pass",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    ...over,
  });

const ctx = (env: Record<string, unknown>) => ({ env }) as never;

test("smtp-test: logs in, closes unsent, returns the server's words only", async () => {
  const env = smtpEnv();
  await ensureSchema(env as never);
  const server = fakeServer();
  const res = await smtpTest(ctx(env), server.connect);
  const body = await res.json();
  assert.equal(res.status, 200);
  assert.equal(body.ok, true);
  assert.equal(body.stage, "AUTH");
  assert.equal(body.tls, "starttls");
  assert.equal(body.authenticated, true);
  assert.match(body.reply, /220 smtp\.example\.com ESMTP ready/);
  assert.match(body.reply, /235 2\.7\.0/);
  assert.ok(server.written.some((l) => l.startsWith("AUTH PLAIN")), "it did log in");
  assert.ok(!server.written.some((l) => l.startsWith("MAIL")), "nothing was sent");
  assert.ok(!body.reply.includes(btoa("\0mailer@example.com\0s3cret-pass")), "client writes (the password) are not in the transcript");
  assert.ok(server.closed() >= 1);
});

test("smtp-test: without a user it stops after EHLO; port 465 is implicit TLS", async () => {
  const env = smtpEnv({ NEWSLETTER_SMTP_USER: "", NEWSLETTER_SMTP_PORT: "465" });
  await ensureSchema(env as never);
  const body = await (await smtpTest(ctx(env), fakeServer().connect)).json();
  assert.deepEqual([body.ok, body.stage, body.tls, body.authenticated], [true, "EHLO", "implicit", false]);
});

test("smtp-test: a refused login reports the stage and the reply", async () => {
  const env = smtpEnv();
  await ensureSchema(env as never);
  const body = await (await smtpTest(ctx(env), fakeServer({ auth: "535 5.7.8 Authentication credentials invalid\r\n" }).connect)).json();
  assert.equal(body.ok, false);
  assert.equal(body.stage, "AUTH");
  assert.match(body.detail, /535/);
  assert.match(body.reply, /535 5\.7\.8/);
});

test("smtp-test: a long transcript is cut", async () => {
  const env = smtpEnv({ NEWSLETTER_SMTP_USER: "" });
  await ensureSchema(env as never);
  const body = await (await smtpTest(ctx(env), fakeServer({ greeting: "220 " + "x".repeat(3000) + "\r\n" }).connect)).json();
  assert.ok(body.reply.endsWith("…"));
  assert.equal(body.reply.length, 2001);
});

test("smtp-test: refusals before any connection", async () => {
  const http = makeEnv({ NEWSLETTER_RESEND_API_KEY: "re_x" });
  let r = await call(http, "POST", "/api/settings/smtp-test");
  assert.equal(r.status, 409);
  assert.equal((await r.json()).transport, "resend");
  r = await call(makeEnv(), "POST", "/api/settings/smtp-test");
  assert.equal(r.status, 409);
  assert.match((await r.json()).detail, /no mail transport/);

  const env = smtpEnv();
  await ensureSchema(env as never);
  r = await smtpTest(ctx(env), null);
  assert.equal(r.status, 503, "no sockets API");
  // without an injected connect it loads cloudflare:sockets, which Node lacks
  r = await call(env, "POST", "/api/settings/smtp-test");
  assert.equal(r.status, 503);
  assert.equal(await loadSockets(), null);
  for (let i = 0; i < 10; i++) await smtpTest(ctx(env), null);
  r = await smtpTest(ctx(env), null);
  assert.equal(r.status, 429);
  assert.equal(r.headers.get("retry-after"), "3600");
});

test("loadSockets: returns the Workers API where it exists", async () => {
  registerHooks({
    resolve: (specifier, context, next) =>
      specifier === "cloudflare:sockets" ? { url: "data:text/javascript,export const connect = () => 'socket';", shortCircuit: true } : next(specifier, context),
  });
  const connect = await loadSockets();
  assert.equal(typeof connect, "function");
});

test("stageOf and tapConnect", async () => {
  assert.equal(stageOf("smtp 554 at greeting: no"), "greeting");
  assert.equal(stageOf("smtp: the server does not offer STARTTLS; refusing"), "connect");
  assert.equal(stageOf("smtp: connection lost at STARTTLS: x"), "STARTTLS");
  const log: string[] = [];
  const raw = { readable: new ReadableStream({ start: (c) => c.close() }), writable: new WritableStream(), startTls: () => raw, close: async () => {} };
  const tapped = tapConnect((() => raw) as never, log)({ hostname: "h", port: 1 }, { secureTransport: "on", allowHalfOpen: false });
  const reader = tapped.readable.getReader();
  assert.equal((await reader.read()).done, true);
  reader.releaseLock();
  assert.deepEqual(log, []);
  assert.ok(tapped.startTls());
  await tapped.close();
});
