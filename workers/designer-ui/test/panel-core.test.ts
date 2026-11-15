// GO-179: the panel's generic core (public/js/core/) — router, API wrapper,
// session renewal, preferences, strings — and the revision restore model.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ApiError, classify, createApi, query } from "../public/js/core/api.js";
import { LANGS, createI18n, pickLang, readConfig } from "../public/js/core/i18n.js";
import { applyTheme, createPrefs } from "../public/js/core/prefs.js";
import { createRouter, isRoute } from "../public/js/core/router.js";
import { readMe, renew } from "../public/js/core/session.js";
import { leaves, lookup, restoreChanges } from "../public/js/models/revision-restore.js";

test("router: parse, href, sections, bad escapes", () => {
  const r = createRouter([["home", ""], ["group", "options/:group"], ["files", "files"]], { group: "home" });
  assert.deepEqual(r.parse("#/options/site?q=a"), { name: "group", params: { group: "site" }, query: { q: "a" }, section: "home" });
  assert.equal(r.parse("").name, "home");
  assert.equal(r.parse("#/nothing/here/x").name, "notFound");
  assert.equal(r.parse("#/options/%E0%A4%A").name, "notFound");
  assert.equal(r.href("group", { group: "a b" }, { q: "x", empty: "", none: null }), "#/options/a%20b?q=x");
  assert.equal(r.href("files"), "#/files");
  assert.throws(() => r.href("nope"));
  assert.ok(isRoute("") && isRoute("#") && isRoute("#/x") && !isRoute("#main"));
  assert.equal(createRouter([["a", "a"]]).parse("#/a").section, "a");
});

const res = (status: number, body: unknown, headers: Record<string, string> = { "content-type": "application/json" }) => new Response(body === undefined ? null : JSON.stringify(body), { status, headers });

test("api: requests, errors by kind, one renewal on 401", async () => {
  const calls: { url: string; init: RequestInit }[] = [];
  const answers = [res(200, { ok: 1 }), res(401, { error: "unauthorized" }), res(200, { again: 1 }), res(503, { error: "auth_not_configured", detail: "x" })];
  const api = createApi({ base: "https://example.com/designer/", fetch: async (url: string, init: RequestInit) => (calls.push({ url, init }), answers.shift()!), onUnauthorized: async () => true, csrfHeader: "x-csrf" });
  assert.deepEqual(await api.get("settings", { path: "a b", cursor: null }), { ok: 1 });
  assert.equal(calls[0].url, "https://example.com/designer/api/settings?path=a+b");
  assert.deepEqual(await api.post("settings/publish", { a: 1 }), { again: 1 });
  assert.equal((calls[1].init.headers as Record<string, string>)["x-csrf"], "1");
  assert.equal(calls[1].init.body, '{"a":1}');
  await assert.rejects(api.post("x"), (e: ApiError) => e.kind === "config" && e.code === "auth_not_configured" && e.detail === "x");
  const down = createApi({ base: "https://example.com/", fetch: async () => { throw new Error("offline"); } });
  await assert.rejects(down.get("me"), (e: ApiError) => e.kind === "network" && e.detail === "offline");
  const thrown = createApi({ base: "https://example.com/", fetch: async () => { throw "plain"; } });
  await assert.rejects(thrown.get("me"), (e: ApiError) => e.detail === "plain");
  const bare = createApi({ base: "https://example.com/", fetch: async () => new Response("x", { status: 502 }) });
  await assert.rejects(bare.get("me"), (e: ApiError) => e.kind === "server");
  const text = createApi({ base: "https://example.com/", fetch: async () => res(500, undefined, { "content-type": "text/plain" }) });
  await assert.rejects(text.get("me"), (e: ApiError) => e.kind === "server" && e.code === null);
  const bad = createApi({ base: "https://example.com/", fetch: async () => new Response("{", { status: 200, headers: { "content-type": "application/json" } }) });
  assert.equal(await bad.get("me"), null);
  const no = createApi({ base: "https://example.com/", fetch: async () => res(401, {}), onUnauthorized: async () => false });
  await assert.rejects(no.get("me"), (e: ApiError) => e.kind === "auth");
  assert.equal(api.url("x"), "https://example.com/designer/api/x");
  assert.ok(globalThis.fetch && createApi({ base: "https://example.com/" }).url("y").endsWith("/api/y"));
});

test("classify and query", () => {
  assert.equal(classify(403, { error: "forbidden", permission: "designer-ui:theme" }).permission, "designer-ui:theme");
  assert.equal(classify(403, { error: "cross_origin" }).kind, "forbidden");
  assert.equal(classify(429, null, "12").retryAfter, 12);
  assert.equal(classify(429, null, null).retryAfter, null);
  assert.equal(classify(422, { error: "invalid_settings" }).kind, "invalid");
  assert.equal(classify(418, null).kind, "invalid");
  assert.equal(classify(409, { error: "conflict", detail: 3 }).detail, null);
  assert.equal(new ApiError("network", 0, null, null).message, "network");
  assert.equal(query({}), "");
});

test("session: readMe and renew", async () => {
  let reloaded = 0;
  let assigned = "";
  const location = { reload: () => reloaded++, assign: (u: string) => (assigned = u), pathname: "/designer/", search: "", hash: "#/files", origin: "https://example.com" };
  const fallback = { permissions: [] };
  assert.deepEqual(await readMe({ get: async () => ({ user: 1 }) }, location, fallback), { user: 1 });
  assert.equal(await readMe({ get: async () => { throw new ApiError("auth", 401, null, null); } }, location, fallback), fallback);
  assert.equal(reloaded, 1);
  assert.equal(await readMe({ get: async () => { throw new ApiError("server", 500, null, null); } }, location, fallback), fallback);
  const loginTarget = (me: { loginUrl?: string }, here: string) => (me.loginUrl ? `${me.loginUrl}?next=${encodeURIComponent(here)}` : null);
  assert.equal(await renew({ refreshUrl: "/r" }, { fetch: async () => new Response(null, { status: 204 }), location, loginTarget }), true);
  assert.equal(await renew({ refreshUrl: "/r", loginUrl: "/login" }, { fetch: async () => { throw new Error("x"); }, location, loginTarget }), false);
  assert.equal(assigned, "/login?next=%2Fdesigner%2F%23%2Ffiles");
  assert.equal(await renew({}, { fetch: async () => new Response(null), location, loginTarget }), false);
  assert.equal(reloaded, 2);
});

test("prefs: stored, missing, throwing storage; theme on <html>", () => {
  const m = new Map<string, string>();
  const storage = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) };
  const p = createPrefs(storage, "x");
  assert.equal(p.theme(), "auto");
  assert.ok(p.setTheme("dark"));
  assert.equal(m.get("x-theme"), "dark");
  assert.equal(p.theme(), "dark");
  p.setTheme("auto");
  assert.equal(m.has("x-theme"), false);
  m.set("x-theme", "neon");
  assert.equal(p.theme(), "auto");
  const broken = createPrefs({ getItem: () => { throw new Error(); }, setItem: () => { throw new Error(); } }, "x");
  assert.equal(broken.read("lang"), null);
  assert.equal(broken.write("lang", "pl"), false);
  assert.equal(createPrefs(null, "x").read("lang"), null);
  const attrs = new Map<string, string>();
  const root = { setAttribute: (k: string, v: string) => void attrs.set(k, v), removeAttribute: (k: string) => void attrs.delete(k) };
  assert.equal(applyTheme(root, "light", true), "light");
  assert.equal(attrs.get("data-theme"), "light");
  assert.equal(applyTheme(root, "auto", true), "dark");
  assert.equal(applyTheme(root, "auto", false), "light");
  assert.equal(attrs.has("data-theme"), false);
});

test("i18n: language choice, config block, lookup chain, plurals", () => {
  assert.deepEqual(LANGS, ["en", "pl", "hi", "de", "fr"]);
  assert.equal(pickLang(["pt-BR", "pl-PL"]), "pl");
  assert.equal(pickLang([null, "xx"]), "en");
  assert.deepEqual(readConfig('{"lang":"pl"}'), { lang: "pl" });
  for (const bad of ["[1]", "nope", "null", ""]) assert.deepEqual(readConfig(bad), {});
  const i = createI18n("pl", { en: { a: "A {x}", n: { one: "one", other: "{count} many" } }, pl: { n: { one: "jeden", few: "{count} kilka", other: "{count} inne" } } }, { pl: { o: "over" } }, (n: number) => `#${n}`);
  assert.equal(i.t("a", { x: 1 }), "A 1");
  assert.equal(i.t("a"), "A {x}");
  assert.equal(i.t("n", { count: 1 }), "jeden");
  assert.equal(i.t("n", { count: 3 }), "#3 kilka");
  assert.equal(i.t("n", { count: 5 }), "#5 inne", "many falls back to other");
  assert.equal(i.t("o"), "over");
  assert.equal(i.t("missing"), "missing");
  assert.ok(i.has("a") && !i.has("missing"));
  assert.equal(createI18n("en", { en: { a: "x" } }).t("a", { count: 2 }), "x");
});

test("revision restore: what brings the old overlay back", () => {
  const e = (path: string, value: unknown, source: string, node: Record<string, unknown> = { type: "string" }) => ({ path, segments: path.split("."), value, source, node, class: "plain" });
  const fields = [
    e("title", "Now", "overlay"),
    e("toc", true, "overlay", { type: "bool" }),
    e("feed_items", 20, "file", { type: "int" }),
    e("mddb.lang", "pl", "overlay"),
    e("jwt_secret", { set: true, reference: null }, "file", { type: "string", class: "secret", secretFormat: "whole" }),
    e("description", "d", "default"),
  ];
  const old = { title: "Then", feed_items: 30, mddb: { lang: "pl" }, jwt_secret: { set: true, reference: null } };
  const plan = restoreChanges(fields, old);
  assert.deepEqual(plan.set.map((s: { entry: { path: string }; value: unknown }) => [s.entry.path, s.value]), [["title", "Then"], ["feed_items", 30]]);
  assert.deepEqual(plan.revert.map((x: { path: string }) => x.path), ["toc"]);
  assert.deepEqual(plan.skipped.map((x: { path: string }) => x.path), ["jwt_secret"]);
  assert.deepEqual(restoreChanges(fields, null).revert.length, 3);
  assert.deepEqual(lookup({ a: { b: 1 } }, ["a", "b"]), { has: true, value: 1 });
  assert.deepEqual(lookup({ a: 1 }, ["a", "b"]), { has: false, value: undefined });
  assert.deepEqual(leaves({ a: { b: 1, c: {} }, s: { set: true, reference: "$X" }, l: [1] }), [{ path: "a.b", value: 1 }, { path: "a.c", value: {} }, { path: "s", value: { set: true, reference: "$X" } }, { path: "l", value: [1] }]);
  assert.deepEqual(leaves("x"), []);
});
