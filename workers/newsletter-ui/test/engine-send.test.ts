// The batch consumer: idempotence, last-moment checks, failures.
import { test } from "node:test";
import assert from "node:assert/strict";
import { suppress } from "../src/suppress.ts";
import {
  addCampaign, batchState, campaignRow, engineEnv, fakeMailer, perPerson, pump, runJob, sendRows, started, subId, truth,
} from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const ageClaims = (env: EngineEnv) =>
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaign_sends SET claimed_at = '2000-01-01T00:00:00.000Z' WHERE status = 'claimed'").run();

test("the same batch delivered twice sends one message per person", async () => {
  const { env, id, jobs } = await started(10);
  assert.equal(jobs.length, 1);
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  await runJob(env, jobs[0], mail.open);
  await Promise.all([runJob(env, jobs[0], mail.open), runJob(env, jobs[0], mail.open)]);
  assert.equal(mail.sent.length, 10);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.deepEqual(batchState(env, id), ["done"]);
  assert.equal(mail.stats.opens, 1, "a duplicate of a done range opens no session");
});

test("a crash mid-batch, then redelivery: only the in-flight row ends uncertain, nobody doubled", async () => {
  const { env, id, jobs } = await started(10);
  const mail = fakeMailer((_m, i) => {
    if (i === 4) throw new Error("isolate died");
    return { ok: true };
  });
  await assert.rejects(runJob(env, jobs[0], mail.open), /isolate died/);
  assert.equal(mail.sent.length, 4);
  assert.equal(mail.stats.closes, 1, "the session is closed on the way out");
  const after = truth(env.NEWSLETTER_DB, id);
  assert.deepEqual(after, { sent: 4, claimed: 1 }, "the in-flight row keeps its claim; nobody after it was claimed");
  const used = env.NEWSLETTER_DB.sqlite.prepare("SELECT SUM(used) AS n FROM send_rate").get() as { n: number };
  assert.equal(used.n, 5, "the crashed message counts against the rate: it may have gone out");
  // An immediate redelivery sends the five never-tried rows and leaves the
  // in-flight one to its owner until the claim goes stale.
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 9);
  assert.deepEqual(batchState(env, id), ["queued"]);
  ageClaims(env);
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 9, "a stale claim is never sent again by the engine");
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 9, uncertain: 1 });
  const row = sendRows(env.NEWSLETTER_DB, id)[4];
  assert.deepEqual([row.subscriber_id, row.status, row.attempts], [subId(5), "uncertain", 1]);
  assert.match(String(row.error), /^no result: the consumer stopped/);
  assert.deepEqual(batchState(env, id), ["done"]);
});

test("a fresh claim is left alone; a stale one becomes uncertain and is not sent", async () => {
  const { env, id, jobs } = await started(3);
  const ins = env.NEWSLETTER_DB.sqlite.prepare(
    "INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'claimed', 1, ?)",
  );
  ins.run(id, subId(1), new Date().toISOString());
  ins.run(id, subId(2), "2000-01-01T00:00:00.000Z");
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual(mail.sent.map((m) => m.to), [`${subId(3)}@example.com`]);
  assert.deepEqual(batchState(env, id), ["queued"], "the busy row keeps the range open");
  const rows = sendRows(env.NEWSLETTER_DB, id);
  assert.deepEqual([rows[0].status, rows[1].status, rows[1].attempts], ["claimed", "uncertain", 1], "no attempt is counted: nothing was tried");
});

test("a stale claim becomes uncertain whatever its attempts; a retry row out of attempts ends failed", async () => {
  const { env, id, jobs } = await started(2);
  const ins = env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, ?, 3, ?)");
  ins.run(id, subId(1), "claimed", "2000-01-01T00:00:00.000Z");
  ins.run(id, subId(2), "retry", "2000-01-01T00:00:00.000Z"); // e.g. the attempt limit was lowered meanwhile
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 0);
  assert.equal(mail.stats.opens, 0);
  const [a, b] = sendRows(env.NEWSLETTER_DB, id);
  assert.equal(a.status, "uncertain");
  assert.equal(b.status, "failed");
  assert.match(String(b.error), /no result after 3 attempts/);
  assert.deepEqual(batchState(env, id), ["done"]);
});

test("unsubscribed between scheduling and sending: no message", async () => {
  const { env, jobs } = await started(3);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(2));
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual([...perPerson(mail.sent).keys()], [`${subId(1)}@example.com`, `${subId(3)}@example.com`]);
});

test("unsubscribed, erased or suppressed mid-batch: their message is not sent", async () => {
  const { env, id, jobs } = await started(5);
  const db = env.NEWSLETTER_DB.sqlite;
  const mail = fakeMailer(async (_m, i) => {
    if (i === 0) {
      db.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(2));
      db.prepare("DELETE FROM subscribers WHERE id = ?").run(subId(3));
      await suppress(env as never, { email: `${subId(4)}@example.com`, reason: "complaint", source: "test", keepEmail: true });
    }
    return { ok: true };
  });
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual([...perPerson(mail.sent).keys()], [`${subId(1)}@example.com`, `${subId(5)}@example.com`]);
  const skipped = sendRows(env.NEWSLETTER_DB, id).filter((r) => r.status === "skipped");
  assert.deepEqual(skipped.map((r) => [r.subscriber_id, r.error, r.attempts]), [
    [subId(2), "not_confirmed", 0],
    [subId(3), "subscriber_gone", 0],
    [subId(4), "suppressed", 0],
  ]);
  assert.deepEqual(batchState(env, id), ["done"]);
});

test("suppressions: kept-address and hash-only excluded; fresh consent after an erasure wins; hard is absolute", async () => {
  const env = await engineEnv(5);
  const db = env.NEWSLETTER_DB.sqlite;
  const mail = (i: number) => `${subId(i)}@example.com`;
  await suppress(env as never, { email: mail(1), reason: "hard", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: mail(2), reason: "manual", source: "erasure", keepEmail: false });
  await suppress(env as never, { email: mail(3), reason: "manual", source: "erasure", keepEmail: false });
  await suppress(env as never, { email: mail(4), reason: "hard", source: "smtp", keepEmail: false });
  // 3 and 4 signed up again after their suppression: fresh consent.
  db.prepare("UPDATE subscribers SET consent_at = '2099-01-01T00:00:00.000Z' WHERE id IN (?, ?)").run(subId(3), subId(4));
  const id = addCampaign(env.NEWSLETTER_DB);
  const m = fakeMailer();
  await pump(env, { open: m.open });
  assert.deepEqual([...perPerson(m.sent).keys()], [mail(3), mail(5)]);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).total_count, 4, "total counts what SQL can see: the kept-address suppression only");
});

test("a fresh-consent person passes the last check too (hash-only manual suppression older than consent)", async () => {
  const { env, jobs } = await started(2);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET consent_at = '2099-01-01T00:00:00.000Z' WHERE id = ?").run(subId(2));
  const m = fakeMailer(async (_x, i) => {
    if (i === 0) await suppress(env as never, { email: `${subId(2)}@example.com`, reason: "manual", source: "erasure", keepEmail: false });
    return { ok: true };
  });
  await runJob(env, jobs[0], m.open);
  assert.equal(m.sent.length, 2);
});

test("a range where nobody is left: closed without opening a session", async () => {
  const { env, id, jobs } = await started(3);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed'").run();
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.stats.opens, 0);
  assert.deepEqual(batchState(env, id), ["done"]);
});

test("the campaign deleted mid-batch: the batch stops at the next message", async () => {
  const { env, id, jobs } = await started(3);
  const mail = fakeMailer((_m, i) => {
    if (i === 0) env.NEWSLETTER_DB.sqlite.prepare("DELETE FROM campaigns WHERE id = ?").run(id);
    return { ok: true };
  });
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 1);
  assert.deepEqual(batchState(env, id), ["done"]);
});

test("a crash on the last message, or with the database gone too, still rethrows the crash", async () => {
  const { env, id, jobs } = await started(2);
  const crash = fakeMailer((_m, i) => {
    if (i === 1) throw new Error("socket reset");
    return { ok: true };
  });
  await assert.rejects(runJob(env, jobs[0], crash.open), /socket reset/);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 1, claimed: 1 });

  const again = await started(2);
  again.env.NEWSLETTER_DB.state.failOn = /SET status = 'retry', attempts = attempts - 1/;
  const first = fakeMailer(() => {
    throw new Error("socket reset");
  });
  await assert.rejects(runJob(again.env, again.jobs[0], first.open), /socket reset/);
  again.env.NEWSLETTER_DB.state.failOn = null;
  assert.deepEqual(truth(again.env.NEWSLETTER_DB, again.id), { claimed: 1 }, "the in-flight row only; the next one was never claimed");
});

// failing wraps the fake database's prepare: `fail(sql, n)` (n = how many
// statements containing the same marker came before) decides what throws.
function failing(env: EngineEnv, fail: (sql: string) => boolean) {
  const db = env.NEWSLETTER_DB;
  const prepare = db.prepare.bind(db);
  db.prepare = ((sql: string) => {
    if (fail(sql)) throw new Error("D1_ERROR: forced");
    return prepare(sql);
  }) as typeof db.prepare;
}

test("the database fails before the send: that claim is handed back, the error rethrown", async () => {
  const { env, id, jobs } = await started(2);
  const mail = fakeMailer();
  let claims = 0;
  failing(env, (sql) => sql.includes("INSERT INTO campaign_sends") && ++claims === 2);
  await assert.rejects(runJob(env, jobs[0], mail.open), /forced/);
  assert.equal(mail.sent.length, 1);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 1 }, "nothing claimed is left behind");

  // Writing the skip fails, and so does handing the claim back: the claim
  // stays (the stale timeout takes over) and the first error is the one thrown.
  const again = await started(2);
  let records = 0;
  failing(again.env, (sql) => (sql.includes("SET status = ?1, error = ?2") && ++records === 2) || sql.includes("attempts = attempts - 1, error = NULL"));
  const m = fakeMailer((_x, i) => {
    if (i === 0) again.env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(2));
    return { ok: true };
  });
  await assert.rejects(runJob(again.env, again.jobs[0], m.open), /forced/);
  assert.deepEqual(truth(again.env.NEWSLETTER_DB, again.id), { sent: 1, claimed: 1 });
});

test("a row another consumer claimed between the read and the claim is left to it", async () => {
  const { env, id, jobs } = await started(3);
  const mail = fakeMailer((_m, i) => {
    if (i === 0) {
      env.NEWSLETTER_DB.sqlite
        .prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'claimed', 1, ?)")
        .run(id, subId(2), new Date().toISOString());
    }
    return { ok: true };
  });
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual(mail.sent.map((m) => m.to), [`${subId(1)}@example.com`, `${subId(3)}@example.com`]);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 2, claimed: 1 });
});
