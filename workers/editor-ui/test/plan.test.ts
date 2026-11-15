// EDITOR_PLAN (GO-186): free by default, paid on request, anything else a 503
// on every path naming the variable — never a silent fall-back. And the Paid
// subrequest budget holds for the worst publication the Paid limits allow.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, GIT_VARS, makeEnv, request, run } from "./fixtures.ts";
import { setup, AUTHOR } from "./repo-fixtures.ts";
import { BODY_LIMIT, PLAN_VAR, SUBREQUEST_BUDGET, activePlan, planOf } from "../portable/plan.ts";
import { MAX_BODY } from "../portable/http-kit.ts";
import { repositoryOf } from "../portable/repo.ts";
import { OpError } from "../portable/ops.ts";
import { MB, PLAN_LIMITS } from "../portable/repo-provider.ts";
import type { Change } from "../portable/repo-provider.ts";
import { PERM } from "../src/permissions.ts";

const PROBLEM = 'EDITOR_PLAN must be "free" or "paid" (empty = free)';

test("planOf: unset, empty and any case of free/paid; anything else names the variable", () => {
  assert.equal(PLAN_VAR, "EDITOR_PLAN");
  for (const v of [undefined, "", "  ", "free", "FREE", " Free "]) {
    assert.deepEqual(planOf({ EDITOR_PLAN: v } as never), { ok: true, active: { plan: "free", limits: PLAN_LIMITS.free, subrequests: 45, maxBody: MAX_BODY } }, String(v));
  }
  for (const v of ["paid", "PAID", " paid\n"]) {
    assert.deepEqual(planOf({ EDITOR_PLAN: v } as never), { ok: true, active: { plan: "paid", limits: PLAN_LIMITS.paid, subrequests: 125, maxBody: 28 * MB } }, v);
  }
  for (const v of ["pro", "free,paid", "constructor", 1, true, null, {}]) {
    assert.deepEqual(planOf({ EDITOR_PLAN: v } as never), { ok: false, problem: PROBLEM }, String(v));
  }
});

test("activePlan: the plan, or 503 bad_plan for an operation called directly", () => {
  assert.equal(activePlan({} as never).plan, "free");
  assert.throws(() => activePlan({ EDITOR_PLAN: "pro" } as never), (e: unknown) => e instanceof OpError && e.code === "bad_plan" && e.status === 503 && e.detail === PROBLEM);
});

test("an unknown EDITOR_PLAN: 503 bad_plan on every path, the panel and a request without a session too", async () => {
  const env = makeEnv({ EDITOR_PLAN: "enterprise" });
  for (const [path, perms] of [["/api/health", [PERM.read]], ["/api/site", [PERM.read]], ["/", [PERM.read]], ["/api/me", null]] as const) {
    const res = await run(await request(BASE + path, perms as string[] | null), env);
    assert.equal(res.status, 503, path);
    assert.deepEqual(await res.json(), { error: "bad_plan", detail: PROBLEM });
    assert.equal(res.headers.get("x-frame-options"), "DENY");
  }
});

test("repositoryOf: the plan's limits and budget; an unknown plan is the problem", () => {
  const free = repositoryOf(GIT_VARS as never, {});
  assert.ok(free.ok && free.repo.plan.plan === "free" && free.repo.plan.subrequests === SUBREQUEST_BUDGET.free);
  const paid = repositoryOf({ ...GIT_VARS, EDITOR_PLAN: "paid" } as never, {});
  assert.ok(paid.ok && paid.repo.plan.limits === PLAN_LIMITS.paid);
  assert.deepEqual(repositoryOf({ ...GIT_VARS, EDITOR_PLAN: "x" } as never, {}), { ok: false, problem: PROBLEM });
});

test("the budgets: within Cloudflare's per-invocation limits (Free 50, Paid 10,000), bodies within 128 MB", () => {
  assert.ok(SUBREQUEST_BUDGET.free <= 50 - 5);
  assert.ok(SUBREQUEST_BUDGET.paid <= 10_000);
  const worst = (files: number) => files + 5 + files + 2 * 6; // GO-168: files + 5 + directories + 6 per retry
  assert.equal(worst(PLAN_LIMITS.free.maxFilesPerChange), 37);
  assert.equal(SUBREQUEST_BUDGET.paid, worst(PLAN_LIMITS.paid.maxFilesPerChange) + (SUBREQUEST_BUDGET.free - 37));
  for (const plan of ["free", "paid"] as const) {
    assert.ok(BODY_LIMIT[plan] >= Math.ceil(PLAN_LIMITS[plan].maxChangeBytes / 3) * 4, `${plan}: a full change as base64 fits the body`);
    assert.ok(BODY_LIMIT[plan] * 2.4 < 128 * MB, `${plan}: GO-168's 2.4x the body stays within the isolate`);
  }
});

test("Paid budget in code: 50 files in 50 directories, two lost races = 117 within 125", async () => {
  const { gh, provider } = await setup({ maxSubrequests: SUBREQUEST_BUDGET.paid, limits: PLAN_LIMITS.paid });
  let races = 2;
  const other = async () => { await gh.store.commitFiles("main", { [`content/other${races}.md`]: "x" }); if (--races > 0) gh.beforePatch = other; };
  gh.beforePatch = other;
  const changes: Change[] = Array.from({ length: PLAN_LIMITS.paid.maxFilesPerChange }, (_, i) => ({ op: "put", path: `content/d${i}/f.md`, content: { text: `f${i}` }, baseBlobId: null }));
  const r = await provider.commit({ branch: "main", message: "Publish", author: AUTHOR, changes });
  assert.equal(r.attempts, 3);
  assert.equal(provider.used(), 117);
});
