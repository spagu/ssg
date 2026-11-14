// Status history (GO-149): /api/core/status records a module's flip between
// reachable and unreachable — once per flip, only on observation — and the
// home page and the management view say since when; retention is bounded and
// a database failure never costs the status answer.
import { after, beforeEach, mock, test } from "node:test";
import assert from "node:assert/strict";
import * as status from "../functions/api/core/status.ts";
import * as overview from "../functions/api/core/overview.ts";
import * as manage from "../functions/api/core/manage/index.ts";
import { MAX_HISTORY_ROWS, latestStatuses, observe, purgeHistory, recentHistory } from "../functions/api/core/_history.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { ensureSchema } from "../functions/api/core/_schema.ts";
import { REGISTRY, SCENARIOS } from "./demo.ts";
import { ORIGIN, PW_ENV, call, fakeFetch } from "./harness.ts";
import { fakeD1 } from "./d1.ts";
import { pw } from "./manage-fixture.ts";

const routes = (patch: Record<string, Response | "network-error"> = {}) => ({
  ...Object.fromEntries(Object.entries(SCENARIOS.good()).map(([p, a]) => [ORIGIN + p, a.raw !== undefined ? new Response(a.raw, { status: a.status }) : Response.json(a.body, { status: a.status })])),
  ...patch,
});
const DOWN = { [`${ORIGIN}/newsletter-admin/api/health`]: "network-error" as const };
const ask = async (env: Record<string, unknown>) => {
  clearStatusCache();
  const r = await call(status.onRequest, { path: "/api/core/status", env, headers: pw() });
  assert.equal(r.status, 200, r.body);
  return r.json();
};

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
});
after(() => useRegistry());

test("a flip is recorded once, with its reason; the home page and the view say since when", async () => {
  mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T14:10:00Z") });
  const db = fakeD1();
  const env = { ...PW_ENV, CORE_DB: db };
  let f = fakeFetch(routes());
  try {
    await ask(env);
    const first = db.rows("SELECT module_id, status FROM core_status_history ORDER BY module_id");
    assert.ok(first.length > 0 && first.every((r) => r.status === "reachable"), "the first observation is the baseline");
    assert.ok(!first.some((r) => r.module_id === "rate-limit"), "unknown (no endpoint) says nothing about being up");
    await ask(env);
    assert.equal(db.rows("SELECT COUNT(*) AS n FROM core_status_history")[0].n, first.length, "no change, no row");
    f.restore();
    f = fakeFetch(routes(DOWN));
    mock.timers.tick(60_000);
    await ask(env);
    await ask(env);
    const down = db.rows("SELECT status, reason, observed_at FROM core_status_history WHERE module_id = 'newsletter-ui' ORDER BY observed_at");
    assert.deepEqual(down.map((r) => r.status), ["reachable", "unreachable"]);
    assert.equal(down[1].reason, "network");
    const home = (await call(overview.onRequest, { path: "/api/core/overview", env, headers: pw() })).json();
    assert.equal(home.items.find((i: { id: string }) => i.id === "newsletter-ui").unreachableSince, "2026-11-13T14:11:00.000Z");
    const v = (await call(manage.onRequest, { path: "/api/core/manage", env, headers: pw() })).json();
    assert.deepEqual(v.items.find((i: { id: string }) => i.id === "newsletter-ui").status, { status: "unreachable", reason: "network", at: "2026-11-13T14:11:00.000Z" });
    assert.deepEqual(v.history[0], { module: "newsletter-ui", status: "unreachable", reason: "network", at: "2026-11-13T14:11:00.000Z" });
    f.restore();
    f = fakeFetch(routes());
    await ask(env);
    assert.equal((await latestStatuses(db as never)).get("newsletter-ui")?.status, "reachable");
  } finally {
    f.restore();
    mock.timers.reset();
  }
});

test("retention: old rows go with the next write, each module's latest row stays; the row cap holds", async () => {
  const db = fakeD1();
  await ensureSchema(db as never);
  const insert = db.sqlite.prepare("INSERT INTO core_status_history (id, module_id, status, reason, observed_at) VALUES (?, ?, ?, NULL, ?)");
  insert.run("1", "a", "reachable", "2026-01-01T00:00:00.000Z");
  insert.run("2", "a", "unreachable", "2026-02-01T00:00:00.000Z");
  insert.run("3", "b", "unreachable", "2026-01-15T00:00:00.000Z");
  mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T00:00:00Z") });
  try {
    await observe({ CORE_DB: db, CORE_HISTORY_DAYS: "30" } as never, [{ id: "c", status: "reachable" }]);
  } finally {
    mock.timers.reset();
  }
  assert.deepEqual(db.rows("SELECT id FROM core_status_history ORDER BY module_id").map((r) => r.id).filter((id) => id !== undefined).length, 3);
  assert.deepEqual((await recentHistory(db as never)).map((r) => r.module), ["c", "a", "b"], "a's January row is gone, b's only row stays");
  const many = fakeD1();
  await ensureSchema(many as never);
  const put = many.sqlite.prepare("INSERT INTO core_status_history (id, module_id, status, reason, observed_at) VALUES (?, 'm', 'reachable', NULL, ?)");
  for (let i = 0; i < MAX_HISTORY_ROWS + 3; i++) put.run(String(i), new Date(Date.UTC(2026, 10, 1) + i * 1000).toISOString());
  await many.batch(purgeHistory(many as never, "2000-01-01T00:00:00.000Z") as never);
  assert.equal(many.rows("SELECT COUNT(*) AS n FROM core_status_history")[0].n, MAX_HISTORY_ROWS);
});

test("observe: nothing without CORE_DB or without an up/down answer; a database failure is swallowed", async () => {
  await observe(PW_ENV, [{ id: "a", status: "reachable" }]);
  const db = fakeD1();
  await observe({ CORE_DB: db } as never, [{ id: "a", status: "denied" }, { id: "b", status: "unknown" }]);
  assert.equal(db.calls.length, 0);
  db.state.failOn = /FROM core_status_history|INTO core_status_history/;
  await observe({ CORE_DB: db } as never, [{ id: "a", status: "reachable" }]);
  const f = fakeFetch(routes());
  try {
    const answer = await ask({ ...PW_ENV, CORE_DB: db });
    assert.ok(answer.items.length > 0, "the status answer does not depend on the history");
  } finally {
    f.restore();
  }
});
