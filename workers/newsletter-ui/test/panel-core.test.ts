// GO-128: the panel's core modules — DOM building, the fetch wrapper,
// routing and strings (formatting, storage and timers: panel-tasks.test.ts).
import { test } from "node:test";
import assert from "node:assert/strict";
import { find, findAll, h, mount, render, text, uid } from "../public/js/core/vnode.js";
import { ApiError, CSRF_HEADER, classify, createApi, query } from "../public/js/core/api.js";
import { href, isRoute, parse } from "../public/js/core/router.js";
import { createI18n, pickLang, readConfig } from "../public/js/core/i18n.js";
import { fakeDoc } from "./panel-dom.ts";

test("vnode: h flattens and drops empty children; text/find/findAll read the tree", () => {
  const tree = h("div", null, "a", [h("b", { id: "x" }, 1), null, false, true, undefined], [[h("i", null, "c")]]);
  assert.equal(tree.children.length, 3);
  assert.equal(text(tree), "a1c");
  assert.equal(find(tree, (n) => n.tag === "i")?.tag, "i");
  assert.equal(find(tree, (n) => n.tag === "p"), null);
  assert.equal(findAll(tree, (n) => typeof n === "object").length, 3);
  assert.equal(find("leaf" as never, () => true), null);
  assert.deepEqual(findAll("leaf" as never, () => true), []);
  assert.notEqual(uid("a"), uid("a"));
});

test("vnode: render escapes by construction and wires props", () => {
  const doc = fakeDoc();
  let clicked = 0;
  let ref = null;
  const el = render(
    h(
      "button",
      { class: "btn", on: { click: () => clicked++ }, ref: (e: unknown) => (ref = e), style: { width: "3px" }, value: "v", checked: true, disabled: true, hidden: false, title: null, "data-n": 0 },
      "<b>not html</b>",
      h("svg", { viewBox: "0 0 1 1" }, h("rect", { width: 1 })),
    ),
    doc as never,
  ) as never as Record<string, unknown> & { attrs: Record<string, string>; children: unknown[]; listeners: Record<string, (() => void)[]> };
  assert.equal(el.attrs.class, "btn");
  assert.equal(el.attrs.disabled, "");
  assert.ok(!("hidden" in el.attrs) && !("title" in el.attrs));
  assert.equal(el.attrs["data-n"], "0");
  assert.equal(el.value, "v");
  assert.equal(el.checked, true);
  assert.equal((el.style as Record<string, string>).width, "3px");
  assert.equal(ref, el);
  el.listeners.click[0]();
  assert.equal(clicked, 1);
  assert.deepEqual(el.children[0], { text: "<b>not html</b>" }, "a text node, never parsed");
  const svg = el.children[1] as { ns: string; children: { ns: string }[] };
  assert.equal(svg.ns, "http://www.w3.org/2000/svg");
  assert.equal(svg.children[0].ns, "http://www.w3.org/2000/svg", "children inherit the namespace");
  const parent = doc.createElement("div");
  mount(parent as never, [h("p", null, "x"), null, false, ["y"]], doc as never);
  assert.equal(parent.children.length, 2);
});

test("api: CSRF header on every state-changing request, none on GET; JSON in and out", async () => {
  const calls: { url: string; init: RequestInit }[] = [];
  const reply = (status: number, body: unknown, headers: Record<string, string> = { "content-type": "application/json" }) =>
    new Response(body === null ? null : JSON.stringify(body), { status, headers });
  let next = () => reply(200, { ok: true });
  const api = createApi({ base: "https://example.com/newsletter-admin/", fetch: async (url: string, init: RequestInit) => (calls.push({ url, init }), next()) });
  assert.deepEqual(await api.get("subscribers", { status: "pending", q: "", tag: null }), { ok: true });
  assert.equal(calls[0].url, "https://example.com/newsletter-admin/api/subscribers?status=pending");
  assert.equal((calls[0].init.headers as Record<string, string>)[CSRF_HEADER], undefined);
  await api.post("/campaigns", { subject: "x" });
  await api.post("campaigns/1/pause");
  await api.patch("campaigns/1", { a: 1 });
  await api.del("suppressions/ab", { confirm: true });
  for (const c of calls.slice(1)) {
    assert.equal((c.init.headers as Record<string, string>)[CSRF_HEADER], "1", c.init.method);
    assert.equal((c.init.headers as Record<string, string>)["content-type"], "application/json");
  }
  assert.equal(calls[2].init.body, "{}");
  await api.request("POST", "import", { raw: "a,b", type: "text/csv" });
  assert.equal((calls[5].init.headers as Record<string, string>)["content-type"], "text/csv");
  await api.request("DELETE", "x");
  assert.equal(calls[6].init.body, undefined);
  next = () => reply(204, null);
  assert.equal(await api.del("campaigns/1"), null);
  next = () => new Response("plain", { status: 200 });
  assert.equal(await api.get("x"), null, "a non-JSON answer reads as null");
  next = () => new Response(new Uint8Array([1]), { status: 200 });
  assert.equal(await api.get("x"), null, "no content type at all");
  assert.equal(api.url("subscribers.csv", { status: "all" }), "https://example.com/newsletter-admin/api/subscribers.csv?status=all");
});

test("api: failures become ApiError with a kind the views can explain", async () => {
  const fail = async (status: number, body: unknown, headers: Record<string, string> = {}) => {
    const api = createApi({ base: "https://example.com/b/", fetch: async () => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } }) });
    return api.get("x").then(() => assert.fail("should throw"), (e) => e as ApiError);
  };
  assert.equal((await fail(401, { error: "unauthorized" })).kind, "auth");
  assert.equal((await fail(403, { error: "missing x-newsletter-admin: 1 header" })).kind, "csrf");
  assert.equal((await fail(403, { error: "forbidden" })).kind, "forbidden");
  const rate = await fail(429, { error: "test_limit", detail: "at most 10" }, { "retry-after": "3600" });
  assert.deepEqual([rate.kind, rate.code, rate.detail, rate.retryAfter], ["rate", "test_limit", "at most 10", 3600]);
  const install = await fail(503, { error: "newsletter not installed", hint: "deploy it" });
  assert.deepEqual([install.kind, install.detail], ["install", "deploy it"]);
  assert.equal((await fail(503, { error: "no_site_url" })).kind, "config");
  assert.equal((await fail(404, { error: "not_found" })).kind, "notfound");
  assert.equal((await fail(409, { error: "invalid_transition" })).kind, "conflict");
  assert.equal((await fail(413, { error: "too_large" })).kind, "invalid");
  assert.equal((await fail(500, { error: "internal error" })).kind, "server");
  assert.equal((await fail(418, null)).kind, "invalid");
  const broken = createApi({ base: "https://example.com/", fetch: async () => new Response("{oops", { status: 502, headers: { "content-type": "application/json" } }) });
  const e = await broken.get("x").catch((x) => x);
  assert.deepEqual([e.kind, e.code, e.retryAfter], ["server", null, null]);
  const offline = createApi({ base: "https://example.com/", fetch: async () => Promise.reject(new TypeError("Failed to fetch")) });
  const n = await offline.get("x").catch((x) => x);
  assert.deepEqual([n.kind, n.detail, n.message], ["network", "Failed to fetch", "Failed to fetch"]);
  const odd = createApi({ base: "https://example.com/", fetch: async () => Promise.reject("down") });
  assert.equal((await odd.get("x").catch((x) => x)).detail, "down");
  assert.equal(new ApiError("server", 500, null, null).message, "server");
  assert.equal(classify(400, { error: "invalid_body" }, null).message, "invalid_body");
  assert.equal(query(), "");
});

test("api: the default fetch is the global one", async () => {
  const original = globalThis.fetch;
  globalThis.fetch = (async () => new Response('{"v":1}', { headers: { "content-type": "application/json" } })) as typeof fetch;
  try {
    assert.deepEqual(await createApi({ base: "https://example.com/" }).get("x"), { v: 1 });
  } finally {
    globalThis.fetch = original;
  }
});

test("router: hash ↔ view, deep links, sections, unknown paths", () => {
  assert.deepEqual(parse(""), { name: "dashboard", params: {}, query: {}, section: "dashboard" });
  assert.equal(parse("#/").name, "dashboard");
  assert.deepEqual(parse("#/subscribers/a%2Fb?status=pending"), { name: "subscriber", params: { id: "a/b" }, query: { status: "pending" }, section: "subscribers" });
  assert.equal(parse("#/campaigns/new").name, "campaignNew");
  assert.equal(parse("#/campaigns/new").section, "campaigns");
  assert.equal(parse("#/campaigns/abc").params.id, "abc");
  assert.equal(parse("#/campaigns/%E0%A4").name, "notFound", "a malformed escape is no id");
  assert.equal(parse("#/nope").name, "notFound");
  assert.equal(parse(undefined as never).name, "dashboard");
  assert.equal(href("subscriber", { id: "a/b" }, { tab: "x", empty: "", none: null }), "#/subscribers/a%2Fb?tab=x");
  assert.equal(href("dashboard"), "#/");
  assert.throws(() => href("nope"));
  assert.equal(isRoute("#main"), false);
  assert.ok(isRoute("") && isRoute("#") && isRoute("#/x"));
});

test("i18n: language choice, config block, lookup chain, plurals, placeholders", () => {
  assert.equal(pickLang(["de-AT", "en"]), "de");
  assert.equal(pickLang([null, "pt-BR", "hi_IN"]), "hi");
  assert.equal(pickLang(["xx"]), "en");
  assert.deepEqual(readConfig('{"lang":"pl"}'), { lang: "pl" });
  assert.deepEqual(readConfig("[1]"), {});
  assert.deepEqual(readConfig("{nope"), {});
  assert.deepEqual(readConfig(undefined), {});
  assert.deepEqual(readConfig("null"), {});
  const tables = { en: { a: "A {x}", n: { one: "{count} item", other: "{count} items" }, only: "en only" }, pl: { a: "P {x}", n: { one: "{count} rzecz", few: "{count} rzeczy", many: "{count} rzeczy!", other: "{count} r." } } };
  const pl = createI18n("pl", tables, { pl: { a: "override {x}" } }, (n: number) => `#${n}`);
  assert.equal(pl.t("a", { x: 1 }), "override 1");
  assert.equal(pl.t("only"), "en only");
  assert.equal(pl.t("missing"), "missing");
  assert.equal(pl.t("n", { count: 1 }), "#1 rzecz");
  assert.equal(pl.t("n", { count: 3 }), "#3 rzeczy");
  assert.equal(pl.t("n", { count: 5 }), "#5 rzeczy!");
  assert.equal(pl.t("a", {}), "override {x}", "an unknown placeholder stays visible");
  assert.ok(pl.has("only") && !pl.has("missing"));
  const en = createI18n("en", { en: { n: { other: "{count} things" } } });
  assert.equal(en.t("n", { count: 1 }), "1 things", "falls back to other");
  assert.equal(en.lang, "en");
});
