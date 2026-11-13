// The cron and queue entry points and their registries.
import { test } from "node:test";
import assert from "node:assert/strict";
import worker from "../src/index.ts";
import { CRON_TASKS, JOB_HANDLERS, handleQueue, handleScheduled } from "../src/jobs.ts";
import { PASSWORD, fakeBatch, fakeCtx, makeEnv, req } from "./fakes.ts";

const controller = { cron: "* * * * *", scheduledTime: 0, noRetry() {} };

test("the default export wires fetch, scheduled and queue", async () => {
  const env = makeEnv();
  const res = await worker.fetch(req("/newsletter-admin/api/health", { auth: PASSWORD }), env as never, fakeCtx() as never);
  assert.equal(res.status, 200);

  const ctx = fakeCtx();
  worker.scheduled(controller as never, env as never, ctx as never);
  assert.equal(ctx.pending.length, 1);
  await Promise.all(ctx.pending);

  const batch = fakeBatch([{ type: "unknown-yet" }]);
  await worker.queue(batch as never, env as never);
  assert.equal(batch.messages[0].acked, true);
});

test("the registries hold the send engine (GO-126): one cron task, one job type", () => {
  assert.equal(CRON_TASKS.length, 1);
  assert.deepEqual(Object.keys(JOB_HANDLERS), ["newsletter.send-batch"]);
});

test("scheduled with no tasks does not touch the database", async () => {
  const env = makeEnv();
  await handleScheduled(controller as never, env as never, []);
  assert.equal(env.NEWSLETTER_DB.calls.length, 0);
});

test("scheduled runs every task after the schema, a failing one does not stop the rest", async () => {
  const env = makeEnv();
  const ran: string[] = [];
  await handleScheduled(controller as never, env as never, [
    async () => {
      ran.push("a");
      throw new Error("a failed");
    },
    async (e, c) => {
      ran.push("b:" + c.cron + ":" + (e === (env as never)));
    },
  ]);
  assert.deepEqual(ran, ["a", "b:* * * * *:true"]);
  assert.ok(env.NEWSLETTER_DB.calls.some((s) => s.includes("schema_meta")));
});

test("queue: a handled job is acked, a failing one retried with backoff", async () => {
  const env = makeEnv();
  const got: unknown[] = [];
  const handlers = {
    ok: async (job: unknown) => void got.push(job),
    bad: async () => {
      throw new Error("transport down");
    },
  };
  const batch = fakeBatch([{ type: "ok", n: 1 }, { type: "bad" }, { type: "bad" }]);
  batch.messages[2].attempts = 9;
  await handleQueue(batch as never, env as never, handlers);
  assert.deepEqual(got, [{ type: "ok", n: 1 }]);
  assert.equal(batch.messages[0].acked, true);
  assert.deepEqual(batch.messages[1].retried, { delaySeconds: 60 });
  assert.equal(batch.messages[1].acked, false);
  assert.deepEqual(batch.messages[2].retried, { delaySeconds: 3600 });
});

test("queue: malformed bodies and prototype keys are acked as unknown", async () => {
  const batch = fakeBatch([null, "text", { type: 5 }, { type: "constructor" }, { type: "toString" }]);
  await handleQueue(batch as never, makeEnv() as never, {});
  assert.ok(batch.messages.every((m) => m.acked && m.retried === null));
});
