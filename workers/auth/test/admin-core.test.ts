// GO-138: the admin panel's core modules — the fetch wrapper (CSRF header,
// renewal, errors in kinds), routing, strings, formatting, preferences and
// the DOM builder — without a browser.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ApiError, CSRF_HEADER, classify, createApi, query } from "../public/auth/assets/admin/js/core/api.js";
import { href, isRoute, parse } from "../public/auth/assets/admin/js/core/router.js";
import { createI18n, pickLang, readConfig } from "../public/auth/assets/admin/js/core/i18n.js";
import { createFormat, isoDay } from "../public/auth/assets/admin/js/core/format.js";
import { LANG_KEY, THEME_KEY, applyTheme, readStored, readTheme, writeStored } from "../public/auth/assets/admin/js/core/prefs.js";
import { find, findAll, h, mount, render, text, uid } from "../public/auth/assets/admin/js/core/vnode.js";

const reply = (status: number, body: unknown, type = "application/json") => new Response(body === null ? null : typeof body === "string" ? body : JSON.stringify(body), { status, headers: { "content-type": type } });

test("api: the CSRF header on every change, none on GET; JSON both ways; base and query", async () => {
  const calls: { url: string; init: RequestInit }[] = [];
  const api = createApi({ fetch: async (url: string, init: RequestInit) => (calls.push({ url, init }), reply(200, { ok: true })) });
  assert.deepEqual(await api.get("admin/users", { q: "a b", status: "", x: null }), { ok: true });
  await api.post("admin/x", { a: 1 });
  await api.post("admin/y");
  await api.put("admin/z", { b: 2 });
  await api.del("/admin/w");
  assert.equal(calls[0].url, "/api/auth/admin/users?q=a+b");
  assert.equal((calls[0].init.headers as Record<string, string>)[CSRF_HEADER], undefined);
  assert.deepEqual(calls.slice(1).map((c) => [c.init.method, (c.init.headers as Record<string, string>)[CSRF_HEADER], c.init.body]), [["POST", "1", '{"a":1}'], ["POST", "1", "{}"], ["PUT", "1", '{"b":2}'], ["DELETE", "1", undefined]]);
  assert.equal(calls[4].url, "/api/auth/admin/w");
  assert.equal(api.url("a", { b: 1 }), "/api/auth/a?b=1");
  assert.equal(query(), "");
});

test("api: a 401 renews once through refresh and retries; a second 401 stands; renewal failures stand", async () => {
  const seen: string[] = [];
  let first = true;
  const ok = createApi({ fetch: async (url: string) => (seen.push(url), url === "/api/auth/refresh" ? reply(200, {}) : first ? ((first = false), reply(401, { error: "not_authenticated" })) : reply(200, { n: 1 })) });
  assert.deepEqual(await ok.get("me"), { n: 1 });
  assert.deepEqual(seen, ["/api/auth/me", "/api/auth/refresh", "/api/auth/me"]);
  const always = createApi({ fetch: async (url: string) => (url.endsWith("refresh") ? reply(200, {}) : reply(401, { error: "not_authenticated" })) });
  await assert.rejects(always.get("me"), (e: ApiError) => e.kind === "auth");
  const down = createApi({ fetch: async (url: string) => { if (url.endsWith("refresh")) throw new Error("offline"); return reply(401, {}); } });
  await assert.rejects(down.get("me"), (e: ApiError) => e.kind === "auth");
  const refused = createApi({ fetch: async (url: string) => (url.endsWith("refresh") ? reply(401, {}) : reply(401, {})) });
  await assert.rejects(refused.get("me"), (e: ApiError) => e.status === 401);
  const net = createApi({ fetch: async () => { throw new Error("no route"); } });
  await assert.rejects(net.get("x"), (e: ApiError) => e.kind === "network" && e.detail === "Error: no route".slice(7));
  const odd = createApi({ fetch: async () => { throw "weird"; } });
  await assert.rejects(odd.get("x"), (e: ApiError) => e.detail === "weird");
  const notJson = createApi({ fetch: async () => reply(502, "<html>", "text/html") });
  await assert.rejects(notJson.get("x"), (e: ApiError) => e.kind === "server" && e.code === null);
  const broken = createApi({ fetch: async () => reply(500, "{not json") });
  await assert.rejects(broken.get("x"), (e: ApiError) => e.kind === "server");
  const empty = createApi({ fetch: async () => new Response(null, { status: 200 }) });
  assert.equal(await empty.get("x"), null);
});

test("api: classify turns answers into kinds the views can word", () => {
  const k = (s: number, b: unknown, ra?: string) => classify(s, b, ra ?? null);
  assert.equal(k(403, { error: "step_up_required", stepUpUrl: "/auth/mfa/step-up", minutes: 10 }).kind, "stepup");
  assert.deepEqual([k(403, { error: "forbidden", permission: "auth:admin" }).kind, k(403, { error: "forbidden", permission: "auth:admin" }).permission], ["permission", "auth:admin"]);
  assert.deepEqual(k(403, { error: "escalation", permissions: ["*"] }).permissions, ["*"]);
  assert.equal(k(403, { error: "csrf_check_failed" }).kind, "forbidden");
  assert.equal(k(409, { error: "owner_required", detail: "d" }).detail, "d");
  assert.equal(k(503, { error: "not_configured", hint: "h" }).detail, "h");
  assert.equal(k(429, {}, "12").retryAfter, 12);
  assert.equal(k(429, {}, "x").retryAfter, null);
  assert.deepEqual([400, 404, 418, 500].map((s) => k(s, null).kind), ["invalid", "notfound", "invalid", "server"]);
  assert.equal(k(400, [1]).code, null);
  assert.ok(new ApiError("x", 1, null, null) instanceof Error);
  assert.equal(new ApiError("x", 1, "c", null).message, "c");
});

test("router: parse, href, isRoute", () => {
  assert.deepEqual(parse("#/people/a%20b?status=blocked"), { name: "person", params: { id: "a b" }, query: { status: "blocked" }, section: "people" });
  assert.equal(parse("").name, "home");
  assert.equal(parse("#/").name, "home");
  assert.equal(parse("#/status").section, "status");
  assert.equal(parse("#/nowhere/x/y").name, "notFound");
  assert.equal(parse("#/people/%E0%A4").name, "notFound");
  assert.equal(parse(undefined as never).name, "home");
  assert.equal(href("person", { id: "a/b" }, { tab: "x", none: "", n: null }), "#/people/a%2Fb?tab=x");
  assert.equal(href("people"), "#/people");
  assert.throws(() => href("nope"));
  assert.deepEqual([isRoute(""), isRoute("#"), isRoute("#/x"), isRoute("#main")], [true, true, true, false]);
});

test("i18n: language choice, config block, lookup chain, placeholders, plurals", () => {
  assert.equal(pickLang(["de-AT", "pl"]), "de");
  assert.equal(pickLang(["xx", null]), "en");
  assert.deepEqual(readConfig('{"lang":"pl"}'), { lang: "pl" });
  for (const bad of ["[1]", "null", "{", "", undefined]) assert.deepEqual(readConfig(bad as never), {});
  const i = createI18n("pl", { en: { a: "A {x}", b: "B", n: { one: "{count} thing", other: "{count} things" } }, pl: { a: "Ą {x} {y}", n: { one: "{count} rzecz", few: "{count} rzeczy", many: "{count} rzeczy", other: "{count} rzeczy" } } }, (n: number) => `#${n}`);
  assert.equal(i.t("a", { x: 1 }), "Ą 1 {y}");
  assert.equal(i.t("b"), "B");
  assert.equal(i.t("zzz"), "zzz");
  assert.equal(i.t("n", { count: 3 }), "#3 rzeczy");
  assert.equal(i.t("n", { count: 1 }), "#1 rzecz");
  assert.deepEqual([i.has("b"), i.has("zzz")], [true, false]);
  const en = createI18n("en", { en: { n: { other: "{count}" } } });
  assert.equal(en.t("n", { count: 1 }), "1", "a missing form falls back to other");
});

test("format: numbers, dates, relative times, lists, UTC days", () => {
  const f = createFormat("en", "UTC");
  assert.equal(f.number(1234), "1,234");
  assert.equal(f.number("x"), "0");
  assert.equal(f.date("2026-11-14T10:00:00Z"), "Nov 14, 2026");
  assert.equal(f.dateTime("bad"), "—");
  assert.equal(f.date(null as never), "—");
  assert.match(f.dateTime("2026-11-14T10:00:00Z"), /Nov 14, 2026/);
  const now = Date.parse("2026-11-14T10:00:00Z");
  assert.deepEqual(["2026-11-14T09:59:30Z", "2026-11-14T09:30:00Z", "2026-11-14T07:00:00Z", "2026-11-11T10:00:00Z", "x"].map((t) => f.relative(t, now)), ["30 seconds ago", "30 minutes ago", "3 hours ago", "3 days ago", "—"]);
  assert.ok(f.relative("2026-11-14T10:00:00Z"));
  assert.equal(f.list(["a", "b", "c"]), "a, b, and c");
  assert.equal(createFormat("de").locale, "de");
  assert.equal(isoDay(now), "2026-11-14");
});

test("prefs: storage that works, is missing or throws; theme on <html>", () => {
  const mem = new Map<string, string>();
  const store = { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => void mem.set(k, v), removeItem: (k: string) => void mem.delete(k) };
  assert.equal(readTheme(store), "auto");
  assert.ok(writeStored(store, THEME_KEY, "dark"));
  assert.equal(readTheme(store), "dark");
  assert.ok(writeStored(store, THEME_KEY, null));
  assert.equal(readStored(store, THEME_KEY), null);
  writeStored(store, THEME_KEY, "purple");
  assert.equal(readTheme(store), "auto");
  const broken = { getItem: () => { throw new Error("blocked"); }, setItem: () => { throw new Error("blocked"); } };
  assert.equal(readStored(broken, LANG_KEY), null);
  assert.equal(readStored(null, LANG_KEY), null);
  assert.equal(writeStored(broken, LANG_KEY, "pl"), false);
  const attrs = new Map<string, string>();
  const root = { setAttribute: (k: string, v: string) => void attrs.set(k, v), removeAttribute: (k: string) => void attrs.delete(k) };
  assert.equal(applyTheme(root, "dark", false), "dark");
  assert.equal(attrs.get("data-theme"), "dark");
  assert.equal(applyTheme(root, "auto", true), "dark");
  assert.equal(applyTheme(root, "auto", false), "light");
  assert.equal(attrs.has("data-theme"), false);
});

test("vnode: a tree, text only, props wired, mounted", () => {
  const tree = h("div", null, "a", [h("b", { id: "x" }, 1), null, false, true, undefined]);
  assert.equal(text(tree), "a1");
  assert.equal(find(tree, (n: { tag: string }) => n.tag === "b")?.tag, "b");
  assert.equal(find(tree, () => false), null);
  assert.equal(find("leaf" as never, () => true), null);
  assert.equal(findAll(tree, (n: unknown) => typeof n === "object").length, 2);
  assert.deepEqual(findAll("leaf" as never, () => true), []);
  assert.notEqual(uid("a"), uid("a"));
  const made: Record<string, unknown>[] = [];
  const el = (tag: string, ns: string | null = null) => {
    const e: Record<string, unknown> = { tag, ns, attrs: {} as Record<string, string>, kids: [] as unknown[], on: {} as Record<string, unknown>, style: { props: {} as Record<string, string>, setProperty(k: string, v: string) { (this.props as Record<string, string>)[k] = v; } } };
    Object.assign(e, { setAttribute: (k: string, v: string) => ((e.attrs as Record<string, string>)[k] = v), addEventListener: (k: string, f: unknown) => ((e.on as Record<string, unknown>)[k] = f), appendChild: (c: unknown) => (e.kids as unknown[]).push(c), replaceChildren: (...c: unknown[]) => (e.kids = c) });
    made.push(e);
    return e;
  };
  const doc = { createTextNode: (t: string) => ({ t }), createElement: (t: string) => el(t), createElementNS: (ns: string, t: string) => el(t, ns) };
  let got: unknown = null;
  const out = render(h("button", { on: { click: () => 1 }, ref: (e: unknown) => (got = e), style: { width: "1px" }, value: "v", disabled: true, hidden: false, title: null, n: 0 }, "<i>", h("svg", null, h("path", null))), doc) as Record<string, unknown>;
  assert.equal(got, out);
  assert.deepEqual(out.attrs, { disabled: "", n: "0" });
  assert.equal(out.value, "v");
  assert.deepEqual((out.style as { props: unknown }).props, { width: "1px" });
  assert.deepEqual((out.kids as unknown[])[0], { t: "<i>" });
  assert.equal(((out.kids as Record<string, unknown>[])[1].kids as Record<string, unknown>[])[0].ns, "http://www.w3.org/2000/svg");
  const parent = el("div");
  mount(parent, [h("p", null), null, "x"], doc);
  assert.equal((parent.kids as unknown[]).length, 2);
});
