// GET /api/core/overview: each module's summary as a state of its own, the
// deadline on a fake clock, hostile / oversized / invalid summaries, the
// merged attention list (order, cap, links that cannot leave the module's
// base), newsletter-ui preferred over newsletter, the per-caller cache, and
// what an account limited to one module sees.
import { after, beforeEach, mock, test } from "node:test";
import assert from "node:assert/strict";
import * as overview from "../functions/api/core/overview.ts";

import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { DETAIL_MAX } from "../functions/api/core/_summary.ts";

import { MODULE } from "../functions/api/core/_module.ts";
import { HOSTILE, REGISTRY, SCENARIOS } from "./demo.ts";
import { AUTH_ENV, ORIGIN, PASSWORD, PW_ENV, basic, call, entry, fakeFetch, freshIP, manifest, registry, session, settle, settleUntil, slow, text } from "./harness.ts";

const pw = (cookie = "s=1") => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP(), cookie });
// scenario: a demo scenario as fakeFetch routes (full URLs).
function scenario(name: string, patch: Record<string, Response | "network-error" | ((s: AbortSignal) => Promise<Response>)> = {}) {
  const routes: Record<string, Response | "network-error" | ((s: AbortSignal) => Promise<Response>)> = {};
  for (const [path, a] of Object.entries(SCENARIOS[name]())) routes[ORIGIN + path] = a.raw !== undefined ? new Response(a.raw, { status: a.status }) : Response.json(a.body, { status: a.status });
  for (const [path, a] of Object.entries(patch)) routes[ORIGIN + path] = a;
  return routes;
}
async function get(env: Record<string, unknown> = PW_ENV, headers: Record<string, string> = pw(), query = "") {
  const r = await call(overview.onRequest, { path: `/api/core/overview${query}`, headers, env });
  assert.equal(r.status, 200, r.body);
  const body = r.json();
  return { body, byId: Object.fromEntries(body.items.map((i: { id: string }) => [i.id, i])) as Record<string, any> };
}

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
});
after(() => useRegistry());

test("good: every summary ok, in the asked language; core is not an item; numbers shown once for the newsletter", async () => {
  const f = fakeFetch(scenario("good"));
  try {
    const { body, byId } = await get(PW_ENV, pw(), "?lang=pl");
    assert.deepEqual(body.items.map((i: { id: string }) => i.id), ["comments", "newsletter", "newsletter-ui", "auth", "rate-limit", "kitchen-log"]);
    assert.equal(body.lang, "pl");
    assert.equal(byId.comments.title, "Komentarze");
    assert.deepEqual(byId.comments.summary.metrics[0], { key: "pending", label: "Czekają na moderację", value: 0, unit: null });
    assert.equal(byId["kitchen-log"].summary.metrics[1].unit, "percent");
    assert.equal(byId["newsletter-ui"].summary.series.label, "Zapisy dziennie");
    assert.equal(byId["newsletter-ui"].summary.series.points.length, 30);
    // newsletter's numbers are newsletter-ui's: shown there, not twice.
    assert.deepEqual([byId.newsletter.summary.metrics, byId.newsletter.summary.series, byId.newsletter.summary.metricsIn], [[], null, "newsletter-ui"]);
    assert.equal(byId.auth.summary, null, "no summary declared");
    assert.deepEqual(body.attention, { items: [], total: 0 });
    // Only the caller's credential travelled, never a redirect followed.
    for (const s of f.sent) assert.deepEqual([s.redirect, Object.keys(s.headers).sort()], ["manual", ["authorization", "cookie"]]);
  } finally {
    f.restore();
  }
});

test("todo: attention merged across modules — severity, then module; hostile text stays a string; links joined on the module's base", async () => {
  const f = fakeFetch(scenario("todo"));
  try {
    const { body, byId } = await get();
    const brief = body.attention.items.map((a: { module: string; severity: string; href: string }) => `${a.severity} ${a.module} ${a.href}`);
    assert.deepEqual(brief, [
      "action comments /comments-admin.html",
      "action newsletter-ui /newsletter-admin/#/campaigns?status=paused",
      "warning comments /comments-admin.html",
      "warning newsletter-ui /newsletter-admin/#/",
      "info kitchen-log /kitchen/#/photos",
    ]);
    assert.equal(body.attention.total, 5);
    const hostile = body.attention.items[4];
    assert.ok(hostile.text.includes(HOSTILE) && hostile.title.includes(HOSTILE), "passed on as text — the page inserts it as text");
    assert.equal(byId.newsletter.summary.problems[0].code, "no_turnstile_secret");
    assert.equal(byId.comments.summary.metrics[0].value, 12);
  } finally {
    f.restore();
  }
});

test("down: each failure is that module's state — never the whole answer's", async () => {
  const big = JSON.stringify({ contract: 1, pad: "x".repeat(70_000) });
  const f = fakeFetch(
    scenario("down", {
      "/newsletter-admin/api/module/summary": new Response("", { status: 403 }),
      "/api/newsletter/module/summary": new Response(big),
      "/kitchen/api/module/summary": new Response(null, { status: 302, headers: { location: "https://evil.example.com/" } }),
    }),
  );
  try {
    const { byId } = await get();
    const st = (id: string) => [byId[id].summary.state, byId[id].summary.reason ?? null, byId[id].summary.httpStatus ?? null];
    assert.deepEqual(st("comments"), ["unavailable", "http_status", 500]);
    assert.deepEqual(st("newsletter-ui"), ["denied", null, 403]);
    assert.deepEqual(st("newsletter"), ["invalid", "too_large", 200]);
    assert.deepEqual(st("kitchen-log"), ["unavailable", "redirect", 302]);
    assert.ok(!f.sent.some((s) => s.url.includes("evil")));
  } finally {
    f.restore();
  }
});

test("invalid summaries: not JSON, wrong shape, a newer contract — the reader's finding, cut short", async () => {
  const long = { contract: 1, generatedAt: "2026-11-13T00:00:00Z", metrics: [], attention: [], problems: [{ code: "x".repeat(900), text: text("t") }] };
  const cases: [unknown, string, RegExp][] = [
    ["<html>", "not_json", /^$/],
    [{ contract: 1, generatedAt: "yesterday" }, "rejected", /required|pattern/],
    [{ contract: 9, generatedAt: "2026-11-13T00:00:00Z", metrics: [], attention: [], problems: [] }, "rejected", /contract 9/],
    [long, "rejected", /./],
  ];
  for (const [body, reason, detail] of cases) {
    clearStatusCache();
    const f = fakeFetch(scenario("good", { "/kitchen/api/module/summary": typeof body === "string" ? new Response(body) : Response.json(body) }));
    try {
      const s = (await get()).byId["kitchen-log"].summary;
      assert.deepEqual([s.state, s.reason], ["invalid", reason]);
      assert.match(s.detail ?? "", detail);
      assert.ok((s.detail ?? "").length <= DETAIL_MAX);
    } finally {
      f.restore();
    }
  }
});

test("a slow module: the answer comes at the deadline, the others are not held up", async () => {
  mock.timers.enable({ apis: ["setTimeout", "Date"], now: 1_000_000 });
  const f = fakeFetch(scenario("good", { "/kitchen/api/module/summary": slow(60_000, () => new Response("{}")) }));
  try {
    const env = { ...PW_ENV, CORE_PROBE_TIMEOUT_MS: "5000", CORE_STATUS_DEADLINE_MS: "3000" };
    let done = false;
    const pending = get(env).then((r) => ((done = true), r));
    await settleUntil(() => f.sent.some((r) => r.url.includes("/kitchen/api/module/summary"))); // its timeout is set before it is asked
    mock.timers.tick(2999);
    await settle();
    assert.equal(done, false);
    mock.timers.tick(1);
    await settle();
    const { byId } = await pending;
    assert.deepEqual([byId["kitchen-log"].summary.state, byId["kitchen-log"].summary.reason], ["unavailable", "deadline"]);
    assert.equal(byId.comments.summary.state, "ok");
  } finally {
    f.restore();
    mock.timers.reset();
  }
});

test("per-caller cache: the same caller is answered from it, another caller is asked afresh", async () => {
  const f = fakeFetch(scenario("good"));
  try {
    const first = (await get(PW_ENV, pw("s=1"))).byId.comments.summary;
    const again = (await get(PW_ENV, pw("s=1"))).byId.comments.summary;
    assert.deepEqual([first.cached, again.cached], [undefined, true]);
    const n = f.sent.length;
    assert.equal((await get(PW_ENV, pw("s=2"))).byId.comments.summary.cached, undefined);
    assert.ok(f.sent.length > n);
  } finally {
    f.restore();
  }
});

test("auth mode: an account limited to comments sees only comments, and only its attention", async () => {
  const f = fakeFetch(scenario("todo"));
  try {
    const { body } = await get(AUTH_ENV, { cookie: await session(["comments:read"]) });
    assert.deepEqual(body.items.map((i: { id: string }) => i.id), ["comments"]);
    assert.deepEqual(new Set(body.attention.items.map((a: { module: string }) => a.module)), new Set(["comments"]));
    assert.ok(!f.sent.some((s) => s.url.includes("newsletter") || s.url.includes("kitchen")), "modules the caller cannot see are not asked");
    assert.equal(body.canManage, false, "without core:manage the page offers no Arrange link");
    assert.equal((await get(AUTH_ENV, { cookie: await session(["comments:read", "core:manage"]) })).body.canManage, true);
    assert.equal((await get(PW_ENV)).body.canManage, true, "a shared-password admin may arrange modules");
  } finally {
    f.restore();
  }
});

test("a module listed live (no manifest from the build) is asked for its manifest, then its summary, in one deadline", async () => {
  useRegistry(registry(entry("kitchen-log", null, "/kitchen"), entry("core", MODULE)));
  const f = fakeFetch(scenario("good"));
  try {
    const { byId } = await get();
    assert.deepEqual([byId["kitchen-log"].status, byId["kitchen-log"].summary.state], ["live", "ok"]);
    assert.deepEqual(f.sent.map((s) => s.url), [`${ORIGIN}/kitchen/api/module`, `${ORIGIN}/kitchen/api/module/summary`]);
  } finally {
    f.restore();
  }
});

