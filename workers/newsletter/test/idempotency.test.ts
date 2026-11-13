// The optional idempotencyKey of a message: Resend's Idempotency-Key header,
// a deterministic Message-ID over SMTP, nothing for MailChannels (its API
// documents no such mechanism). Without a key every request is unchanged —
// transport.test.ts asserts those bytes. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildMime, checkMessage, messageId } from "../functions/api/newsletter/_mime.ts";
import type { Outgoing } from "../functions/api/newsletter/_mime.ts";
import { honoursIdempotencyKey, openMailer, sendMail } from "../functions/api/newsletter/_transport.ts";
import { fakeFetch, smtpServer } from "./fakes.ts";

const message = { to: "reader@example.com", subject: "Hi", text: "Body" };
const KEY = "c-1:sub-00001";
const out = (over: Partial<Outgoing> = {}): Outgoing => ({ from: "news@example.com", fromName: "", ...message, ...over });

test("Resend: the key travels as the Idempotency-Key header, the body is the same as without it", async () => {
  const f = fakeFetch(() => new Response("{}", { status: 200 }));
  try {
    const env = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_RESEND_API_KEY: "re_key" };
    assert.deepEqual(await sendMail(env, { ...message, idempotencyKey: KEY }), { ok: true });
    await sendMail(env, message);
    assert.deepEqual(f.calls[0].init.headers, { authorization: "Bearer re_key", "content-type": "application/json", "Idempotency-Key": KEY });
    assert.deepEqual(f.calls[1].init.headers, { authorization: "Bearer re_key", "content-type": "application/json" });
    assert.equal(f.calls[0].init.body, f.calls[1].init.body, "the key never enters the JSON body");
  } finally {
    f.restore();
  }
});

test("MailChannels: no documented idempotency mechanism, so the request ignores the key", async () => {
  const f = fakeFetch(() => new Response("", { status: 202 }));
  try {
    const env = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAILCHANNELS_API_KEY: "mc_key" };
    await sendMail(env, { ...message, idempotencyKey: KEY });
    await sendMail(env, message);
    assert.deepEqual(f.calls[0].init.headers, f.calls[1].init.headers);
    assert.equal(f.calls[0].init.body, f.calls[1].init.body);
  } finally {
    f.restore();
  }
});

test("SMTP: the same key gives the same Message-ID; without one it stays random", async () => {
  const s = smtpServer();
  const opened = await openMailer({ NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_USER: "mailer@example.com", NEWSLETTER_SMTP_PASSWORD: "s3cret" }, s.connect);
  assert.ok(opened.ok);
  await opened.mailer.send({ ...message, idempotencyKey: KEY });
  await opened.mailer.send({ ...message, idempotencyKey: KEY });
  await opened.mailer.send(message);
  await opened.mailer.close();
  const ids = s.state.messages.map((m) => /\r\nMessage-ID: <([^>]*)>\r\n/.exec(m)?.[1]);
  assert.equal(ids[0], "c-1=3Asub-00001@example.com");
  assert.equal(ids[1], ids[0]);
  assert.match(String(ids[2]), /^[0-9a-f-]{36}@example\.com$/);
});

test("messageId: only [A-Za-z0-9_-] kept, everything else =XX; the boundary keeps the random id", () => {
  assert.equal(messageId(out(), "id-1"), "id-1");
  assert.equal(messageId(out({ idempotencyKey: "a.b/c=d~E_9-" }), "id-1"), "a=2Eb=2Fc=3Dd=7EE_9-");
  const mime = buildMime(out({ html: "<p>x</p>", idempotencyKey: KEY }), new Date(0), "id-2");
  assert.match(mime, /\r\nMessage-ID: <c-1=3Asub-00001@example\.com>\r\n/);
  assert.match(mime, /boundary="=_id-2"/);
});

test("checkMessage: a key must be 1–256 visible ASCII characters", () => {
  assert.equal(checkMessage(out({ idempotencyKey: KEY })), null);
  assert.equal(checkMessage(out({ idempotencyKey: "x".repeat(256) })), null);
  for (const bad of ["", "x".repeat(257), "a b", "a\r\nBcc: v@example.com", "zażółć"]) {
    assert.equal(checkMessage(out({ idempotencyKey: bad })), "invalid idempotency key", JSON.stringify(bad));
  }
});

test("honoursIdempotencyKey: Resend only", () => {
  assert.equal(honoursIdempotencyKey({ NEWSLETTER_RESEND_API_KEY: "k", NEWSLETTER_SMTP_HOST: "h" }), true);
  assert.equal(honoursIdempotencyKey({ NEWSLETTER_MAILCHANNELS_API_KEY: "k" }), false);
  assert.equal(honoursIdempotencyKey({ NEWSLETTER_SMTP_HOST: "h" }), false);
  assert.equal(honoursIdempotencyKey({}), false);
});

// Resend's documented 409s on a keyed request: `invalid_idempotent_request`
// (the key was used for a different payload: a message under it was accepted)
// and `concurrent_idempotent_requests` (still in progress: try later).
const conflict = (body: string) => new Response(body, { status: 409, headers: { "content-type": "application/json" } });
const RESEND = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_RESEND_API_KEY: "re_key" };

test("Resend 409 with a key: reused key = already sent, in progress = temporary, unknown = temporary", async () => {
  const cases: [string, unknown][] = [
    [JSON.stringify({ statusCode: 409, name: "invalid_idempotent_request", message: "different payload" }), { ok: true, duplicate: true }],
    [JSON.stringify({ statusCode: 409, name: "concurrent_idempotent_requests" }), { ok: false, temporary: true, detail: "resend returned 409 concurrent_idempotent_requests" }],
    [JSON.stringify({ name: "something_new" }), { ok: false, temporary: true, detail: "resend returned 409 something_new" }],
    [JSON.stringify({ name: "Not<a>code" }), { ok: false, temporary: true, detail: "resend returned 409" }],
    [JSON.stringify({ name: 42 }), { ok: false, temporary: true, detail: "resend returned 409" }],
    ["null", { ok: false, temporary: true, detail: "resend returned 409" }],
    ["not json", { ok: false, temporary: true, detail: "resend returned 409" }],
  ];
  for (const [body, want] of cases) {
    const f = fakeFetch(() => conflict(body));
    try {
      assert.deepEqual(await sendMail(RESEND, { ...message, idempotencyKey: KEY }), want, body);
    } finally {
      f.restore();
    }
  }
});

test("409 without a key, or from MailChannels, stays a permanent refusal as before", async () => {
  const body = JSON.stringify({ name: "invalid_idempotent_request" });
  const f = fakeFetch(() => conflict(body));
  try {
    assert.deepEqual(await sendMail(RESEND, message), { ok: false, temporary: false, detail: "resend returned 409" });
    const mc = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAILCHANNELS_API_KEY: "mc_key" };
    assert.deepEqual(await sendMail(mc, { ...message, idempotencyKey: KEY }), { ok: false, temporary: false, detail: "mailchannels returned 409" });
  } finally {
    f.restore();
  }
});
