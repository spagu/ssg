// A consumer that dies mid-send never causes an automatic second copy: the
// message in flight ends `uncertain`, everyone else is sent exactly once, and
// the campaign finishes with the uncertain row counted on its own.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { planTick } from "../src/engine-plan.ts";
import type { MailMessage, MailResult } from "../src/mail/_transport.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { batchState, campaignRow, fakeMailer, perPerson, pump, runJob, sendRows, started, subId, truth } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const NOW = Date.parse("2026-11-13T10:00:00.000Z");
const STALE = 31 * 60_000; // past the default 30-minute stale timeout

export const progress = async (env: EngineEnv, id: string) =>
  (await handleFetch(req(`/newsletter-admin/api/campaigns/${id}/progress`, { auth: PASSWORD }), env as never, fakeCtx() as never)).json();

// provider: what the outside world received, whatever the worker recorded.
// `answer` may push to `received` and then throw or hang, like a provider
// that accepted a message just before the worker died.
function provider(answer: (m: MailMessage, i: number, received: MailMessage[]) => MailResult | Promise<MailResult>) {
  const received: MailMessage[] = [];
  const mail = fakeMailer((m, i) => answer(m, i, received));
  return { received, mail, all: () => [...received, ...mail.sent] };
}

test("the provider accepted message k, then the consumer died: after the stale timeout nobody gets a second copy", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const { env, id, jobs } = await started(10);
  env.NEWSLETTER_QUEUE.sent.push(...jobs.map((body) => ({ body })));
  const p = provider((m, i, received) => {
    if (i === 3) {
      received.push(m); // accepted …
      throw new Error("isolate killed"); // … and the worker never heard back
    }
    return { ok: true };
  });
  await pump(env, { open: p.mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sending", "the in-flight claim is young: the campaign waits");
  t.mock.timers.tick(STALE);
  await pump(env, { open: p.mail.open });

  const counts = perPerson(p.all());
  assert.equal(counts.size, 10);
  assert.ok([...counts.values()].every((n) => n === 1), "every recipient exactly once, the in-flight one included");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 9, uncertain: 1 });
  assert.equal(sendRows(env.NEWSLETTER_DB, id)[3].status, "uncertain");
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.deepEqual([c.status, c.sent_count, c.failed_count], ["sent", 9, 0], "uncertain is neither sent nor failed");
  const pr = await progress(env, id);
  assert.deepEqual([pr.total, pr.sent, pr.failed, pr.uncertain, pr.claimed, pr.remaining], [10, 9, 0, 1, 0, 0]);
});

test("a consumer killed without any cleanup mid-batch leaves exactly one row ambiguous", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const { env, id, jobs } = await started(10);
  let reached!: () => void;
  const inFlight = new Promise<void>((r) => (reached = r));
  const dead = provider((m, i, received) => {
    if (i < 3) return { ok: true };
    received.push(m);
    reached();
    return new Promise<MailResult>(() => {}); // never answers: no catch, no finally
  });
  void runJob(env, jobs[0], dead.mail.open);
  await inFlight;
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 3, claimed: 1 }, "only the row being sent holds a claim");

  // The queue redelivers the job to a healthy consumer.
  const healthy = provider(() => ({ ok: true }));
  await runJob(env, jobs[0], healthy.mail.open);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 9, claimed: 1 });
  assert.deepEqual(batchState(env, id), ["queued"]);
  t.mock.timers.tick(STALE);
  await planTick(env as never); // re-queues the range left queued
  const [again] = env.NEWSLETTER_QUEUE.sent.splice(0).map((m) => m.body);
  await runJob(env, again, healthy.mail.open);
  await planTick(env as never);

  const counts = perPerson([...dead.all(), ...healthy.all()]);
  assert.equal(counts.size, 10);
  assert.ok([...counts.values()].every((n) => n === 1));
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 9, uncertain: 1 });
  assert.equal(sendRows(env.NEWSLETTER_DB, id)[3].subscriber_id, subId(4));
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("a retry row the segment no longer selects keeps its range open, then closes as skipped", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: NOW });
  const { env, id, jobs } = await started(3);
  // A temporary failure left subscriber 2 in `retry`; then they unsubscribed.
  env.NEWSLETTER_DB.sqlite
    .prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'retry', 1, ?)")
    .run(id, subId(2), new Date(NOW).toISOString());
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(2));
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 2);
  const row = env.NEWSLETTER_DB.sqlite.prepare("SELECT status, retry_only FROM send_batches WHERE campaign_id = ?").get(id) as Record<string, unknown>;
  assert.deepEqual({ ...row }, { status: "queued", retry_only: 1 }, "not done while a row waits");
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sending", "the campaign does not finish over a pending row");

  t.mock.timers.tick(STALE);
  await pump(env, { open: mail.open });
  assert.equal(mail.sent.length, 2, "the last check skips the person who left");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 2, skipped: 1 });
  assert.equal(sendRows(env.NEWSLETTER_DB, id)[1].error, "not_confirmed");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("engine messages carry the idempotency key campaign:subscriber (Resend: the Idempotency-Key header)", async () => {
  const { env, id, jobs } = await started(2);
  const keys: (string | null)[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (_url: string, init: RequestInit) => {
    keys.push(new Headers(init.headers).get("idempotency-key"));
    return new Response("{}", { status: 200 });
  }) as typeof fetch;
  try {
    await runJob(env, jobs[0], undefined);
  } finally {
    globalThis.fetch = original;
  }
  assert.deepEqual(keys, [`${id}:${subId(1)}`, `${id}:${subId(2)}`]);
});

test("a test send and a confirmation mail carry no idempotency key (each is a new message)", async () => {
  const { call, mailEnv } = await import("./campaign-fixtures.ts");
  const confirm = await import("./confirm-fixtures.ts");
  const keys: (string | null)[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (_url: string, init: RequestInit) => {
    keys.push(new Headers(init.headers).get("idempotency-key"));
    return new Response("{}", { status: 200 });
  }) as typeof fetch;
  try {
    const env = mailEnv();
    const c = (await call(env, "POST", "", { subject: "Hi", body: "Body" })).body;
    assert.equal((await call(env, "POST", `/${c.id}/test`, { to: "me@example.com" })).status, 200);
    const cenv = await confirm.confirmEnv();
    confirm.add(cenv.NEWSLETTER_DB, "p1");
    assert.equal((await confirm.post(cenv, "/p1/confirmation")).status, 200);
  } finally {
    globalThis.fetch = original;
  }
  assert.deepEqual(keys, [null, null]);
});
