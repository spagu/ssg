// POST /api/contact against a stubbed fetch. Run from the repository root:
//   node --import ./workers/contact-form/test/resolve.mjs --test 'workers/contact-form/test/*.test.ts'
import { afterEach, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost } from "../functions/api/contact.ts";
import { missingMailConfig, provider, sendMail } from "../functions/api/_mail.ts";
import type { MailEnv } from "../functions/api/_mail.ts";

type Call = { url: string; init: RequestInit };
type Reply = { status?: number; body?: unknown; throws?: boolean };

const realFetch = globalThis.fetch;
const realError = console.error;
let calls: Call[];
let logs: string[];
let replies: Record<string, Reply>;

beforeEach(() => {
  calls = [];
  logs = [];
  replies = { turnstile: { body: { success: true } } };
  console.error = (...a: unknown[]) => void logs.push(a.join(" "));
  globalThis.fetch = (async (input: string, init: RequestInit) => {
    calls.push({ url: String(input), init });
    const r = String(input).includes("turnstile") ? replies.turnstile : replies.mail ?? {};
    if (r.throws) throw new Error("network down");
    return new Response(JSON.stringify(r.body ?? {}), { status: r.status ?? 200 });
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  console.error = realError;
});

const BASE = { CONTACT_TURNSTILE_SECRET: "ts", CONTACT_TO: "team@example.com", CONTACT_FROM: "noreply@example.com" };
const FIELDS = { name: "Ann", email: "ann@example.com", message: "Hello", "cf-turnstile-response": "tok" };

function post(env: Record<string, string>, fields: Record<string, string> = FIELDS, raw?: string) {
  let request: Request;
  if (raw !== undefined) {
    request = new Request("https://example.com/api/contact", {
      method: "POST",
      body: raw,
      headers: { "content-type": "multipart/form-data; boundary=x" },
    });
  } else {
    const body = new FormData();
    for (const [k, v] of Object.entries(fields)) body.append(k, v);
    request = new Request("https://example.com/api/contact", {
      method: "POST",
      body,
      headers: { "cf-connecting-ip": "203.0.113.7" },
    });
  }
  return onRequestPost({ request, env } as unknown as Parameters<typeof onRequestPost>[0]);
}

const mailCall = () => calls.find((c) => !c.url.includes("turnstile"));
const headers = (c: Call) => c.init.headers as Record<string, string>;
const payload = (c: Call) => JSON.parse(String(c.init.body));

test("sends through Resend with the prefixed key", async () => {
  const res = await post({ ...BASE, CONTACT_RESEND_API_KEY: "re_own", RESEND_API_KEY: "re_shared" });
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { ok: true });
  const c = mailCall()!;
  assert.equal(c.url, "https://api.resend.com/emails");
  assert.equal(headers(c).authorization, "Bearer re_own");
  assert.deepEqual(payload(c), {
    from: "noreply@example.com",
    to: "team@example.com",
    subject: "Contact form: Ann",
    text: "From: Ann <ann@example.com>\n\nHello",
    reply_to: "ann@example.com",
  });
});

test("sends through MailChannels with the X-Api-Key header", async () => {
  const res = await post({ ...BASE, CONTACT_MAILCHANNELS_API_KEY: "mc_key" });
  assert.equal(res.status, 200);
  const c = mailCall()!;
  assert.equal(c.url, "https://api.mailchannels.net/tx/v1/send");
  assert.equal(headers(c)["x-api-key"], "mc_key");
  const p = payload(c);
  assert.deepEqual(p.personalizations, [{ to: [{ email: "team@example.com" }] }]);
  assert.deepEqual(p.reply_to, { email: "ann@example.com", name: "Ann" });
  assert.deepEqual(p.content, [{ type: "text/plain", value: "From: Ann <ann@example.com>\n\nHello" }]);
});

test("the legacy RESEND_API_KEY is a fallback only", async () => {
  await post({ ...BASE, RESEND_API_KEY: "re_shared" });
  assert.equal(headers(mailCall()!).authorization, "Bearer re_shared");

  calls = [];
  await post({ ...BASE, CONTACT_MAILCHANNELS_API_KEY: "mc_key", RESEND_API_KEY: "re_shared" });
  assert.equal(mailCall()!.url, "https://api.mailchannels.net/tx/v1/send");
});

test("no mail key answers 503 before spending the captcha", async () => {
  const res = await post(BASE);
  assert.equal(res.status, 503);
  assert.match((await res.json()).error, /CONTACT_RESEND_API_KEY or CONTACT_MAILCHANNELS_API_KEY/);
  assert.equal(calls.length, 0);
  assert.match(logs.join("\n"), /mail not configured/);
});

test("missing CONTACT_TO or CONTACT_FROM answers 503", async () => {
  const res = await post({ CONTACT_TURNSTILE_SECRET: "ts", CONTACT_RESEND_API_KEY: "k" });
  assert.equal(res.status, 503);
  assert.match((await res.json()).error, /CONTACT_TO and CONTACT_FROM/);
});

test("Turnstile behaviour is unchanged", async () => {
  const env = { ...BASE, CONTACT_RESEND_API_KEY: "k" };
  replies.turnstile = { body: { success: false } };
  assert.equal((await post(env)).status, 403);
  assert.equal(mailCall(), undefined);

  const { "cf-turnstile-response": _, ...noToken } = FIELDS;
  assert.equal((await post(env, noToken)).status, 403);

  const noSecret = { CONTACT_TO: "a@example.com", CONTACT_FROM: "b@example.com", CONTACT_RESEND_API_KEY: "k" };
  const res = await post(noSecret);
  assert.equal(res.status, 503);
  assert.deepEqual(await res.json(), { error: "contact form not configured" });

  replies.turnstile = { body: { success: true } };
  calls = [];
  const { CONTACT_TURNSTILE_SECRET: __, ...shared } = env;
  assert.equal((await post({ ...shared, TURNSTILE_SECRET: "shared" })).status, 200);
  const verify = calls[0].init.body as FormData;
  assert.equal(verify.get("secret"), "shared");
  assert.equal(verify.get("remoteip"), "203.0.113.7");
});

test("bad input is rejected", async () => {
  const env = { ...BASE, CONTACT_RESEND_API_KEY: "k" };
  assert.equal((await post(env, {}, "not multipart")).status, 400);
  assert.equal((await post(env, { name: "Ann" })).status, 422);
});

test("provider errors answer 502 and are logged", async () => {
  const env = { ...BASE, CONTACT_MAILCHANNELS_API_KEY: "mc" };
  replies.mail = { status: 401 };
  assert.equal((await post(env)).status, 502);
  assert.match(logs.join("\n"), /mailchannels returned 401/);

  replies.mail = { throws: true };
  assert.equal((await post(env)).status, 502);
  assert.match(logs.join("\n"), /request failed: Error: network down/);
});

test("_mail helpers", async () => {
  const none: MailEnv = { CONTACT_TO: "a@example.com", CONTACT_FROM: "b@example.com" };
  assert.equal(provider(none), null);
  assert.deepEqual(provider({ CONTACT_RESEND_API_KEY: "r", CONTACT_MAILCHANNELS_API_KEY: "m" }), { kind: "resend", key: "r" });
  assert.equal(missingMailConfig({ ...none, CONTACT_MAILCHANNELS_API_KEY: "m" }), "");
  assert.equal(await sendMail(none, { replyTo: { email: "x@example.com", name: "X" }, subject: "s", text: "t" }), false);
  assert.match(logs.join("\n"), /no provider key set/);
});
