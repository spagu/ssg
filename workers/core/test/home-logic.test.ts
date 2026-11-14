// The home page's logic modules: state derivation and the other decisions
// (model.js), the API wrapper (api.js), preferences (prefs.js), formatting
// (format.js), strings (i18n.js) and the tree renderer (vnode.js).
import { test } from "node:test";
import assert from "node:assert/strict";
import * as model from "../public/core/js/model.js";
import { ApiError, BASE, classify, createApi } from "../public/core/js/api.js";
import { LANG_KEY, THEME_KEY, applyTheme, readStored, readTheme, writeStored, writeTheme } from "../public/core/js/prefs.js";
import { createFormat } from "../public/core/js/format.js";
import { createI18n, pickLang, readConfig } from "../public/core/js/i18n.js";
import { find, findAll, h, mount, render, text, uid } from "../public/core/js/vnode.js";
import { fakeDoc, textOf } from "./dom.ts";

const ok = (problems: unknown[] = [], extra = {}) => ({ state: "ok", metrics: [1, 2, 3, 4, 5].map((n) => ({ key: `k${n}`, label: `L${n}`, value: n, unit: null })), problems, series: null, ...extra });

test("cardState: the most serious first, from the status and the summary", () => {
  const cases: [Record<string, unknown>, Record<string, unknown> | undefined, Record<string, unknown>][] = [
    [{ summary: ok() }, { status: "unreachable", reason: "timeout" }, { state: "unavailable", reason: "timeout", httpStatus: null }],
    [{ summary: ok() }, { status: "unreachable", reason: "http_status", httpStatus: 502 }, { state: "unavailable", reason: "http_status", httpStatus: 502 }],
    [{ summary: null }, { status: "invalid_manifest", reason: "rejected", detail: "bad id" }, { state: "unavailable", reason: "invalid_manifest", detail: "bad id" }],
    [{ summary: null }, { status: "invalid_manifest", reason: "too_large" }, { state: "unavailable", reason: "invalid_manifest", detail: "too_large" }],
    [{ summary: null, status: "invalid_manifest" }, undefined, { state: "unavailable", reason: "invalid_manifest", detail: null }],
    [{ summary: null, status: "unreachable" }, undefined, { state: "unavailable", reason: "other" }],
    [{ summary: { state: "unavailable", reason: "deadline" } }, { status: "reachable" }, { state: "unavailable", reason: "deadline", httpStatus: null }],
    [{ summary: { state: "unavailable", reason: "http_status", httpStatus: 500 } }, undefined, { state: "unavailable", reason: "http_status", httpStatus: 500 }],
    [{ summary: ok() }, { status: "denied" }, { state: "denied" }],
    [{ summary: { state: "denied" } }, { status: "reachable" }, { state: "denied" }],
    [{ summary: ok([{ code: "a" }, { code: "b" }]) }, { status: "reachable" }, { state: "needsConfig", problems: 2 }],
    [{ summary: ok() }, undefined, { state: "working" }],
    [{ summary: { state: "invalid" } }, undefined, { state: "working" }],
    [{ summary: null }, { status: "reachable" }, { state: "working" }],
    [{ summary: null }, { status: "unknown" }, { state: "unchecked" }],
    [{ summary: null }, undefined, { state: "unchecked" }],
  ];
  for (const [item, status, want] of cases) assert.deepEqual(model.cardState(item, status), want, JSON.stringify([item, status]));
});

test("model helpers: pictures, reasons, metrics, series, hint, counts, keys, unanswered", () => {
  assert.equal(model.illustration("comments"), "img/modules/comments.svg");
  assert.equal(model.illustration("newsletter-ui"), "img/modules/newsletter.svg");
  assert.equal(model.illustration("my-own"), "img/modules/default.svg");
  assert.equal(model.illustration("__proto__"), "img/modules/default.svg");
  assert.deepEqual(["timeout", "nope", undefined].map(model.reasonKey), ["reason.timeout", "reason.other", "reason.other"]);
  assert.equal(model.metricsOf({ summary: ok() }).length, 4);
  assert.deepEqual([model.metricsOf({ summary: { state: "denied" } }), model.metricsOf({ summary: null })], [[], []]);
  const pts = (n: number) => ({ label: "s", points: Array.from({ length: n }, (_, i) => ({ day: `2026-11-0${i + 1}`, value: i })) });
  assert.equal(model.seriesOf({ summary: ok([], { series: pts(2) }) })!.points.length, 2);
  assert.deepEqual([model.seriesOf({ summary: ok([], { series: pts(1) }) }), model.seriesOf({ summary: ok() }), model.seriesOf({ summary: null })], [null, null, null]);
  const panels = [{ panel: "/a" }, { panel: "/b" }, { panel: null }];
  assert.deepEqual([model.authHint("password", panels), model.authHint("password", panels.slice(1)), model.authHint("auth", panels), model.authHint("access", panels)], [true, false, false, false]);
  assert.deepEqual(model.severityCounts([{ severity: "action" }, { severity: "action" }, { severity: "info" }, { severity: "odd" }]), { action: 2, warning: 0, info: 1 });
  assert.deepEqual(["password", "auth", "access", "x"].map(model.modeKey), ["mode.password", "mode.auth", "mode.access", "mode.unknown"]);
  assert.deepEqual(["registered", "live", "unreachable", "invalid_manifest", "x"].map(model.statusKey), ["modstatus.registered", "modstatus.live", "modstatus.unreachable", "modstatus.invalid_manifest", "modstatus.other"]);
  assert.deepEqual([{ kind: "signin" }, { kind: "weird" }, null].map(model.errorKey), ["error.signin", "error.server", "error.server"]);
  assert.equal(model.unanswered([{ summary: null }, { summary: { state: "ok" } }, { summary: { state: "denied" } }, { summary: { state: "invalid" } }]), 2);
  const idx = model.statusIndex({ items: [{ id: "a", status: "reachable" }, null, { id: 3 }] });
  assert.deepEqual([...idx.keys()], ["a"]);
  assert.equal(model.statusIndex(null).size, 0);
});

test("buildLines: facts when the report is there, otherwise how to publish it", () => {
  const fmt = createFormat("en", "UTC");
  const now = Date.parse("2026-11-13T10:12:00Z");
  const b = { version: "1.9.9", buildTime: "2026-11-13T07:12:00Z", finished: null, durationSeconds: 8.6, pages: 412, files: 1288 };
  assert.deepEqual(model.buildLines({ buildState: "ok", build: b }, fmt, now), [
    ["site.build.ok", { version: "1.9.9", time: "Nov 13, 2026, 7:12 AM", relative: "3 hours ago" }],
    ["site.build.sizeTime", { pages: "412", files: "1,288", duration: "8.6 sec" }],
  ]);
  const plain = model.buildLines({ buildState: "ok", build: { ...b, version: null, buildTime: null, finished: "2026-11-13T07:12:00Z", durationSeconds: null } }, fmt, now);
  assert.deepEqual([plain[0][1].version, plain[1][0]], ["—", "site.build.size"]);
  assert.equal(model.buildLines({ buildState: "ok", build: { ...b, pages: null } }, fmt, now).length, 1);
  assert.deepEqual(model.buildLines({ buildState: "absent", build: null, buildOption: "build_report" }, fmt), [["site.build.absent", { option: "build_report" }]]);
  assert.deepEqual(model.buildLines({ buildState: "invalid", build: null }, fmt), [["site.build.invalid", { option: "build_report" }]]);
  assert.deepEqual(model.buildLines({ buildState: "weird", build: null }, fmt), [["site.build.unreadable", { option: "build_report" }]]);
  assert.deepEqual(model.buildLines({ buildState: "ok", build: null }, fmt)[0][0], "site.build.unreadable");
});

test("loginHref: the configured login page on this host, with this page as next", () => {
  assert.equal(model.loginHref({}, "/core/"), "/auth/login?next=%2Fcore%2F");
  assert.equal(model.loginHref({ loginUrl: "/accounts/in?x=1" }, "/core/"), "/accounts/in?x=1&next=%2Fcore%2F");
  for (const bad of ["https://evil.example.com/", "//evil.example.com", "/\\evil", 7]) assert.equal(model.loginHref({ loginUrl: bad }, "/core/"), "/auth/login?next=%2Fcore%2F");
});

test("api: GET under /api/core/, JSON back; failures become kinds with words", async () => {
  const seen: [string, RequestInit][] = [];
  const answer = (status: number, body: unknown, headers: Record<string, string> = {}) => async (u: string, i: RequestInit) => (
    seen.push([u, i]), new Response(body === undefined ? "" : typeof body === "string" ? body : JSON.stringify(body), { status, headers: { "content-type": typeof body === "string" ? "text/html" : "application/json", ...headers } })
  );
  assert.deepEqual(await createApi(answer(200, { a: 1 })).get("overview", { lang: "pl" }), { a: 1 });
  assert.deepEqual([seen[0][0], seen[0][1].credentials], [`${BASE}overview?lang=pl`, "same-origin"]);
  await createApi(answer(200, { a: 1 })).get("status");
  assert.equal(seen[1][0], `${BASE}status`);
  const kinds: [number, unknown, Record<string, string>, string][] = [
    [404, "<html>", {}, "missing"],
    [401, { error: "unauthorized" }, { "www-authenticate": 'Bearer realm="ssg-auth"' }, "signin"],
    [401, { error: "unauthorized" }, { "www-authenticate": "Basic" }, "auth"],
    [401, { error: "unauthorized" }, {}, "auth"],
    [403, { error: "access_audience_wrong" }, {}, "forbidden"],
    [429, { error: "too_many_failed_logins" }, { "retry-after": "60" }, "rate"],
    [503, { error: "registry_missing", detail: "list workers/core" }, {}, "config"],
    [500, { error: "internal_error" }, {}, "server"],
    [400, null, {}, "server"],
  ];
  for (const [status, body, headers, kind] of kinds) {
    const e = await createApi(answer(status, body, headers)).get("x").catch((err) => err);
    assert.ok(e instanceof ApiError);
    assert.equal(e.kind, kind, String(status));
  }
  const rate = await createApi(answer(429, { error: "x" }, { "retry-after": "42" })).get("x").catch((e) => e);
  assert.equal(rate.retryAfter, 42);
  const cfg = await createApi(answer(503, { error: "registry_missing", detail: "do this" })).get("x").catch((e) => e);
  assert.deepEqual([cfg.code, cfg.detail, cfg.message], ["registry_missing", "do this", "do this"]);
  const net = await createApi(async () => {
    throw new TypeError("offline");
  }).get("x").catch((e) => e);
  assert.deepEqual([net.kind, net.detail], ["network", "offline"]);
  const odd = await createApi(async () => {
    throw "plain";
  }).get("x").catch((e) => e);
  assert.equal(odd.detail, "plain");
  const html = await createApi(answer(200, "<html>")).get("x").catch((e) => e);
  assert.deepEqual([html.kind, html.status], ["server", 200]);
  const bare = await createApi(async () => new Response(null, { status: 200 })).get("x").catch((e) => e);
  assert.equal(bare.kind, "server", "no content type: not JSON");
  const broken = await createApi(async () => new Response("{", { status: 200, headers: { "content-type": "application/json" } })).get("x").catch((e) => e);
  assert.equal(broken.kind, "server");
  assert.equal(classify(418, null, new Headers()).message, "server");
  const defaultFetch = createApi();
  const real = globalThis.fetch;
  globalThis.fetch = (async () => Response.json({ via: "global" })) as typeof fetch;
  try {
    assert.deepEqual(await defaultFetch.get("x"), { via: "global" });
  } finally {
    globalThis.fetch = real;
  }
});

test("prefs: remembered when storage works, defaults when it throws or is missing", () => {
  const store = new Map<string, string>();
  const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) };
  assert.equal(readTheme(storage), "auto");
  assert.equal(writeTheme(storage, "dark"), true);
  assert.equal(readTheme(storage), "dark");
  writeTheme(storage, "auto");
  assert.equal(store.has(THEME_KEY), false);
  store.set(THEME_KEY, "neon");
  assert.equal(readTheme(storage), "auto");
  writeStored(storage, LANG_KEY, "pl");
  assert.equal(readStored(storage, LANG_KEY), "pl");
  const broken = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); }, removeItem() { throw new Error("blocked"); } };
  assert.deepEqual([readTheme(broken), readStored(null, LANG_KEY), writeStored(broken, LANG_KEY, "x"), writeTheme(broken, "auto")], ["auto", null, false, false]);
  const root = fakeDoc().documentElement;
  assert.equal(applyTheme(root, "dark", false), "dark");
  assert.equal(root.attrs["data-theme"], "dark");
  assert.equal(applyTheme(root, "auto", true), "dark");
  assert.equal(root.attrs["data-theme"], undefined);
  assert.equal(applyTheme(root, "auto", false), "light");
});

test("format: numbers, units, times; a bad time is a dash", () => {
  const f = createFormat("en", "UTC");
  const de = createFormat("de", "UTC");
  assert.deepEqual([f.number(1284), f.number("x"), de.number(1284.56)], ["1,284", "0", "1.284,6"]);
  assert.deepEqual([f.metric(92.5, "percent"), f.metric(3, "days"), f.metric(1, "days"), f.metric(7, null), f.metric("x", "percent"), f.metric(null, "days")], ["92.5%", "3 days", "1 day", "7", "0%", "0 days"]);
  assert.deepEqual([f.seconds(8.6), f.seconds(null)], ["8.6 sec", "0 sec"]);
  assert.deepEqual([f.dateTime("2026-11-13T07:12:00Z"), f.dateTime("nope"), f.time("2026-11-13T07:12:00Z"), f.time(undefined)], ["Nov 13, 2026, 7:12 AM", "—", "7:12 AM", "—"]);
  assert.deepEqual([f.day("2026-11-13"), f.dayLong("2026-11-13")], ["Nov 13", "November 13, 2026"]);
  const now = Date.parse("2026-11-13T12:00:00Z");
  assert.deepEqual(
    ["2026-11-13T11:59:30Z", "2026-11-13T11:30:00Z", "2026-11-13T09:00:00Z", "2026-11-10T12:00:00Z", "bad"].map((iso) => f.relative(iso, now)),
    ["30 seconds ago", "30 minutes ago", "3 hours ago", "3 days ago", "—"],
  );
  assert.equal(typeof createFormat("en").relative("2026-11-13T11:59:30Z"), "string");
});

test("i18n: language choice, config block, lookups with plurals, overrides and fallbacks", () => {
  assert.deepEqual([pickLang(["xx", "pl-PL"]), pickLang(["de_AT"]), pickLang([null, "zz"]), pickLang([])], ["pl", "de", "en", "en"]);
  assert.deepEqual([readConfig('{"lang":"pl"}'), readConfig("[1]"), readConfig("{"), readConfig(""), readConfig(undefined), readConfig("null")], [{ lang: "pl" }, {}, {}, {}, {}, {}]);
  const tables = { en: { a: "A {x}", n: { one: "{count} thing", other: "{count} things" }, only: "en only" }, pl: { a: "PL {x}", n: { one: "{count} rzecz", few: "{count} rzeczy", many: "{count} rzeczy", other: "{count} rzeczy" } } };
  const i = createI18n("pl", tables, { pl: { a: "override {x}" } }, (n: number) => `#${n}`);
  assert.deepEqual([i.t("a", { x: 1 }), i.t("n", { count: 1 }), i.t("n", { count: 3 }), i.t("only"), i.t("missing"), i.t("a"), i.has("a"), i.has("zz")], ["override 1", "#1 rzecz", "#3 rzeczy", "en only", "missing", "override {x}", true, false]);
  const en = createI18n("en", { en: { n: { other: "{count} x" } } });
  assert.equal(en.t("n", { count: 1 }), "1 x", "a missing form falls back to other");
});

test("vnode: text is a text node, attributes go through setAttribute, events and refs are wired", () => {
  const doc = fakeDoc();
  let ref: unknown = null;
  let clicked = 0;
  const tree = h(
    "div",
    { class: "x", hidden: true, skip: false, nothing: null, value: "v", style: { color: "red" }, ref: (el: unknown) => (ref = el), on: { click: () => clicked++ } },
    '<script>alert(1)</script>',
    7,
    [null, false, true, h("svg", { viewBox: "0 0 1 1" }, h("path", { d: "M0" }))],
  );
  const el = render(tree, doc as never) as never as ReturnType<typeof doc.createElement>;
  assert.equal(ref, el);
  assert.deepEqual([el.attrs.class, el.attrs.hidden, "skip" in el.attrs, "nothing" in el.attrs, (el as Record<string, unknown>).value, el.style.color], ["x", "", false, false, "v", "red"]);
  assert.equal(textOf(el), "<script>alert(1)</script>7");
  const svg = el.children[2] as ReturnType<typeof doc.createElement>;
  assert.deepEqual([svg.ns, (svg.children[0] as ReturnType<typeof doc.createElement>).ns], ["http://www.w3.org/2000/svg", "http://www.w3.org/2000/svg"]);
  el.listeners.click[0]({});
  assert.equal(clicked, 1);
  assert.equal(text(tree), "<script>alert(1)</script>7");
  assert.equal(find(tree, (n: { tag: string }) => n.tag === "path").props.d, "M0");
  assert.equal(find("x", () => true), null);
  assert.equal(find(tree, () => false), null);
  assert.equal(findAll(tree, (n: { tag: string }) => n.tag !== undefined).length, 3);
  const parent = doc.createElement("main");
  mount(parent as never, [h("p", null, "a"), null, false, [h("p", null, "b")]], doc as never);
  assert.equal(parent.children.length, 2);
  assert.notEqual(uid("a"), uid("a"));
});
