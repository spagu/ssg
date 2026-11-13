// Confirmation mail for pending subscribers, the batch route: bounds, cursor,
// suppressions, the engine's shared rate budget, confirm_sent_at only on
// success — on real SQL.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CONFIRM_STRINGS } from "../src/confirm-strings.ts";
import { confirmBatch } from "../src/confirm.ts";
import { takeBudget } from "../src/engine-rate.ts";
import { suppress } from "../src/suppress.ts";
import { NOW, add, confirmEnv, mailbox, post, sentAt, used } from "./confirm-fixtures.ts";

test("batch: bounded per call, cursor to the end, the template's mail, only pending + never mailed + not suppressed", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv();
  const d1 = env.NEWSLETTER_DB;
  for (let i = 1; i <= 23; i++) add(d1, `p${String(i).padStart(2, "0")}`, { language: ["pl", "de-AT", "pt-BR", null][i % 4] });
  add(d1, "q-confirmed", { status: "confirmed" });
  add(d1, "q-unsub", { status: "unsubscribed" });
  add(d1, "q-mailed", { sentAt: "2026-11-01T00:00:00.000Z" });
  add(d1, "p05b"); // between p05 and p06
  await suppress(env as never, { email: "p03@example.com", reason: "hard", source: "test", keepEmail: true });
  await suppress(env as never, { email: "p05b@example.com", reason: "complaint", source: "test", keepEmail: false });
  const box = mailbox();
  try {
    const first = await post(env, "/confirmations");
    assert.equal(first.status, 200);
    assert.deepEqual({ ...first.body, failures: undefined }, { sent: 9, failed: 0, skipped: 1, failures: undefined, remaining: 13, next: "p10" });
    // p03 (address kept) is excluded in SQL; p05b (hash-only) is read and skipped.
    const pages = [first.body];
    for (let next = first.body.next; next; ) {
      const r = await post(env, "/confirmations", { after: next });
      pages.push(r.body);
      next = r.body.next;
    }
    assert.deepEqual(pages.map((p) => [p.sent, p.remaining]), [[9, 13], [10, 3], [3, 0]]);
    assert.equal(pages.at(-1).next, null);
    assert.equal(box.mails.length, 22);
    const to = box.mails.map((m) => m.to);
    for (const never of ["p03", "p05b", "q-confirmed", "q-unsub", "q-mailed"]) assert.ok(!to.includes(`${never}@example.com`), never);
    const p07 = box.mails.find((m) => m.to === "p07@example.com") as { subject: string; text: string; from: string };
    assert.equal(p07.subject, CONFIRM_STRINGS.en.mailSubject, "null language → en");
    assert.equal(p07.text, CONFIRM_STRINGS.en.mailBody.replace("{url}", `https://example.com/api/newsletter/confirm?token=${"tok-p07".padEnd(43, "x")}`));
    assert.equal(p07.from, "news@example.com");
    assert.equal(box.mails.find((m) => m.to === "p01@example.com")?.subject, CONFIRM_STRINGS.de.mailSubject, "de-AT → de");
    assert.equal(box.mails.find((m) => m.to === "p02@example.com")?.subject, CONFIRM_STRINGS.en.mailSubject, "pt-BR → en");
    assert.equal(box.mails.find((m) => m.to === "p04@example.com")?.subject, CONFIRM_STRINGS.pl.mailSubject);
    assert.match(box.mails.find((m) => m.to === "p04@example.com")?.text ?? "", /^Ktoś/);
    assert.equal(sentAt(env, "p01"), new Date(NOW).toISOString());
    assert.equal(sentAt(env, "p03"), null);
    assert.equal(sentAt(env, "q-mailed"), "2026-11-01T00:00:00.000Z");
    assert.equal(used(env), 22, "every mail took from the engine's budget");
    const done = await post(env, "/confirmations", {});
    // Only the hash-only suppressed row is left; it is read and skipped again.
    assert.deepEqual(done.body, { sent: 0, failed: 0, skipped: 1, failures: [], remaining: 0, next: null });
  } finally {
    box.restore();
  }
});

test("batch: a refused mail leaves confirm_sent_at unset, is reported, and does not block the cursor", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv({ NEWSLETTER_CONFIRM_BATCH: "3" });
  for (const id of ["a1", "a2", "a3", "a4"]) add(env.NEWSLETTER_DB, id);
  const box = mailbox({ "a2@example.com": 422, "a3@example.com": 503 });
  try {
    const r = await post(env, "/confirmations");
    assert.equal(r.body.sent, 1);
    assert.equal(r.body.failed, 2);
    assert.deepEqual(r.body.failures, [
      { id: "a2", detail: "resend returned 422", temporary: false },
      { id: "a3", detail: "resend returned 503", temporary: true },
    ]);
    assert.deepEqual([r.body.remaining, r.body.next], [1, "a3"]);
    assert.deepEqual(["a1", "a2", "a3"].map((id) => sentAt(env, id) !== null), [true, false, false]);
    const rest = await post(env, "/confirmations", { after: r.body.next });
    assert.deepEqual([rest.body.sent, rest.body.next], [1, null]);
    const again = await post(env, "/confirmations", {});
    assert.deepEqual([again.body.failed, again.body.remaining], [2, 0], "a fresh pass retries the failed ones");
  } finally {
    box.restore();
  }
});

test("batch: shares the engine's per-minute budget; out of budget = 429; unused budget is given back", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv({ NEWSLETTER_SEND_RATE: "5" });
  for (let i = 1; i <= 8; i++) add(env.NEWSLETTER_DB, `b${i}`);
  await takeBudget(env as never, 3, 5); // a campaign batch already took 3 this minute
  const box = mailbox();
  try {
    const r = await post(env, "/confirmations");
    assert.deepEqual([r.body.sent, r.body.next, r.body.remaining], [2, "b2", 6]);
    assert.equal(used(env), 5);
    const out = await post(env, "/confirmations", { after: "b2" });
    assert.equal(out.status, 429);
    assert.equal(out.body.error, "rate_limited");
    assert.equal(out.headers.get("retry-after"), "50");
    t.mock.timers.tick(60_000);
    const later = await post(env, "/confirmations", { after: "b2" });
    assert.deepEqual([later.body.sent, later.body.next], [5, "b7"], "batch size is capped by the rate");
  } finally {
    box.restore();
  }
  const smtp = await confirmEnv({ NEWSLETTER_RESEND_API_KEY: undefined, NEWSLETTER_SMTP_HOST: "smtp.example.com" });
  add(smtp.NEWSLETTER_DB, "c1");
  const failed = await post(smtp, "/confirmations");
  assert.deepEqual([failed.status, failed.body.error], [502, "send_failed"]);
  assert.equal(used(smtp), 0, "a transport that never opened gives the budget back");
  assert.equal(sentAt(smtp, "c1"), null);
});

test("batch: input and configuration refusals", async () => {
  const env = await confirmEnv();
  add(env.NEWSLETTER_DB, "x1");
  for (const body of [{ after: 5 }, { after: "x".repeat(201) }]) {
    const r = await post(env, "/confirmations", body);
    assert.deepEqual([r.status, r.body.error], [400, "invalid_input"]);
  }
  const configs: [Record<string, unknown>, string][] = [
    [{ NEWSLETTER_SITE_URL: "http://example.com" }, "no_site_url"],
    [{ NEWSLETTER_MAIL_FROM: undefined }, "no_sender"],
    [{ NEWSLETTER_RESEND_API_KEY: undefined }, "no_transport"],
  ];
  for (const [o, error] of configs) {
    const e = await confirmEnv(o);
    add(e.NEWSLETTER_DB, "x1");
    for (const path of ["/confirmations", "/x1/confirmation"]) {
      const r = await post(e, path);
      assert.deepEqual([r.status, r.body.error], [503, error], `${path} ${error}`);
    }
    assert.equal(sentAt(e, "x1"), null);
  }
});

test("batch: nothing pending; a transport that throws or fails to close", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const env = await confirmEnv();
  const empty = await post(env, "/confirmations");
  assert.deepEqual(empty.body, { sent: 0, failed: 0, skipped: 0, failures: [], remaining: 0, next: null });
  for (const id of ["t1", "t2", "t3"]) add(env.NEWSLETTER_DB, id);
  let closed = 0;
  const mailer = (send: (to: string) => Promise<{ ok: true }>) => async () => ({
    ok: true as const,
    mailer: {
      send: (m: { to: string }) => send(m.to),
      close: async () => {
        closed++;
        throw new Error("QUIT refused");
      },
    },
  });
  const warn = t.mock.method(console, "warn", () => {});
  const r = await confirmBatch(env as never, "", mailer(async () => ({ ok: true })) as never);
  assert.equal(((await r.json()) as { sent: number }).sent, 3, "a failed close does not undo mails that left");
  assert.equal(closed, 1);
  assert.match(String(warn.mock.calls[0].arguments[0]), /closing the mailer/);
  env.NEWSLETTER_DB.sqlite.exec("UPDATE subscribers SET confirm_sent_at = NULL; DELETE FROM send_rate");
  const crash = mailer(async (to) => {
    if (to === "t2@example.com") throw new Error("socket gone");
    return { ok: true };
  });
  await assert.rejects(confirmBatch(env as never, "", crash as never), /socket gone/);
  assert.equal(used(env), 2, "the message that crashed counts; the one never tried is given back");
  assert.deepEqual(["t1", "t2", "t3"].map((id) => sentAt(env, id) !== null), [true, false, false]);
});
