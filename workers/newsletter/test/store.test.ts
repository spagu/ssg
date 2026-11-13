// D1 access, against a recording fake. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { confirm, findBy, markConfirmSent, purgeExpired, subscribe, unsubscribe } from "../functions/api/newsletter/_store.ts";
import type { Signup } from "../functions/api/newsletter/_store.ts";
import { fakeDB } from "./fakes.ts";

const signup: Signup = {
  email: "reader@example.com",
  language: "pl",
  sourcePage: "/blog/",
  tags: "news",
  consentText: "I agree",
  ipHash: null,
  uaHash: null,
};
const env = (db: ReturnType<typeof fakeDB>, over: Record<string, unknown> = {}) => ({ NEWSLETTER_DB: db, ...over }) as never;
const row = (over: Record<string, unknown> = {}) => ({
  email: "reader@example.com",
  status: "pending",
  language: "en",
  token: "t".repeat(43),
  confirm_sent_at: null,
  ...over,
});

test("a new address: inserted pending (double opt-in) or confirmed", async () => {
  for (const doi of [true, false]) {
    const db = fakeDB();
    const r = await subscribe(env(db), signup, doi);
    assert.equal(r.sendConfirmation, doi);
    assert.equal(r.subscriber.status, doi ? "pending" : "confirmed");
    assert.equal(r.subscriber.token.length, 43);
    const insert = db.calls[1];
    assert.match(insert.sql, /^INSERT INTO subscribers .* ON CONFLICT\(email\) DO NOTHING$/);
    assert.equal(insert.args[2], r.subscriber.status);
    assert.equal(insert.args[11], doi ? null : insert.args[7]);
  }
});

test("a confirmed address is left alone", async () => {
  const db = fakeDB({ first: row({ status: "confirmed" }) });
  const r = await subscribe(env(db), signup, true);
  assert.deepEqual(r, { sendConfirmation: false, subscriber: row({ status: "confirmed" }) });
  assert.equal(db.calls.length, 1);
});

test("pending or unsubscribed: a fresh sign-up, same token, throttled re-send", async () => {
  const recent = new Date(Date.now() - 60_000).toISOString();
  const old = new Date(Date.now() - 60 * 60_000).toISOString();
  const cases: [Record<string, unknown>, boolean, boolean][] = [
    [{ status: "pending", confirm_sent_at: recent }, true, false], // within 15 minutes
    [{ status: "pending", confirm_sent_at: old }, true, true],
    [{ status: "pending" }, true, true], // never sent
    [{ status: "unsubscribed", confirm_sent_at: recent }, true, true],
    [{ status: "unsubscribed" }, false, false], // single opt-in
  ];
  for (const [existing, doi, send] of cases) {
    const db = fakeDB({ first: row(existing) });
    const r = await subscribe(env(db), signup, doi);
    assert.equal(r.sendConfirmation, send, JSON.stringify(existing));
    assert.equal(r.subscriber.token, "t".repeat(43));
    assert.equal(r.subscriber.status, doi ? "pending" : "confirmed");
    assert.equal(r.subscriber.language, "pl");
    assert.match(db.calls[1].sql, /^UPDATE subscribers SET status = \?/);
    assert.equal(db.calls[1].args.at(-1), "reader@example.com");
  }
});

test("findBy looks up by column", async () => {
  const db = fakeDB({ first: row() });
  assert.deepEqual(await findBy(env(db), "token", "abc"), row());
  assert.match(db.calls[0].sql, /WHERE token = \?$/);
  assert.deepEqual(db.calls[0].args, ["abc"]);
});

test("confirm reports whether a row changed", async () => {
  assert.equal(await confirm(env(fakeDB({ changes: 1 })), "abc"), true);
  assert.equal(await confirm(env(fakeDB({ changes: 0 })), "abc"), false);
  assert.equal(await confirm(env(fakeDB()), "abc"), false);
});

test("unsubscribe and markConfirmSent write what they say", async () => {
  const db = fakeDB();
  await unsubscribe(env(db), "abc");
  await markConfirmSent(env(db), "reader@example.com");
  assert.match(db.calls[0].sql, /SET status = 'unsubscribed'.*WHERE token = \? AND status != 'unsubscribed'$/);
  assert.equal(db.calls[0].args[2], "abc");
  assert.equal(db.calls[1].sql, "UPDATE subscribers SET confirm_sent_at = ? WHERE email = ?");
  assert.equal(db.calls[1].args[1], "reader@example.com");
});

test("purgeExpired: off by default, at most hourly unless forced", async () => {
  const off = fakeDB();
  assert.equal(await purgeExpired(env(off, {}), true), 0);
  assert.equal(off.calls.length, 0);

  const first = fakeDB();
  assert.equal(await purgeExpired(env(first, { NEWSLETTER_RETENTION_DAYS: "30" })), 0); // no `changes` reported
  assert.equal(first.calls.length, 1);
  const cutoff = Date.parse(first.calls[0].args[0] as string);
  assert.ok(Math.abs(Date.now() - 30 * 86400000 - cutoff) < 5000);

  const throttled = fakeDB({ changes: 4 });
  assert.equal(await purgeExpired(env(throttled, { NEWSLETTER_RETENTION_DAYS: "30" })), 0);
  assert.equal(throttled.calls.length, 0);
  assert.equal(await purgeExpired(env(throttled, { NEWSLETTER_RETENTION_DAYS: "30" }), true), 4);
});
