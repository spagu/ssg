// Confirmation mail, the single re-send: the 15-minute throttle, status and
// suppression checks, the shared rate budget, the language of the mail.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CONFIRM_STRINGS } from "../src/confirm-strings.ts";
import { confirmLanguage } from "../src/confirm-mail.ts";
import { takeBudget } from "../src/engine-rate.ts";
import { suppress } from "../src/suppress.ts";
import { NOW, add, confirmEnv, mailbox, post, sentAt } from "./confirm-fixtures.ts";

test("single re-send: the 15-minute throttle, status and suppression checks, failures", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv();
  add(env.NEWSLETTER_DB, "s1", { language: "pl" });
  add(env.NEWSLETTER_DB, "s2", { status: "confirmed" });
  add(env.NEWSLETTER_DB, "s3");
  add(env.NEWSLETTER_DB, "s4", { consentAt: "2026-06-01T00:00:00.000Z" });
  add(env.NEWSLETTER_DB, "s5", { sentAt: "garbage" });
  await suppress(env as never, { email: "s3@example.com", reason: "hard", source: "test", keepEmail: true });
  t.mock.timers.setTime(Date.parse("2026-01-01T00:00:00.000Z"));
  await suppress(env as never, { email: "s4@example.com", reason: "manual", source: "erasure", keepEmail: false });
  t.mock.timers.setTime(NOW);
  const box = mailbox({ "s5@example.com": 500 });
  try {
    const ok = await post(env, "/s1/confirmation");
    assert.deepEqual([ok.status, ok.body], [200, { ok: true, transport: "resend" }]);
    assert.equal(box.mails[0].subject, CONFIRM_STRINGS.pl.mailSubject);
    assert.equal(sentAt(env, "s1"), new Date(NOW).toISOString());
    t.mock.timers.tick(60_000);
    const throttled = await post(env, "/s1/confirmation");
    assert.deepEqual([throttled.status, throttled.body.error, throttled.headers.get("retry-after")], [429, "confirm_throttled", "840"]);
    t.mock.timers.tick(14 * 60_000);
    assert.equal((await post(env, "/s1/confirmation")).status, 200, "after 15 minutes it may go again");
    assert.deepEqual([(await post(env, "/nobody/confirmation")).status], [404]);
    const confirmed = await post(env, "/s2/confirmation");
    assert.deepEqual([confirmed.status, confirmed.body.error], [409, "not_pending"]);
    const suppressed = await post(env, "/s3/confirmation");
    assert.deepEqual([suppressed.status, suppressed.body.error, suppressed.body.detail], [409, "suppressed", "the address is suppressed (hard)"]);
    assert.equal((await post(env, "/s4/confirmation")).status, 200, "fresh consent after an erasure wins, as in the engine");
    const failed = await post(env, "/s5/confirmation");
    assert.deepEqual([failed.status, failed.body.error, failed.body.temporary], [502, "send_failed", true]);
    assert.equal(sentAt(env, "s5"), "garbage", "an unreadable timestamp does not throttle, and a failure does not stamp");
    assert.equal(box.mails.length, 3);
  } finally {
    box.restore();
  }
});

test("single re-send: takes one message of the shared budget", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv({ NEWSLETTER_SEND_RATE: "2" });
  add(env.NEWSLETTER_DB, "r1");
  add(env.NEWSLETTER_DB, "r2");
  await takeBudget(env as never, 1, 2);
  const box = mailbox();
  try {
    assert.equal((await post(env, "/r1/confirmation")).status, 200);
    const out = await post(env, "/r2/confirmation");
    assert.deepEqual([out.status, out.body.error], [429, "rate_limited"]);
    assert.equal(sentAt(env, "r2"), null);
  } finally {
    box.restore();
  }
});

test("confirmLanguage: primary subtag of a shipped language, else en", () => {
  assert.deepEqual(["pl", "PL-pl", "hi", "fr-CA", "pt", "", null].map((l) => confirmLanguage(l)), ["pl", "pl", "hi", "fr", "en", "en", "en"]);
});
