// Transport choice and the HTTP providers. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { openMailer, sendMail, transportOf, transportProblem } from "../functions/api/newsletter/_transport.ts";
import type { MailEnv } from "../functions/api/newsletter/_transport.ts";
import { fakeFetch, quietly, smtpServer } from "./fakes.ts";

const SMTP: MailEnv = {
  NEWSLETTER_MAIL_FROM: "news@example.com",
  NEWSLETTER_SMTP_HOST: "smtp.example.com",
  NEWSLETTER_SMTP_USER: "mailer@example.com",
  NEWSLETTER_SMTP_PASSWORD: "s3cret",
};
const message = { to: "reader@example.com", subject: "Hi", text: "Body" };

test("precedence: Resend, then MailChannels, then SMTP", () => {
  assert.equal(transportOf({}), null);
  assert.equal(transportOf({ NEWSLETTER_SMTP_HOST: "h" }), "smtp");
  assert.equal(transportOf({ NEWSLETTER_SMTP_HOST: "h", NEWSLETTER_MAILCHANNELS_API_KEY: "m" }), "mailchannels");
  assert.equal(transportOf({ NEWSLETTER_SMTP_HOST: "h", NEWSLETTER_MAILCHANNELS_API_KEY: "m", NEWSLETTER_RESEND_API_KEY: "r" }), "resend");
});

test("a Resend key wins over SMTP settings: no socket is opened", async () => {
  const s = smtpServer();
  const f = fakeFetch(() => new Response("{}", { status: 200 }));
  try {
    const r = await sendMail({ ...SMTP, NEWSLETTER_RESEND_API_KEY: "re_key" }, message, s.connect);
    assert.deepEqual(r, { ok: true });
    assert.equal(s.state.connects.length, 0);
    assert.equal(f.calls[0].url, "https://api.resend.com/emails");
  } finally {
    f.restore();
  }
});

test("Resend: the request is exactly what it always was", async () => {
  const f = fakeFetch(() => new Response("{}", { status: 200 }));
  try {
    const env = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAIL_FROM_NAME: "Example News", NEWSLETTER_RESEND_API_KEY: "re_key" };
    assert.deepEqual(await sendMail(env, message), { ok: true });
    assert.equal(f.calls[0].init.method, "POST");
    assert.deepEqual(f.calls[0].init.headers, { authorization: "Bearer re_key", "content-type": "application/json" });
    assert.equal(f.calls[0].init.body, JSON.stringify({ from: "Example News <news@example.com>", to: "reader@example.com", subject: "Hi", text: "Body" }));
    // html and headers are added only when given.
    await sendMail({ ...env, NEWSLETTER_MAIL_FROM_NAME: undefined }, { ...message, html: "<p>Body</p>", headers: { "List-Unsubscribe": "<https://example.com/u>" } });
    assert.deepEqual(f.calls[1].body, {
      from: "news@example.com",
      to: "reader@example.com",
      subject: "Hi",
      text: "Body",
      html: "<p>Body</p>",
      headers: { "List-Unsubscribe": "<https://example.com/u>" },
    });
  } finally {
    f.restore();
  }
});

test("MailChannels: the request is exactly what it always was", async () => {
  const f = fakeFetch(() => new Response("", { status: 202 }));
  try {
    const env = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAIL_FROM_NAME: "Example News", NEWSLETTER_MAILCHANNELS_API_KEY: "mc_key" };
    assert.deepEqual(await sendMail(env, message), { ok: true });
    assert.equal(f.calls[0].url, "https://api.mailchannels.net/tx/v1/send");
    assert.deepEqual(f.calls[0].init.headers, { "x-api-key": "mc_key", "content-type": "application/json" });
    assert.equal(
      f.calls[0].init.body,
      JSON.stringify({
        personalizations: [{ to: [{ email: "reader@example.com" }] }],
        from: { email: "news@example.com", name: "Example News" },
        subject: "Hi",
        content: [{ type: "text/plain", value: "Body" }],
      }),
    );
    await sendMail({ ...env, NEWSLETTER_MAIL_FROM_NAME: "" }, { ...message, html: "<p>B</p>", headers: { "List-Id": "<news.example.com>" } });
    assert.deepEqual(f.calls[1].body, {
      personalizations: [{ to: [{ email: "reader@example.com" }] }],
      from: { email: "news@example.com" },
      subject: "Hi",
      content: [{ type: "text/plain", value: "Body" }, { type: "text/html", value: "<p>B</p>" }],
      headers: { "List-Id": "<news.example.com>" },
    });
  } finally {
    f.restore();
  }
});

test("HTTP providers: 429 and 5xx are temporary, other refusals permanent, a throw temporary", async () => {
  const env = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_RESEND_API_KEY: "k" };
  for (const [status, temporary] of [[429, true], [503, true], [422, false], [401, false]] as [number, boolean][]) {
    const f = fakeFetch(() => new Response("", { status }));
    try {
      assert.deepEqual(await sendMail(env, message), { ok: false, temporary, detail: `resend returned ${status}` });
    } finally {
      f.restore();
    }
  }
  const f = fakeFetch(() => Promise.reject(new Error("network down")));
  try {
    assert.deepEqual(await sendMail(env, message), { ok: false, temporary: true, detail: "resend: Error: network down" });
  } finally {
    f.restore();
  }
});

test("header injection is refused on every transport, before any request", async () => {
  const f = fakeFetch(() => new Response("{}"));
  try {
    for (const env of [{ NEWSLETTER_RESEND_API_KEY: "k" }, { NEWSLETTER_MAILCHANNELS_API_KEY: "k" }, SMTP]) {
      const s = smtpServer();
      const r = await sendMail({ NEWSLETTER_MAIL_FROM: "news@example.com", ...env }, { ...message, subject: "x\r\nBcc: v@example.com" }, s.connect);
      assert.deepEqual(r, { ok: false, temporary: false, detail: "CR or LF in subject" });
      assert.equal(s.state.connects.length, 0);
    }
    assert.equal(f.calls.length, 0);
  } finally {
    f.restore();
  }
});

test("no sender or no transport is permanent", async () => {
  assert.deepEqual(await sendMail({ NEWSLETTER_RESEND_API_KEY: "k" }, message), {
    ok: false,
    temporary: false,
    detail: "no sender: NEWSLETTER_MAIL_FROM is not set",
  });
  assert.deepEqual(await sendMail({ NEWSLETTER_MAIL_FROM: "news@example.com" }, message), {
    ok: false,
    temporary: false,
    detail: "no mail transport configured",
  });
});

test("SMTP through sendMail: one connection, sent, closed", async () => {
  const s = smtpServer();
  assert.deepEqual(await sendMail({ ...SMTP, NEWSLETTER_SMTP_PORT: "465", NEWSLETTER_MAIL_FROM_NAME: "Example News" }, message, s.connect), { ok: true });
  assert.equal(s.state.connects[0].address.port, 465);
  assert.equal(s.state.log[0].line, "EHLO example.com"); // the sender's domain
  assert.equal(s.state.log.at(-1)!.line, "QUIT");
  assert.match(s.state.messages[0], /^From: "Example News" <news@example\.com>/);
});

test("SMTP defaults to 587 and takes NEWSLETTER_SMTP_HELO", async () => {
  const s = smtpServer();
  assert.deepEqual(await sendMail({ ...SMTP, NEWSLETTER_SMTP_HELO: "mailer.example.com" }, message, s.connect), { ok: true });
  assert.equal(s.state.connects[0].address.port, 587);
  assert.equal(s.state.log[0].line, "EHLO mailer.example.com");
});

test("SMTP port 25 is refused with a message", async () => {
  const s = smtpServer();
  const r = await sendMail({ ...SMTP, NEWSLETTER_SMTP_PORT: "25" }, message, s.connect);
  assert.ok(!r.ok && !r.temporary && /port 25 is blocked by Cloudflare/.test(r.detail));
});

test("one SMTP mailer sends a batch; a per-message sender overrides the env", async () => {
  const s = smtpServer();
  const opened = await openMailer(SMTP, s.connect);
  assert.ok(opened.ok);
  assert.deepEqual(await opened.mailer.send(message), { ok: true });
  assert.deepEqual(await opened.mailer.send({ ...message, from: "other@example.com", fromName: "Other" }), { ok: true });
  const bad = await opened.mailer.send({ ...message, to: "x@example.com\nBcc: y@example.com" });
  assert.ok(!bad.ok && !bad.temporary);
  await opened.mailer.close();
  assert.equal(s.state.connects.length, 1);
  assert.equal(s.state.messages.length, 2);
  assert.match(s.state.messages[1], /^From: "Other" <other@example\.com>/);
});

test("an HTTP mailer: send and a no-op close", async () => {
  const f = fakeFetch(() => new Response("{}"));
  try {
    const opened = await openMailer({ NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAILCHANNELS_API_KEY: "k" });
    assert.ok(opened.ok);
    assert.deepEqual(await opened.mailer.send(message), { ok: true });
    const bad = await opened.mailer.send({ ...message, fromName: "a\nb" });
    assert.deepEqual(bad, { ok: false, temporary: false, detail: "CR or LF in display name" });
    await opened.mailer.close();
    assert.equal(f.calls.length, 1);
  } finally {
    f.restore();
  }
});

test("without cloudflare:sockets SMTP fails permanently and warns once", async () => {
  const { result, warnings } = await quietly(async () => [
    await sendMail({ ...SMTP, NEWSLETTER_MAIL_FROM: "@" }, message),
    await sendMail(SMTP, message),
  ]);
  for (const r of result) assert.deepEqual(r, { ok: false, temporary: false, detail: "smtp: cloudflare:sockets is not available in this runtime" });
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /cloudflare:sockets is not available/);
});

test("SMTP helo falls back to localhost without a sender domain", async () => {
  const s = smtpServer();
  const opened = await openMailer({ NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "465" }, s.connect);
  assert.ok(opened.ok);
  assert.equal(s.state.log[0].line, "EHLO localhost");
  const r = await opened.mailer.send(message);
  assert.deepEqual(r, { ok: false, temporary: false, detail: "no sender: NEWSLETTER_MAIL_FROM is not set" });
  await opened.mailer.close();
});

test("an SMTP open failure comes back from sendMail", async () => {
  const s = smtpServer({ on: (st) => (st === "AUTH" ? "535 5.7.8 no" : undefined) });
  const r = await sendMail(SMTP, message, s.connect);
  assert.deepEqual(r, { ok: false, temporary: false, detail: "smtp 535 at AUTH: 5.7.8 no" });
});

test("transportProblem: only an SMTP setting that can never work is a problem", () => {
  assert.equal(transportProblem({}), null);
  assert.equal(transportProblem({ NEWSLETTER_RESEND_API_KEY: "k", NEWSLETTER_SMTP_PORT: "25", NEWSLETTER_SMTP_HOST: "h" }), null);
  assert.equal(transportProblem({ NEWSLETTER_MAILCHANNELS_API_KEY: "k", NEWSLETTER_SMTP_HOST: "h", NEWSLETTER_SMTP_PORT: "x" }), null);
  assert.equal(transportProblem(SMTP), null);
  assert.equal(transportProblem({ ...SMTP, NEWSLETTER_SMTP_PORT: "465" }), null);
  assert.match(String(transportProblem({ ...SMTP, NEWSLETTER_SMTP_PORT: "25" })), /port 25 is blocked/);
  assert.equal(transportProblem({ ...SMTP, NEWSLETTER_SMTP_PORT: "submission" }), "smtp: invalid port NaN");
  assert.equal(transportProblem({ ...SMTP, NEWSLETTER_SMTP_PORT: "70000" }), "smtp: invalid port 70000");
  assert.equal(transportProblem({ ...SMTP, NEWSLETTER_SMTP_PASSWORD: undefined }), "smtp: a user is set but the password is not");
  assert.equal(transportProblem({ ...SMTP, NEWSLETTER_SMTP_HELO: "bad name" }), "smtp: invalid EHLO name");
});
