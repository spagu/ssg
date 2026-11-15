// DESIGNER_PLAN (GO-186): free by default, paid on request, anything else a
// 503 on every path naming the variable. The plan sets the publication limits
// the provider enforces and the request body the routes accept.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BODY_LIMIT, PLAN_VAR, activePlan, planOf } from "../portable/plan.ts";
import { OpError } from "../portable/ops.ts";
import { repositoryOf } from "../portable/repo.ts";
import { BASE, GIT_VARS, READ, call, makeEnv, request, run, site } from "./fixtures.ts";
import { MB, PLAN_LIMITS } from "../portable/repo-provider.ts";

const PROBLEM = 'DESIGNER_PLAN must be "free" or "paid" (empty = free)';

test("planOf: unset, empty, free, paid in any case; anything else names DESIGNER_PLAN", () => {
  assert.equal(PLAN_VAR, "DESIGNER_PLAN");
  for (const v of [undefined, " ", "Free"]) assert.equal((planOf({ DESIGNER_PLAN: v } as never) as { active: { plan: string } }).active.plan, "free");
  assert.deepEqual(planOf({ DESIGNER_PLAN: "PAID" } as never), { ok: true, active: { plan: "paid", limits: PLAN_LIMITS.paid, subrequests: 125, maxBody: 28 * MB } });
  for (const v of ["business", 2, null]) assert.deepEqual(planOf({ DESIGNER_PLAN: v } as never), { ok: false, problem: PROBLEM });
  assert.throws(() => activePlan({ DESIGNER_PLAN: "x" } as never), (e: unknown) => e instanceof OpError && e.code === "bad_plan" && e.status === 503);
  assert.deepEqual(repositoryOf({ ...GIT_VARS, DESIGNER_PLAN: "x" } as never, {}), { ok: false, problem: PROBLEM });
  const paid = repositoryOf({ ...GIT_VARS, DESIGNER_PLAN: "paid" } as never, {});
  assert.ok(paid.ok && paid.repo.plan.limits === PLAN_LIMITS.paid && paid.repo.plan.subrequests === 125);
});

test("an unknown DESIGNER_PLAN: 503 bad_plan on every path, the panel and a request without a session too", async () => {
  const env = makeEnv({ DESIGNER_PLAN: "enterprise" });
  for (const [path, perms] of [["/api/health", READ], ["/api/settings", READ], ["/", READ], ["/api/me", null]] as const) {
    const res = await run(await request(BASE + path, perms as string[] | null), env);
    assert.equal(res.status, 503, path);
    assert.deepEqual(await res.json(), { error: "bad_plan", detail: PROBLEM });
  }
});

test("DESIGNER_PLAN=paid: api/health reports it; a body above the Free 8 MiB passes the size check", async () => {
  const gh = await site();
  const paid = makeEnv({ DESIGNER_PLAN: "paid" });
  const health = await (await run(await request(BASE + "/api/health", READ), paid)).json();
  assert.deepEqual(health.plan, { plan: "paid", limits: PLAN_LIMITS.paid, subrequests: 125, maxBody: BODY_LIMIT.paid });
  const message = "x".repeat(BODY_LIMIT.free + 1);
  const init = { method: "POST", body: JSON.stringify({ message }), headers: { origin: "https://example.com", "content-type": "application/json" } };
  const free = await call(gh, "/api/theme/publish", ["designer-ui:*"], init);
  assert.deepEqual([free.status, free.body.detail], [413, `the request body is larger than ${BODY_LIMIT.free} bytes`]);
  const past = await call(gh, "/api/theme/publish", ["designer-ui:*"], init, paid);
  assert.notEqual(past.status, 413, "read whole on Paid; refused afterwards for what it says, not its size");
  const declared = await call(gh, "/api/theme/publish", ["designer-ui:*"], { ...init, headers: { ...init.headers, "content-length": String(BODY_LIMIT.paid + 1) } }, paid);
  assert.deepEqual([declared.status, declared.body.detail], [413, `the request body is larger than ${BODY_LIMIT.paid} bytes`]);
});
