// The home page's views, its load order and the chart. Answers come from the
// real core handlers over the demo scenarios (no hand-made shapes), and the
// whole page is rendered through vnode.render into a stand-in DOM: a module's
// markup and script must arrive as text — no element is ever made from it.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as overview from "../functions/api/core/overview.ts";
import * as status from "../functions/api/core/status.ts";
import * as info from "../functions/api/core/info.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { loadAll, pageTree } from "../public/core/js/page.js";
import { attentionSection } from "../public/core/js/views/attention.js";
import { modulesSection } from "../public/core/js/views/modules.js";
import { siteSection } from "../public/core/js/views/site.js";
import { errorBlock, prefs } from "../public/core/js/views/shell.js";
import { ApiError } from "../public/core/js/api.js";
import { createFormat } from "../public/core/js/format.js";
import { createI18n } from "../public/core/js/i18n.js";
import { render } from "../public/core/js/vnode.js";
import en from "../public/core/js/strings/en.js";
import { all, fakeDoc, fire, textOf, walk } from "./dom.ts";
import type { FakeEl } from "./dom.ts";
import { HOSTILE, REGISTRY, SCENARIOS } from "./demo.ts";
import { ORIGIN, PASSWORD, PW_ENV, basic, call, fakeFetch, freshIP } from "./harness.ts";

const fmt = createFormat("en", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);
const calls: string[] = [];
const app = { t, fmt, lang: "en", theme: "auto", config: {}, here: "/core/", now: () => Date.parse("2026-11-13T10:00:00Z"), refresh: () => calls.push("refresh"), setLang: (l: string) => calls.push(`lang:${l}`), setTheme: (v: string) => calls.push(`theme:${v}`) };

// api: the page's api.get, answered by the real handlers of core.
const HANDLERS: Record<string, typeof overview.onRequest> = { overview: overview.onRequest, status: status.onRequest, info: info.onRequest };
const api = (fail: Record<string, ApiError> = {}) => ({
  async get(path: string, params: Record<string, string> = {}) {
    if (fail[path]) throw fail[path];
    const q = new URLSearchParams(params).toString();
    const r = await call(HANDLERS[path], { path: `/api/core/${path}${q ? "?" + q : ""}`, headers: { authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() }, env: { ...PW_ENV, CORE_PROBE_TIMEOUT_MS: "50" } });
    return r.json();
  },
});
function routes(name: string) {
  const out: Record<string, Response | ((s: AbortSignal) => Promise<Response>)> = {};
  for (const [path, a] of Object.entries(SCENARIOS[name]())) {
    const res = () => (a.raw !== undefined ? new Response(a.raw, { status: a.status }) : Response.json(a.body, { status: a.status }));
    // A slow module here simply never answers in time: core's own timeout decides.
    out[ORIGIN + path] = a.delayMs ? (signal) => new Promise((_, reject) => signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")))) : res();
  }
  return out;
}
async function pageFor(name: string, fail: Record<string, ApiError> = {}) {
  const f = fakeFetch(routes(name) as never);
  try {
    const data = await loadAll(api(fail), "en");
    const doc = fakeDoc();
    const root = doc.createElement("div") as FakeEl;
    for (const n of pageTree(app, data, false)) root.appendChild(render(n, doc as never));
    return { data, root, text: textOf(root) };
  } finally {
    f.restore();
  }
}

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
  calls.length = 0;
});
after(() => useRegistry());

test("good: all clear, a card per module (core not among them), the chart, the build, and, with a shared password and two panels, the calm hint about auth", async () => {
  const { root, text } = await pageFor("good");
  assert.match(text, /Nothing needs you right now\./);
  const cards = all(root, (e) => e.tag === "li" && (e.attrs.class ?? "").includes("module"));
  assert.deepEqual(cards.map((c) => c.attrs["aria-labelledby"]), ["mod-comments", "mod-newsletter", "mod-newsletter-ui", "mod-auth", "mod-rate-limit", "mod-kitchen-log"]);
  assert.match(textOf(cards[1]), /Its numbers are shown with Newsletter\./);
  assert.match(textOf(cards[2]), /Last 30 days: .* in total/);
  assert.equal(all(cards[2], (e) => e.tag === "svg").length, 1);
  assert.match(textOf(cards[4]), /Not checked/);
  assert.match(textOf(cards[3]), /No panel/);
  assert.equal(walk(cards[0], (e) => e.tag === "a")!.attrs.href, "/comments-admin.html");
  assert.match(text, /Built with ssg 1\.9\.9/);
  assert.match(text, /A shared password\./);
  assert.match(text, /The auth worker gives each of them their own sign-in/);
  assert.match(text, /7 modules registered by the build/);
});

test("todo: attention first with severity words and links; hostile text inert everywhere; problems listed; build hint", async () => {
  const { root, text } = await pageFor("todo");
  assert.match(text, /2 need action · 2 to check · 1 good to know/);
  const items = all(root, (e) => e.tag === "li" && (e.attrs.class ?? "").startsWith("callout sev-"));
  assert.deepEqual(items.map((i) => i.attrs.class), ["callout sev-action", "callout sev-action", "callout sev-warning", "callout sev-warning", "callout sev-info"]);
  assert.equal(walk(items[1], (e) => e.tag === "a")!.attrs.href, "/newsletter-admin/#/campaigns?status=paused");
  // The hostile module: its markup is text — no img, no script element anywhere in the page.
  assert.ok(text.includes(HOSTILE));
  assert.deepEqual(all(root, (e) => e.tag === "script").length, 0);
  assert.ok(all(root, (e) => e.tag === "img").every((i) => i.attrs.src.startsWith("img/") && !("onerror" in i.attrs)));
  assert.match(text, /TURNSTILE_SECRET is not set/);
  assert.match(text, /Needs configuration/);
  assert.match(text, /Add build_report: build\.json to \.ssg\.yaml/);
});

test("down: unavailable modules say why and what to check; an unreadable summary says so; nothing waits only among those that answered", async () => {
  const { root, text } = await pageFor("down");
  const card = (id: string) => textOf(walk(root, (e) => e.attrs["aria-labelledby"] === `mod-${id}`)!);
  assert.match(card("comments"), /Unavailable.*It answered with an error \(HTTP 500\)\.Check that the worker is deployed/);
  assert.match(card("newsletter-ui"), /Unavailable.*It did not answer/);
  assert.match(card("kitchen-log"), /Its numbers could not be read \(summary/);
  assert.match(text, /Nothing waits among the modules that answered\.3 modules could not report this time/);
  assert.match(card("newsletter"), /Confirmed subscribers/, "newsletter-ui did not answer: newsletter's own numbers show");
});

test("loadAll: overview fails → the page fails with words; status or info failing leave the rest", async () => {
  const signin = await pageFor("good", { overview: new ApiError("signin", 401, "unauthorized", null) });
  assert.match(signin.text, /You are not signed in/);
  assert.equal(walk(signin.root, (e) => e.tag === "a" && e.attrs.class === "btn btn-primary")!.attrs.href, "/auth/login?next=%2Fcore%2F");
  fire(walk(signin.root, (e) => e.tag === "button" && textOf(e) === "Try again")!, "click");
  assert.deepEqual(calls, ["refresh"]);
  const noStatus = await pageFor("good", { status: new ApiError("network", 0, null, "x") });
  assert.match(noStatus.text, /Working|Not checked/);
  assert.equal(noStatus.data.status, null);
  const noInfo = await pageFor("good", { info: new ApiError("config", 503, "registry_missing", "list workers/core") });
  assert.match(noInfo.text, /core is not set up yet: list workers\/core/);
  fire(walk(noInfo.root, (e) => e.tag === "button" && textOf(e) === "Try again")!, "click");
  assert.ok(calls.includes("refresh"));
});

test("pageTree before the first answer: heading and a loading line; busy disables Refresh", () => {
  const doc = fakeDoc();
  const [head, wait] = pageTree(app, null, true).map((n: unknown) => render(n, doc as never) as never as FakeEl);
  const button = walk(head, (e) => e.tag === "button")!;
  assert.deepEqual([button.attrs["aria-busy"], "disabled" in button.attrs, textOf(button)], ["true", true, "Asking the modules…"]);
  assert.equal(wait.attrs.role, "status");
  const idle = render(pageTree(app, null, false)[0], doc as never) as never as FakeEl;
  const refresh = walk(idle, (e) => e.tag === "button")!;
  fire(refresh, "click");
  assert.ok(calls.includes("refresh"));
});

test("prefs: native selects, current values chosen, changes handed to the app", () => {
  const doc = fakeDoc();
  const el = render(prefs({ ...app, lang: "pl", theme: "dark" }), doc as never) as never as FakeEl;
  const [lang, theme] = all(el, (e) => e.tag === "select");
  assert.equal((all(lang, (e) => e.tag === "option" && (e as Record<string, unknown>).selected === true)[0] as Record<string, unknown>).value, "pl");
  assert.equal((all(theme, (e) => e.tag === "option" && (e as Record<string, unknown>).selected === true)[0] as Record<string, unknown>).value, "dark");
  fire(lang, "change", { target: { value: "de" } });
  fire(theme, "change", { target: { value: "light" } });
  assert.deepEqual(calls, ["lang:de", "theme:light"]);
});

test("errorBlock: every kind has words; rate says the wait; unknown errors are the server's", () => {
  const doc = fakeDoc();
  const say = (e: unknown) => textOf(render(errorBlock(app, e, () => {}), doc as never) as never as FakeEl);
  assert.match(say(new ApiError("rate", 429, null, null, 42)), /Wait 42 seconds/);
  assert.match(say(new ApiError("rate", 429, null, null)), /Wait 60 seconds/);
  assert.match(say(new ApiError("config", 503, "signin_not_configured", null)), /signin_not_configured/);
  assert.match(say(new ApiError("missing", 404, null, null)), /no core API/);
  assert.match(say(null), /core failed while answering/);
});

test("attention: more than shown is said; a link core refused is left out; missing data is an empty state", () => {
  const doc = fakeDoc();
  const items = [{ module: "m", title: "M", severity: "warning", text: "x", href: null }];
  const el = render(attentionSection(app, { items, total: 25 }), doc as never) as never as FakeEl;
  assert.match(textOf(el), /And 24 more — the first 1 are shown/);
  assert.equal(all(el, (e) => e.tag === "a").length, 0);
  assert.match(textOf(render(attentionSection(app, { items }, 1), doc as never) as never as FakeEl), /1 module could not report/);
  assert.match(textOf(render(attentionSection(app, undefined), doc as never) as never as FakeEl), /Nothing needs you/);
});

test("module cards for the rarer states; an empty list says why; site facts without a user, version or contract", () => {
  const doc = fakeDoc();
  const scoped = { ...app, titleOf: (id: string) => id };
  const items = [
    { id: "far", title: "Far", panel: null, status: "registered", summary: null },
    { id: "bad", title: "Bad", panel: null, status: "registered", summary: null },
    { id: "shut", title: "Shut", panel: "/s/", status: "registered", summary: { state: "denied" } },
    { id: "odd", title: "Odd", panel: null, status: "registered", summary: { state: "invalid", reason: "not_json" } },
    { id: "moved", title: "Moved", panel: null, status: "registered", summary: { state: "ok", metrics: [], problems: [], series: null, metricsIn: "ghost" } },
  ];
  const status = new Map([
    ["far", { status: "unreachable", reason: "refused_target" }],
    ["bad", { status: "invalid_manifest", reason: "rejected", detail: "no title" }],
  ]);
  const el = render(modulesSection(scoped, items, status), doc as never) as never as FakeEl;
  const card = (id: string) => textOf(walk(el, (e) => e.attrs["aria-labelledby"] === `mod-${id}`)!);
  assert.match(card("far"), /leads off this site.*Rebuild the site/);
  assert.match(card("bad"), /Its manifest is not valid: no title.*Update the module's module\.json/);
  assert.match(card("shut"), /No access.*The module refused this account/);
  assert.match(card("odd"), /Its numbers could not be read \(not_json\)/);
  assert.match(card("moved"), /Its numbers are shown with ghost\./);
  assert.match(textOf(render(modulesSection(scoped, [], new Map()), doc as never) as never as FakeEl), /No module is visible to this account/);
  const link = (canManage: boolean) => walk(render(modulesSection(scoped, [], new Map(), canManage), doc as never) as never as FakeEl, (e) => e.attrs.id === "arrange-link");
  assert.ok(!link(false), "no Arrange link for a caller who may not manage modules");
  assert.ok(link(true), "the Arrange link for one who may");
  const page = pageTree(app, { overview: { items: [items[4]], attention: { items: [], total: 0 } }, status: null, info: { failed: new ApiError("server", 500, null, null) }, error: null }, false);
  assert.match(textOf(render(page[2], doc as never) as never as FakeEl), /shown with ghost/, "an id the page has no title for is shown as the id");
  const info = { mode: "auth", user: null, buildState: "absent", build: null, problems: [], registry: { format: 1, modules: 1 }, modules: [{ id: "m", title: "M", version: null, contract: null, status: "live" }] };
  const site = textOf(render(siteSection(scoped, info, items), doc as never) as never as FakeEl);
  assert.match(site, /The auth worker: everyone signs in/);
  assert.doesNotMatch(site, /Signed in as|More than one person/);
  assert.match(site, /M——Described itself just now/);
  const signedIn = textOf(render(siteSection(scoped, { ...info, user: { id: "u1", email: "ada@example.com" } }, items), doc as never) as never as FakeEl);
  assert.match(signedIn, /Signed in as ada@example\.com\./);
});
