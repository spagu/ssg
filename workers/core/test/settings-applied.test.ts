// The module settings as everyone sees them (GO-149): hidden modules leave
// the bar (/api/core/modules) and the home page (/api/core/overview, their
// attention items too), the rest come in the set order under their labels —
// while a hidden module is still asked, still listed by /status, /info and
// the management view, and its own panel's routes answer as before (hiding
// is cosmetic). Without CORE_DB, or when it fails, nothing changes.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as modules from "../functions/api/core/modules.ts";
import * as overview from "../functions/api/core/overview.ts";
import * as status from "../functions/api/core/status.ts";
import * as info from "../functions/api/core/info.ts";
import * as manage from "../functions/api/core/manage/index.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { arrange, parseLabels, readState, relabel } from "../functions/api/core/_settings.ts";
import { REGISTRY, SCENARIOS } from "./demo.ts";
import { ORIGIN, PW_ENV, call, fakeFetch } from "./harness.ts";
import { fakeD1 } from "./d1.ts";
import { pw, send } from "./manage-fixture.ts";
import * as modulePatch from "../functions/api/core/manage/modules/[id].ts";
import * as order from "../functions/api/core/manage/order.ts";

// The comments template's own manifest route, loaded with that template's
// resolve hook (it imports types without `import type`).
await import("../../comments/test/resolve.mjs");
const commentsManifest = await import("../../comments/functions/api/comments/module/index.ts");

const routes = (name: string) =>
  Object.fromEntries(Object.entries(SCENARIOS[name]()).map(([p, a]) => [ORIGIN + p, a.raw !== undefined ? new Response(a.raw, { status: a.status }) : Response.json(a.body, { status: a.status })]));
const get = async (handler: Parameters<typeof call>[0], path: string, env: Record<string, unknown>) => {
  const r = await call(handler, { path, env, headers: pw() });
  assert.equal(r.status, 200, r.body);
  return r.json();
};
const ids = (items: { id: string }[]) => items.map((i) => i.id);

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
});
after(() => useRegistry());

test("hidden, ordered and labelled for everyone; still asked, still listed where the facts are", async () => {
  const f = fakeFetch(routes("todo"));
  try {
    const db = fakeD1();
    const env = { ...PW_ENV, CORE_DB: db };
    const before = await get(modules.onRequest, "/api/core/modules", env);
    assert.deepEqual(ids(before.items), ["comments", "newsletter", "newsletter-ui", "auth", "rate-limit", "kitchen-log", "core"]);
    await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/comments", env, { hidden: true });
    await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/kitchen-log", env, { labels: { en: "Kitchen", pl: "Kuchnia" } });
    await send(order.onRequest, "PUT", "/api/core/manage/order", env, { order: ["kitchen-log", "core", "auth", "comments", "newsletter", "newsletter-ui", "rate-limit"] });

    const bar = await get(modules.onRequest, "/api/core/modules?lang=pl", env);
    assert.deepEqual(ids(bar.items), ["kitchen-log", "core", "auth", "newsletter", "newsletter-ui", "rate-limit"]);
    assert.equal(bar.items[0].title, "Kuchnia");
    assert.equal(bar.items[1].title, "Rdzeń administracji", "no label in pl for core: the manifest's title");
    const de = await get(modules.onRequest, "/api/core/modules?lang=de", env);
    assert.match(de.items[0].title, /^Küchenbuch/, "a label exists only in the languages it was given in");

    const home = await get(overview.onRequest, "/api/core/overview", env);
    assert.deepEqual(ids(home.items), ["kitchen-log", "auth", "newsletter", "newsletter-ui", "rate-limit"]);
    assert.equal(home.items[0].title, "Kitchen");
    assert.ok(!home.attention.items.some((a: { module: string }) => a.module === "comments"), "a hidden module's attention items leave the home page too");
    assert.ok(home.attention.items.some((a: { module: string; title: string }) => a.module === "kitchen-log" && a.title === "Kitchen"));
    assert.ok(home.items.every((i: { unreachableSince: unknown }) => i.unreachableSince === null));

    assert.ok(ids((await get(status.onRequest, "/api/core/status", env)).items).includes("comments"), "status still asks it");
    assert.ok(f.sent.some((s) => s.url.endsWith("/api/comments/module/summary")), "its summary is still asked (its problems still reach /info)");
    assert.ok(ids((await get(info.onRequest, "/api/core/info", env)).modules).includes("comments"), "the facts table lists it");
    const m = (await get(manage.onRequest, "/api/core/manage", env)).items.find((i: { id: string }) => i.id === "comments");
    assert.equal(m.hidden, true, "the management view keeps it, to show it again");
  } finally {
    f.restore();
  }
});

test("a hidden module's own panel and API answer exactly as before: core never stands in between", async () => {
  const db = fakeD1();
  await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/comments", { ...PW_ENV, CORE_DB: db }, { hidden: true });
  const r = await call(commentsManifest.onRequestGet as never, {
    path: "/api/comments/module",
    env: { COMMENTS_ADMIN_PASSWORD: "moderator-pass" },
    headers: { authorization: "Basic " + btoa(":moderator-pass") },
  });
  assert.equal(r.status, 200);
  assert.equal(r.json().id, "comments");
});

test("without CORE_DB, or with a database that fails, the answers are exactly as the build listed them", async () => {
  const f = fakeFetch(routes("good"));
  try {
    const plain = await get(modules.onRequest, "/api/core/modules", PW_ENV);
    const broken = fakeD1();
    broken.state.failOn = /FROM core_module_settings/;
    const failing = await get(modules.onRequest, "/api/core/modules", { ...PW_ENV, CORE_DB: broken });
    assert.deepEqual(failing, plain);
    const home = await get(overview.onRequest, "/api/core/overview", PW_ENV);
    assert.ok(home.items.every((i: object) => !("unreachableSince" in i)), "no new field without CORE_DB");
    assert.equal(await readState(PW_ENV), null);
  } finally {
    f.restore();
  }
});

test("arrange, relabel, parseLabels", () => {
  const list = [{ id: "a" }, { id: "b" }, { id: "c" }, { id: "d" }];
  const s = (id: string, position: number | null, hidden = false) => [id, { id, hidden, position, labels: {}, note: "", updatedAt: null, updatedBy: null }] as const;
  const settings = new Map([s("c", 0), s("a", 1), s("d", null, true)]);
  assert.deepEqual(ids(arrange(list, (x) => x.id, settings)), ["c", "a", "b"]);
  assert.deepEqual(ids(arrange(list, (x) => x.id, settings, true)), ["c", "a", "b", "d"]);
  assert.equal(arrange(list, (x) => x.id, undefined), list);
  const labelled = new Map([["a", { ...settings.get("a")!, labels: { en: "Alpha" } }]]);
  assert.deepEqual(relabel({ id: "a", title: "A" }, labelled, "en"), { id: "a", title: "Alpha" });
  const same = { id: "a", title: "A" };
  assert.equal(relabel(same, labelled, "pl"), same);
  assert.deepEqual([parseLabels('{"en":"X","pl":"","de":3}'), parseLabels("[1]"), parseLabels("null"), parseLabels("{")], [{ en: "X" }, {}, {}, {}]);
});
