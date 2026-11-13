// Two engine cleanups found in review (fixed with GO-127): deleting a
// campaign takes the engine's plan and ranges with it, and resuming clears
// the reason the engine paused it for.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { campaignRow, fakeMailer, planError, runJob, started } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const call = (env: EngineEnv, method: string, path: string) =>
  handleFetch(req(`/newsletter-admin/api/campaigns${path}`, { method, auth: PASSWORD }), env as never, fakeCtx() as never);
const rows = (env: EngineEnv, table: string, id: string) =>
  Number((env.NEWSLETTER_DB.sqlite.prepare(`SELECT COUNT(*) AS n FROM ${table} WHERE campaign_id = ?`).get(id) as { n: number }).n);

test("deleting a cancelled campaign removes its send_plans and send_batches rows", async () => {
  const { env, id } = await started(3);
  const other = await started(0); // a second database: nothing shared
  assert.equal(rows(env, "send_plans", id), 1);
  assert.ok(rows(env, "send_batches", id) > 0);
  assert.equal((await call(env, "POST", `/${id}/cancel`)).status, 200);
  assert.equal((await call(env, "DELETE", `/${id}`)).status, 204);
  for (const t of ["send_plans", "send_batches", "campaign_sends"]) assert.equal(rows(env, t, id), 0, t);
  assert.equal(rows(other.env, "send_plans", other.id), 1);
});

test("a refused delete leaves the engine's rows alone", async () => {
  const { env, id } = await started(2);
  assert.equal((await call(env, "DELETE", `/${id}`)).status, 409);
  assert.equal(rows(env, "send_plans", id), 1);
  assert.ok(rows(env, "send_batches", id) > 0);
});

test("resume clears the engine's pause reason; a refused resume leaves it", async () => {
  const { env, id, jobs } = await started(2, {}, { body: "Hi {{name}}" });
  await runJob(env, jobs[0], fakeMailer().open);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
  assert.match(String(planError(env, id)), /^render:/);

  // Not paused any more → 409, the reason stays (it still explains the state).
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'cancelled' WHERE id = ?").run(id);
  assert.equal((await call(env, "POST", `/${id}/resume`)).status, 409);
  assert.match(String(planError(env, id)), /^render:/);

  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'paused' WHERE id = ?").run(id);
  const res = await call(env, "POST", `/${id}/resume`);
  assert.equal(res.status, 200);
  assert.equal((await res.json()).status, "sending");
  assert.equal(planError(env, id), null);
  const progress = await (await call(env, "GET", `/${id}/progress`)).json();
  assert.equal(progress.error, null);
});
