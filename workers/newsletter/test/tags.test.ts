// Message tags (#460): only Resend sends them, as its `tags` list; a message
// without tags — or with an empty object — is sent byte for byte as before on
// every transport; a tag outside Resend's documented rule is refused on every
// transport, before any request or connection.
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildMime } from "../functions/api/newsletter/_mime.ts";
import { sendMail } from "../functions/api/newsletter/_transport.ts";
import type { MailEnv } from "../functions/api/newsletter/_transport.ts";
import { fakeFetch, smtpServer } from "./fakes.ts";

const RESEND: MailEnv = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_RESEND_API_KEY: "re_key" };
const MC: MailEnv = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_MAILCHANNELS_API_KEY: "mc_key" };
const SMTP: MailEnv = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_USER: "mailer@example.com", NEWSLETTER_SMTP_PASSWORD: "s3cret" };
const message = { to: "reader@example.com", subject: "Hi", text: "Body", html: "<p>Body</p>", headers: { "List-Id": "<news.example.com>" }, idempotencyKey: "c1:s1" };
const TAGS = { newsletter_campaign: "0f8fad5b-d9cb-469f-a165-70867728950e", kind: "news-letter_2" };

// bodies sends each message through `env` and returns the raw request bodies.
async function bodies(env: MailEnv, messages: object[]): Promise<string[]> {
  const f = fakeFetch(() => new Response("{}", { status: 200 }));
  try {
    for (const m of messages) assert.deepEqual(await sendMail(env, m as never), { ok: true });
    return f.calls.map((c) => c.init.body as string);
  } finally {
    f.restore();
  }
}

test("Resend: tags go last in the body as [{name, value}]", async () => {
  const [body] = await bodies(RESEND, [{ ...message, tags: TAGS }]);
  assert.equal(
    body,
    JSON.stringify({
      from: "news@example.com",
      to: "reader@example.com",
      subject: "Hi",
      text: "Body",
      html: "<p>Body</p>",
      headers: { "List-Id": "<news.example.com>" },
      tags: [
        { name: "newsletter_campaign", value: TAGS.newsletter_campaign },
        { name: "kind", value: "news-letter_2" },
      ],
    }),
  );
});

test("no tags or empty tags: the Resend and MailChannels bodies and the MIME text are unchanged", async () => {
  for (const env of [RESEND, MC]) {
    const [plain, empty] = await bodies(env, [message, { ...message, tags: {} }]);
    assert.equal(empty, plain);
    assert.doesNotMatch(plain, /tags/);
  }
  const when = new Date("2026-11-26T10:00:00.000Z");
  const out = { ...message, from: "news@example.com", fromName: "" };
  assert.equal(buildMime({ ...out, tags: TAGS }, when, "id-1"), buildMime(out, when, "id-1"));
});

test("MailChannels and SMTP leave tags out silently", async () => {
  const [mc, plain] = await bodies(MC, [{ ...message, tags: TAGS }, message]);
  assert.equal(mc, plain);
  const s = smtpServer();
  assert.deepEqual(await sendMail(SMTP, { ...message, tags: TAGS }, s.connect), { ok: true });
  assert.doesNotMatch(s.state.messages[0], /newsletter_campaign|0f8fad5b/);
});

test("a tag outside Resend's rule is refused on every transport, before any request", async () => {
  const bad: Record<string, string>[] = [
    { "with space": "x" },
    { name: "a.b" },
    { name: "" },
    { "": "x" },
    { name: "zażółć" },
    { ["n".repeat(257)]: "x" },
    { name: "v".repeat(257) },
  ];
  const f = fakeFetch(() => new Response("{}", { status: 200 }));
  try {
    for (const env of [RESEND, MC, SMTP]) {
      for (const tags of bad) {
        const r = await sendMail(env, { ...message, tags });
        assert.ok(!r.ok && !r.temporary && r.detail.startsWith("invalid tag "), JSON.stringify(tags));
      }
    }
    assert.equal(f.calls.length, 0);
    assert.deepEqual(await sendMail(RESEND, { ...message, tags: { ["n".repeat(256)]: "v".repeat(256) } }), { ok: true }, "256 characters are allowed");
  } finally {
    f.restore();
  }
});
