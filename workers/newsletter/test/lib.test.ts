// Shared helpers. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  doubleOptIn,
  isToken,
  mailConfigProblem,
  json,
  newToken,
  requireAdmin,
  retentionDays,
  saltedHash,
  sha256hex,
  turnstileSecret,
  verifyTurnstile,
} from "../functions/api/newsletter/_lib.ts";
import { fakeFetch } from "./fakes.ts";

const env = (over: Record<string, unknown> = {}) => ({ NEWSLETTER_DB: null, ...over }) as never;

test("doubleOptIn: a sender plus any one transport", () => {
  assert.equal(doubleOptIn(env()), false);
  assert.equal(doubleOptIn(env({ NEWSLETTER_MAIL_FROM: "news@example.com" })), false);
  assert.equal(doubleOptIn(env({ NEWSLETTER_RESEND_API_KEY: "k" })), false);
  for (const key of ["NEWSLETTER_RESEND_API_KEY", "NEWSLETTER_MAILCHANNELS_API_KEY", "NEWSLETTER_SMTP_HOST"]) {
    assert.equal(doubleOptIn(env({ NEWSLETTER_MAIL_FROM: "news@example.com", [key]: "x" })), true, key);
  }
});

test("doubleOptIn / mailConfigProblem: an SMTP setting that can never send is not double opt-in, and is a problem", () => {
  const smtp = { NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com" };
  for (const port of ["25", "smtp", "0"]) {
    assert.equal(doubleOptIn(env({ ...smtp, NEWSLETTER_SMTP_PORT: port })), false, port);
    assert.match(String(mailConfigProblem(env({ ...smtp, NEWSLETTER_SMTP_PORT: port }))), /^smtp: /, port);
  }
  assert.equal(doubleOptIn(env({ ...smtp, NEWSLETTER_SMTP_PORT: "465" })), true);
  assert.equal(mailConfigProblem(env({ ...smtp, NEWSLETTER_SMTP_PORT: "465" })), null);
  // No sender: single opt-in as before, nothing to refuse.
  assert.equal(mailConfigProblem(env({ NEWSLETTER_SMTP_HOST: "h", NEWSLETTER_SMTP_PORT: "25" })), null);
  // A provider key wins over SMTP, whatever the SMTP settings say.
  const resend = env({ ...smtp, NEWSLETTER_SMTP_PORT: "25", NEWSLETTER_RESEND_API_KEY: "k" });
  assert.equal(doubleOptIn(resend), true);
  assert.equal(mailConfigProblem(resend), null);
  assert.equal(mailConfigProblem(env()), null);
});

test("json: status, type, no-store", async () => {
  const res = json({ a: 1 }, 418);
  assert.equal(res.status, 418);
  assert.equal(res.headers.get("content-type"), "application/json");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.deepEqual(await res.json(), { a: 1 });
  assert.equal(json(null).status, 200);
});

test("sha256hex and saltedHash", async () => {
  assert.equal(await sha256hex("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  assert.equal(await saltedHash(env(), "1.2.3.4"), null);
  assert.equal(await saltedHash(env({ NEWSLETTER_HASH_SALT: "s" }), null), null);
  assert.equal(await saltedHash(env({ NEWSLETTER_HASH_SALT: "s" }), "abc"), await sha256hex("sabc"));
});

test("turnstileSecret: the prefixed one wins", () => {
  assert.equal(turnstileSecret(env({ NEWSLETTER_TURNSTILE_SECRET: "a", TURNSTILE_SECRET: "b" })), "a");
  assert.equal(turnstileSecret(env({ TURNSTILE_SECRET: "b" })), "b");
  assert.equal(turnstileSecret(env()), undefined);
});

test("verifyTurnstile posts the token and reads success", async () => {
  const f = fakeFetch((_url, init) => Response.json({ success: (init.body as FormData).get("response") === "good" }));
  try {
    assert.equal(await verifyTurnstile("sec", "good", "1.2.3.4"), true);
    assert.equal(await verifyTurnstile("sec", "bad", null), false);
    assert.equal(f.calls[0].url, "https://challenges.cloudflare.com/turnstile/v0/siteverify");
    assert.equal((f.calls[0].body as FormData).get("remoteip"), "1.2.3.4");
    assert.equal((f.calls[1].body as FormData).get("remoteip"), null);
  } finally {
    f.restore();
  }
});

test("newToken / isToken", () => {
  const t = newToken();
  assert.ok(isToken(t));
  assert.notEqual(newToken(), t);
  assert.ok(!isToken("short"));
  assert.ok(!isToken(`${"a".repeat(42)}!`));
  assert.ok(!isToken(42));
});

test("retentionDays", () => {
  assert.equal(retentionDays(env()), 0);
  assert.equal(retentionDays(env({ NEWSLETTER_RETENTION_DAYS: "30" })), 30);
  assert.equal(retentionDays(env({ NEWSLETTER_RETENTION_DAYS: "-3" })), 0);
  assert.equal(retentionDays(env({ NEWSLETTER_RETENTION_DAYS: "x" })), 0);
});

const basic = (pass: string) => new Request("https://example.com/api/newsletter/admin", { headers: { authorization: `Basic ${btoa(`:${pass}`)}` } });

test("requireAdmin: HTTP Basic", async () => {
  assert.equal((await requireAdmin(basic("pw"), env()))!.status, 503);
  const e = env({ NEWSLETTER_ADMIN_PASSWORD: "pw" });
  assert.equal(await requireAdmin(basic("pw"), e), null);
  assert.equal((await requireAdmin(basic("pW"), e))!.status, 401);
  assert.equal((await requireAdmin(basic("pw2"), e))!.status, 401);
  assert.equal((await requireAdmin(new Request("https://example.com/"), e))!.status, 401);
  const junk = new Request("https://example.com/", { headers: { authorization: "Basic !!!" } });
  assert.equal((await requireAdmin(junk, e))!.status, 401);
});

test("requireAdmin: Cloudflare Access takes over when configured", async () => {
  const e = env({ NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud", NEWSLETTER_ADMIN_PASSWORD: "pw" });
  const res = await requireAdmin(basic("pw"), e);
  assert.equal(res!.status, 401);
  assert.deepEqual(await res!.json(), { error: "Cloudflare Access required" });
});
