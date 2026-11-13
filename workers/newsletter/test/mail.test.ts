// The confirmation mail on top of the transports. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { sendConfirmation } from "../functions/api/newsletter/_mail.ts";
import { STRINGS } from "../functions/api/newsletter/_i18n.ts";
import { fakeDB, fakeFetch, quietly, smtpServer } from "./fakes.ts";

const URL = "https://example.com/api/newsletter/confirm?token=abc";
const env = (over: Record<string, unknown> = {}) => {
  const db = fakeDB();
  return { db, env: { NEWSLETTER_DB: db, NEWSLETTER_MAIL_FROM: "news@example.com", ...over } as never };
};
const marked = (db: ReturnType<typeof fakeDB>) => db.calls.filter((c) => c.sql.startsWith("UPDATE subscribers SET confirm_sent_at"));

test("Resend: the same mail as always, and the throttle starts", async () => {
  const f = fakeFetch(() => new Response("{}"));
  const { db, env: e } = env({ NEWSLETTER_RESEND_API_KEY: "k", NEWSLETTER_MAIL_FROM_NAME: "Example News" });
  try {
    await sendConfirmation(e, "reader@example.com", "pl", URL);
    assert.deepEqual(f.calls[0].body, {
      from: "Example News <news@example.com>",
      to: "reader@example.com",
      subject: STRINGS.pl.mailSubject,
      text: STRINGS.pl.mailBody.replace("{url}", URL),
    });
    assert.equal(marked(db).length, 1);
    assert.equal(marked(db)[0].args[1], "reader@example.com");
  } finally {
    f.restore();
  }
});

test("an unknown language falls back to English", async () => {
  const f = fakeFetch(() => new Response("{}"));
  const { env: e } = env({ NEWSLETTER_MAILCHANNELS_API_KEY: "k" });
  try {
    await sendConfirmation(e, "reader@example.com", "xx", URL);
    assert.equal((f.calls[0].body as { subject: string }).subject, STRINGS.en.mailSubject);
  } finally {
    f.restore();
  }
});

test("SMTP: the confirmation goes through your own server", async () => {
  const s = smtpServer();
  // Through the real import("cloudflare:sockets") path (test/sockets.mjs).
  (globalThis as { fakeSocketsConnect?: unknown }).fakeSocketsConnect = s.connect;
  const { db, env: e } = env({ NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_USER: "mailer@example.com", NEWSLETTER_SMTP_PASSWORD: "s3cret" });
  await sendConfirmation(e, "reader@example.com", "de", URL);
  assert.equal(s.state.messages.length, 1);
  assert.match(s.state.messages[0], /\r\nTo: reader@example\.com\r\n/);
  assert.equal(marked(db).length, 1);
});

test("a failed send is logged, not thrown, and does not start the throttle", async () => {
  const f = fakeFetch(() => new Response("", { status: 500 }));
  const { db, env: e } = env({ NEWSLETTER_RESEND_API_KEY: "k" });
  try {
    const { warnings } = await quietly(() => sendConfirmation(e, "reader@example.com", "en", URL));
    assert.deepEqual(warnings, ["newsletter mail: resend returned 500"]);
    assert.equal(marked(db).length, 0);
  } finally {
    f.restore();
  }
});

test("without a sender or a transport nothing happens", async () => {
  const f = fakeFetch(() => new Response("{}"));
  try {
    const a = env({ NEWSLETTER_MAIL_FROM: undefined, NEWSLETTER_RESEND_API_KEY: "k" });
    await sendConfirmation(a.env, "reader@example.com", "en", URL);
    const b = env();
    const { warnings } = await quietly(() => sendConfirmation(b.env, "reader@example.com", "en", URL));
    assert.equal(f.calls.length, 0);
    assert.equal(warnings.length, 0);
    assert.equal(a.db.calls.length + b.db.calls.length, 0);
  } finally {
    f.restore();
  }
});
