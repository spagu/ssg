// The admin's side of uncertain sends: list, re-send (one more attempt through
// the normal path), dismiss — on a finished, a running and a paused campaign.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transition } from "../src/campaign-state.ts";
import { suppress } from "../src/suppress.ts";
import { call } from "./campaign-fixtures.ts";
import { batchState, campaignRow, fakeMailer, perPerson, pump, runJob, sendJobs, sendRows, started, subId, truth } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const NOW = Date.parse("2026-11-13T10:00:00.000Z");
const addr = (i: number) => `${subId(i)}@example.com`;

// finished: n recipients all sent, then the listed ones marked uncertain as a
// crash would have left them (claimed at `claimedAt`).
async function finished(n: number, uncertain: number[], claimedAt = new Date(NOW).toISOString(), overrides: Record<string, unknown> = {}) {
  const s = await started(n, overrides);
  s.env.NEWSLETTER_QUEUE.sent.push(...s.jobs.map((body) => ({ body })));
  await pump(s.env, { open: fakeMailer().open });
  const mark = s.env.NEWSLETTER_DB.sqlite.prepare(
    "UPDATE campaign_sends SET status = 'uncertain', sent_at = NULL, claimed_at = ? WHERE campaign_id = ? AND subscriber_id = ?",
  );
  for (const i of uncertain) mark.run(claimedAt, s.id, subId(i));
  return s;
}

const usedBudget = (env: EngineEnv) => Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COALESCE(SUM(used), 0) AS n FROM send_rate").get() as { n: number }).n);

test("list: who, since when, whether a re-send is safe; cursor and refusals", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const { env, id } = await finished(5, [1, 3, 4]);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaign_sends SET claimed_at = '2026-11-10T00:00:00.000Z' WHERE subscriber_id = ?").run(subId(4));
  env.NEWSLETTER_DB.sqlite.prepare("DELETE FROM subscribers WHERE id = ?").run(subId(3));
  const all = await call(env, "GET", `/${id}/uncertain`);
  assert.equal(all.status, 200);
  assert.deepEqual(all.body, {
    items: [
      { subscriberId: subId(1), email: addr(1), claimedAt: new Date(NOW).toISOString(), safeResend: true },
      { subscriberId: subId(3), email: null, claimedAt: new Date(NOW).toISOString(), safeResend: true },
      { subscriberId: subId(4), email: addr(4), claimedAt: "2026-11-10T00:00:00.000Z", safeResend: false },
    ],
    nextCursor: null,
    transport: "resend",
    safeResend: true,
  });
  const p1 = await call(env, "GET", `/${id}/uncertain?limit=2`);
  assert.deepEqual([p1.body.items.length, p1.body.nextCursor], [2, subId(3)]);
  const p2 = await call(env, "GET", `/${id}/uncertain?limit=2&cursor=${subId(3)}`);
  assert.deepEqual([p2.body.items.map((i: { subscriberId: string }) => i.subscriberId), p2.body.nextCursor], [[subId(4)], null]);
  for (const bad of ["0", "201", "x", "1.5"]) assert.equal((await call(env, "GET", `/${id}/uncertain?limit=${bad}`)).body.error, "invalid_limit");
  assert.equal((await call(env, "GET", "/nope/uncertain")).status, 404);

  const smtp = await finished(1, [1], undefined, { NEWSLETTER_RESEND_API_KEY: undefined, NEWSLETTER_SMTP_HOST: "smtp.example.com" });
  const s = (await call(smtp.env, "GET", `/${smtp.id}/uncertain`)).body;
  assert.deepEqual([s.transport, s.safeResend, s.items[0].safeResend], ["smtp", false, false]);
});

test("re-send on a finished campaign: exactly one more attempt; unsubscribe, suppression and erasure still win", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const { env, id } = await finished(10, [2, 5, 8, 9]);
  const before = campaignRow(env.NEWSLETTER_DB, id);
  const budget = usedBudget(env);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(5));
  await suppress(env as never, { email: addr(8), reason: "hard", source: "smtp", keepEmail: true });
  env.NEWSLETTER_DB.sqlite.prepare("DELETE FROM subscribers WHERE id = ?").run(subId(9));
  const r = await call(env, "POST", `/${id}/uncertain/resend`);
  assert.deepEqual([r.status, r.body], [200, { resent: 4, ranges: 1, safeResend: true }]);
  assert.deepEqual(sendJobs(env).map((j) => j.after), [""]);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent", "the campaign is not reopened");

  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.deepEqual([...perPerson(mail.sent)], [[addr(2), 1]]);
  assert.equal(usedBudget(env), budget + 1, "the re-send used the shared rate budget");
  const rows = sendRows(env.NEWSLETTER_DB, id);
  assert.deepEqual([4, 7, 8].map((k) => [rows[k].status, rows[k].error]), [["skipped", "not_confirmed"], ["skipped", "suppressed"], ["skipped", "subscriber_gone"]]);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 7, skipped: 3 });
  const after = campaignRow(env.NEWSLETTER_DB, id);
  assert.deepEqual([after.status, after.finished_at, after.sent_count], ["sent", before.finished_at, 7], "finished_at keeps the run's end; counters follow");
  assert.deepEqual(batchState(env, id), ["done"]);
  assert.deepEqual((await call(env, "POST", `/${id}/uncertain/resend`, {})).body, { resent: 0, ranges: 0, safeResend: true });
});

test("re-send picked ids, dismiss the rest; refusals", async () => {
  const { env, id } = await finished(4, [1, 2, 3]);
  const r = await call(env, "POST", `/${id}/uncertain/resend`, { ids: [subId(1), "not-a-row"] });
  assert.deepEqual(r.body, { resent: 1, ranges: 1, safeResend: true });
  const d = await call(env, "POST", `/${id}/uncertain/dismiss`, { ids: [subId(2)] });
  assert.deepEqual([d.status, d.body], [200, { dismissed: 1 }]);
  assert.deepEqual((await call(env, "POST", `/${id}/uncertain/dismiss`)).body, { dismissed: 1 }, "no body = every uncertain row");
  await pump(env, { open: fakeMailer().open });
  const rows = sendRows(env.NEWSLETTER_DB, id);
  assert.deepEqual(rows.slice(0, 3).map((x) => [x.status, x.error]), [["sent", null], ["skipped", "dismissed"], ["skipped", "dismissed"]]);

  for (const ids of ["x", [], [1], [""], ["x".repeat(201)], Array(1001).fill("x")]) {
    for (const action of ["resend", "dismiss"]) {
      const bad = await call(env, "POST", `/${id}/uncertain/${action}`, { ids });
      assert.deepEqual([bad.status, bad.body.error], [400, "invalid_ids"], `${action} ${JSON.stringify(ids).slice(0, 20)}`);
    }
  }
  for (const action of ["resend", "dismiss"]) assert.equal((await call(env, "POST", `/nope/uncertain/${action}`)).status, 404);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'cancelled' WHERE id = ?").run(id);
  const refused = await call(env, "POST", `/${id}/uncertain/resend`);
  assert.deepEqual([refused.status, refused.body.error], [409, "not_resendable"]);
});

test("re-send while the campaign runs: only the re-sent row, not people who signed up since", async () => {
  const { env, id, jobs } = await started(20);
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaign_sends SET status = 'uncertain' WHERE subscriber_id = ?").run(subId(3));
  env.NEWSLETTER_DB.sqlite
    .prepare(
      `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at)
       VALUES ('sub-00003a', 'late@example.com', 'confirmed', 'en', '', 'yes', '2026-01-01', 'tok-late-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx', '2026-01-01', '2026-01-01')`,
    )
    .run();
  assert.deepEqual((await call(env, "POST", `/${id}/uncertain/resend`)).body.ranges, 1);
  env.NEWSLETTER_QUEUE.sent.push({ body: jobs[1] });
  await pump(env, { open: mail.open });
  const counts = perPerson(mail.sent);
  assert.equal(counts.get(addr(3)), 2, "the original (which the ledger called uncertain) and the one re-send");
  assert.equal(counts.has("late@example.com"), false, "a reopened range mails nobody new");
  assert.equal(counts.size, 20);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("re-send on a paused campaign waits for resume; a lost re-send job is re-queued by the cron", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const paused = await finished(3, [2]);
  paused.env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'paused' WHERE id = ?").run(paused.id);
  assert.deepEqual((await call(paused.env, "POST", `/${paused.id}/uncertain/resend`)).body.ranges, 1);
  assert.deepEqual([sendJobs(paused.env).length, batchState(paused.env, paused.id)], [0, ["parked"]]);
  assert.equal(await transition(paused.env as never, paused.id, "paused", "sending"), true);
  const m1 = fakeMailer();
  await pump(paused.env, { open: m1.open });
  assert.deepEqual([...perPerson(m1.sent).keys()], [addr(2)]);
  assert.equal(campaignRow(paused.env.NEWSLETTER_DB, paused.id).status, "sent");

  const lost = await finished(3, [1, 3]);
  await call(lost.env, "POST", `/${lost.id}/uncertain/resend`);
  lost.env.NEWSLETTER_QUEUE.sent.length = 0; // the job never arrives
  // Meanwhile one row was settled another way: the range finds only the other.
  lost.env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaign_sends SET status = 'skipped' WHERE subscriber_id = ?").run(subId(1));
  const m2 = fakeMailer();
  await pump(lost.env, { open: m2.open });
  assert.equal(m2.sent.length, 0, "not before the stale timeout");
  t.mock.timers.tick(31 * 60_000);
  await pump(lost.env, { open: m2.open });
  assert.deepEqual([...perPerson(m2.sent).keys()], [addr(3)]);
  assert.deepEqual(batchState(lost.env, lost.id), ["done"]);
  // A range that holds no retry row any more closes at once.
  lost.env.NEWSLETTER_DB.sqlite.prepare("UPDATE send_batches SET status = 'queued', updated_at = '2000-01-01T00:00:00.000Z'").run();
  await pump(lost.env, { open: m2.open });
  assert.deepEqual([m2.sent.length, batchState(lost.env, lost.id)], [1, ["done"]]);
});
