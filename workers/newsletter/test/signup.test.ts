// POST /api/newsletter (index.ts). Pins the answers installs already rely on —
// no mail config = single opt-in, a Resend or MailChannels key = double opt-in
// — and the refusal of mail settings that can never send (SMTP on port 25, a
// port that is not a number): `not_configured`, never a silent single opt-in.
// Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost } from "../functions/api/newsletter/index.ts";
import { fakeDB, fakeFetch, quietly } from "./fakes.ts";

const FROM = { NEWSLETTER_MAIL_FROM: "news@example.com" };
const FIELDS = { email: "Reader@Example.com", consent: "1", consent_text: "Send me the newsletter", token: "tok", language: "en" };

function db() {
  const d = fakeDB({ changes: 1 });
  return { ...d, batch: async () => [] };
}

// call runs the handler with Turnstile passing (unless `captcha` is false) and
// every mail provider accepting; it waits for the waitUntil work.
async function call(env: Record<string, unknown>, body: Record<string, string> | null, opts: { form?: boolean; captcha?: boolean } = {}) {
  const f = fakeFetch((url) => {
    if (url.includes("turnstile")) return Response.json({ success: opts.captcha !== false });
    return new Response("{}", { status: url.includes("mailchannels") ? 202 : 200 });
  });
  const d = db();
  const errors: string[] = [];
  const original = console.error;
  console.error = (...a: unknown[]) => void errors.push(a.join(" "));
  const pending: Promise<unknown>[] = [];
  try {
    const headers: Record<string, string> = { "cf-connecting-ip": "192.0.2.1", "accept-language": "en" };
    let payload: string;
    if (opts.form) {
      headers["content-type"] = "application/x-www-form-urlencoded";
      payload = new URLSearchParams(body ?? {}).toString();
    } else {
      headers["content-type"] = "application/json";
      payload = body === null ? "[]" : JSON.stringify(body);
    }
    const request = new Request("https://example.com/api/newsletter", { method: "POST", headers, body: payload });
    const res = await onRequestPost({
      request,
      env: { NEWSLETTER_DB: d, NEWSLETTER_TURNSTILE_SECRET: "s", ...env },
      waitUntil: (p: Promise<unknown>) => void pending.push(p),
    } as never);
    await Promise.all(pending);
    return { res, db: d, fetches: f.calls.map((c) => c.url), errors };
  } finally {
    console.error = original;
    f.restore();
  }
}

const inserted = (d: ReturnType<typeof db>) => d.calls.find((c) => c.sql.startsWith("INSERT INTO subscribers"));

test("no mail configuration: single opt-in, exactly as before", async () => {
  const { res, db: d, fetches } = await call({}, FIELDS);
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { ok: true, confirm: false });
  assert.equal(inserted(d)?.args[2], "confirmed");
  assert.deepEqual(fetches, ["https://challenges.cloudflare.com/turnstile/v0/siteverify"]);
});

test("a sender alone, or an SMTP host on port 25 without a sender, stays single opt-in as before", async () => {
  for (const env of [FROM, { NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" }]) {
    const { res, db: d } = await call(env, FIELDS);
    assert.deepEqual(await res.json(), { ok: true, confirm: false });
    assert.equal(inserted(d)?.args[2], "confirmed");
  }
});

test("Resend or MailChannels key: double opt-in, the confirmation goes out, as before", async () => {
  const cases: [Record<string, string>, string][] = [
    [{ NEWSLETTER_RESEND_API_KEY: "re_key" }, "https://api.resend.com/emails"],
    [{ NEWSLETTER_MAILCHANNELS_API_KEY: "mc_key" }, "https://api.mailchannels.net/tx/v1/send"],
    // A provider key wins over SMTP settings, even unusable ones.
    [{ NEWSLETTER_RESEND_API_KEY: "re_key", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" }, "https://api.resend.com/emails"],
  ];
  for (const [keys, endpoint] of cases) {
    const { res, db: d, fetches } = await call({ ...FROM, ...keys }, FIELDS);
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { ok: true, confirm: true });
    assert.equal(inserted(d)?.args[2], "pending");
    assert.equal(fetches.at(-1), endpoint);
    assert.ok(d.calls.some((c) => c.sql.startsWith("UPDATE subscribers SET confirm_sent_at")));
  }
});

test("unusable SMTP with a sender: JSON not_configured 503, logged, nothing written or verified", async () => {
  for (const port of ["25", "submission"]) {
    const { res, db: d, fetches, errors } = await call({ ...FROM, NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: port }, FIELDS);
    assert.equal(res.status, 503);
    assert.deepEqual(await res.json(), { error: "not_configured" });
    assert.deepEqual(d.calls, []);
    assert.deepEqual(fetches, []);
    assert.equal(errors.length, 1);
    assert.match(errors[0], /^newsletter: sign-up refused, double opt-in is configured but cannot send: smtp: /);
  }
});

test("unusable SMTP with a sender: a form post gets the localized not_configured page", async () => {
  const { res, errors } = await call({ ...FROM, NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" }, FIELDS, { form: true });
  assert.equal(res.status, 503);
  assert.match(await res.text(), /Newsletter sign-up is not available at the moment\./);
  assert.equal(errors.length, 1);
});

test("usable SMTP with a sender: double opt-in", async () => {
  // Node has no cloudflare:sockets: the confirmation itself is only logged.
  const { result, warnings } = await quietly(() => call({ ...FROM, NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "465" }, FIELDS));
  const { res, db: d } = result;
  assert.ok(warnings.some((w) => /cloudflare:sockets/.test(w)));
  assert.deepEqual(await res.json(), { ok: true, confirm: true });
  assert.equal(inserted(d)?.args[2], "pending");
});

test("form post: 303 to the thank-you URL", async () => {
  const { res } = await call({ NEWSLETTER_THANKS_URL: "/{lang}/thanks/" }, { ...FIELDS, "cf-turnstile-response": "t" }, { form: true });
  assert.equal(res.status, 303);
  assert.equal(res.headers.get("location"), "/en/thanks/");
});

test("the existing refusals keep their codes", async () => {
  const cases: [Record<string, unknown>, Record<string, string> | null, number, string, boolean?][] = [
    [{}, null, 400, "invalid_body"],
    [{ NEWSLETTER_TURNSTILE_SECRET: undefined }, FIELDS, 503, "not_configured"],
    [{}, { ...FIELDS, email: "nope" }, 422, "invalid_email"],
    [{}, { ...FIELDS, consent_text: "" }, 422, "consent_required"],
    [{}, { ...FIELDS, token: "" }, 403, "captcha_failed"],
    [{}, FIELDS, 403, "captcha_failed", false],
  ];
  for (const [env, body, status, error, captcha] of cases) {
    const { res } = await call(env, body, { captcha });
    assert.equal(res.status, status, error);
    assert.deepEqual(await res.json(), { error });
  }
  const page = await call({}, { ...FIELDS, email: "nope" }, { form: true });
  assert.equal(page.res.status, 422);
  assert.match(await page.res.text(), /<html/);
});
