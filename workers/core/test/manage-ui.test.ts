// The Modules view (GO-149) in the browser modules: the decisions
// (js/manage.js), the state and its actions against the real handlers and a
// real-SQL database (js/manage-state.js), and the trees (js/views/manage*.js)
// rendered through vnode into the stand-in DOM — keyboard paths, the live
// announcements, read-only without CORE_DB, data always as text.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as manage from "../functions/api/core/manage/index.ts";
import * as order from "../functions/api/core/manage/order.ts";
import * as exporter from "../functions/api/core/manage/export.ts";
import * as importer from "../functions/api/core/manage/import.ts";
import * as modulePatch from "../functions/api/core/manage/modules/[id].ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { ApiError, classify } from "../public/core/js/api.js";
import { createManage } from "../public/core/js/manage-state.js";
import { changeKey, draftOf, draftPatch, labelsText, moveOrder, previewKey, readOnlyKey, statusLine, tooLarge, whoKey } from "../public/core/js/manage.js";
import { manageView } from "../public/core/js/views/manage.js";
import { pageTree } from "../public/core/js/page.js";
import { routeOf } from "../public/core/js/route.js";
import { createFormat } from "../public/core/js/format.js";
import { createI18n } from "../public/core/js/i18n.js";
import { render } from "../public/core/js/vnode.js";
import en from "../public/core/js/strings/en.js";
import { all, fakeDoc, fire, textOf, walk } from "./dom.ts";
import type { FakeEl } from "./dom.ts";
import { PASSWORD, PW_ENV, basic, call, freshIP } from "./harness.ts";
import { fakeD1 } from "./d1.ts";
import { useSite } from "./manage-fixture.ts";

const fmt = createFormat("en", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);
const ROUTES: Record<string, Parameters<typeof call>[0]> = { manage: manage.onRequest, "manage/order": order.onRequest, "manage/export": exporter.onRequest, "manage/import": importer.onRequest };

// api: the page's api, answered by core's real handlers (errors as api.js classifies them).
function api(env: Record<string, unknown>) {
  const ask = async (method: string, path: string, body?: unknown, params: Record<string, string> = {}) => {
    const q = new URLSearchParams(params).toString();
    const handler = path.startsWith("manage/modules/") ? modulePatch.onRequest : ROUTES[path];
    const headers: Record<string, string> = { authorization: basic(PASSWORD), "cf-connecting-ip": freshIP(), "x-core-admin": "1" };
    const r = await call(handler, { method, path: `/api/core/${path}${q ? "?" + q : ""}`, env, headers, body: body === undefined ? undefined : typeof body === "string" ? body : JSON.stringify(body) });
    const data = JSON.parse(r.body);
    if (r.status >= 400) throw classify(r.status, data, new Headers(r.headers));
    return data;
  };
  return { get: (path: string, params?: Record<string, string>) => ask("GET", path, undefined, params), send: (m: string, p: string, b: unknown, params?: Record<string, string>) => ask(m, p, b, params) };
}

function setup(env: Record<string, unknown>) {
  const said: string[] = [];
  let draws = 0;
  let changed = 0;
  const app: Record<string, any> = { t, fmt, lang: "en", theme: "auto", config: {}, here: "/core/", route: "modules", setLang() {}, setTheme() {}, refresh() {} };
  app.manage = createManage({ api: api(env), app, redraw: () => void draws++, announce: (s: string) => void said.push(s), changed: () => void changed++ });
  const draw = () => {
    const doc = fakeDoc();
    const root = doc.createElement("div") as FakeEl;
    for (const n of [pageTree(app, null, false)].flat(Infinity).filter(Boolean)) root.appendChild(render(n, doc as never));
    return { root, text: textOf(root), doc };
  };
  return { app, said, draw, draws: () => draws, changed: () => changed };
}
const byId = (root: FakeEl, id: string) => walk(root, (e) => e.attrs.id === id)!;

beforeEach(useSite);
after(() => useRegistry());

test("decisions: moving, read-only reasons, statuses, who, drafts, previews, size", () => {
  assert.deepEqual([moveOrder(["a", "b", "c"], "b", -1), moveOrder(["a", "b", "c"], "b", 1), moveOrder(["a"], "a", -1), moveOrder(["a"], "a", 1), moveOrder(["a"], "x", 1)], [["b", "a", "c"], ["a", "c", "b"], null, null, null]);
  assert.deepEqual([readOnlyKey({ writable: true }), readOnlyKey({ writable: false, readOnly: "no_database" }), readOnlyKey({ writable: false, readOnly: "database_error" })], [null, "manage.readonly.no_database", "manage.readonly.database_error"]);
  assert.deepEqual([statusLine({ status: null }), statusLine({ status: { status: "unreachable", at: "x" } }), statusLine({ status: { status: "reachable", at: "y" } })], [null, { key: "manage.status.down", at: "x" }, { key: "manage.status.up", at: "y" }]);
  assert.deepEqual([whoKey("password"), whoKey("access"), whoKey("auth"), whoKey("ada@example.com")], ["manage.who.password", "manage.who.access", "manage.who.auth", null]);
  assert.deepEqual([changeKey({ action: "order" }), changeKey({ action: "later" })], ["manage.change.order", "manage.change.other"]);
  const item = { labels: { en: "Shop", es: "Tienda" }, note: "n" };
  const d = draftOf(item);
  assert.deepEqual(d, { labels: { en: "Shop", pl: "", hi: "", de: "", fr: "" }, note: "n" });
  assert.equal(draftPatch(item, d), null);
  assert.deepEqual(draftPatch(item, { labels: { ...d.labels, en: " ", pl: " Sklep " }, note: " m " }), { labels: { es: "Tienda", pl: "Sklep" }, note: "m" }, "other languages stay, empty removes, text is trimmed");
  assert.deepEqual(draftPatch({ labels: {}, note: "" }, { labels: {}, note: "" }), null);
  assert.deepEqual([labelsText({ en: "A", pl: "B" }), labelsText({}), labelsText(undefined)], ["en: A, pl: B", null, null]);
  assert.deepEqual(
    [
      { field: "hidden", to: true }, { field: "hidden", to: false }, { field: "position", to: 2 }, { field: "position", to: null },
      { field: "labels", to: { en: "A" } }, { field: "labels", to: {} }, { field: "note", to: "x" }, { field: "note", to: "" },
    ].map(previewKey),
    ["manage.import.hide", "manage.import.show", "manage.import.place", "manage.import.unplace", "manage.import.labels", "manage.import.nolabels", "manage.import.note", "manage.import.nonote"],
  );
  assert.deepEqual([tooLarge({ size: 10 }, 5), tooLarge({ size: 5 }, 5), tooLarge(null, 5)], [true, false, false]);
  assert.deepEqual([routeOf("#modules"), routeOf("#appearance"), routeOf("#constructor"), routeOf("")], ["modules", "appearance", "home", "home"]);
  assert.deepEqual([classify(409, null, new Headers()).kind, classify(422, null, new Headers()).kind, classify(413, null, new Headers()).kind], ["conflict", "invalid", "invalid"]);
});

test("without CORE_DB: read-only with the reason and how to enable it; every control disabled; nothing can be sent", async () => {
  const s = setup(PW_ENV);
  assert.match(s.draw().text, /Asking the modules/, "loading first");
  await s.app.manage.load();
  const { root, text } = s.draw();
  assert.match(text, /Read-only/);
  assert.match(text, /bind it to core as CORE_DB/);
  assert.match(text, /Downloading and importing need core's database/);
  for (const e of all(root, (x) => ["input", "button", "textarea"].includes(x.tag) && !/^pref-/.test(x.attrs.id ?? ""))) assert.ok(e.disabled === true || "disabled" in e.attrs, e.attrs.id);
  assert.ok(!walk(root, (e) => e.attrs.id === "import-file"), "no file input");
  assert.match(text, /Hidden or not, its panel still works at \/comments-admin\.html\./);
  assert.match(text, /Remove it\./, "the manifest's own words on switching it off");
  assert.match(text, /Remove it from workers: \(or modules:\)/, "a module without a manifest gets the general sentence");
  assert.match(text, /ssg new worker contact-form/);
  assert.match(text, /modules:\n  - id: newsletter-ui\n    url: \/newsletter-ui/);
});

test("switch, move by keyboard with announcements, save a name and note — each logged and shown", async () => {
  const db = fakeD1();
  const s = setup({ ...PW_ENV, CORE_DB: db });
  await s.app.manage.load();
  let { root } = s.draw();
  assert.equal(byId(root, "up-blog").attrs["aria-disabled"], "true", "first: up stays focusable, aria-disabled");
  fire(byId(root, "up-blog"), "click");
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(s.said.at(-1), "blog title is already first.");

  fire(byId(root, "down-blog"), "click");
  await waitFor(() => s.said.at(-1)?.startsWith("blog title moved"));
  assert.equal(s.said.at(-1), "blog title moved to position 2 of 5.");
  assert.equal(s.changed(), 1, "the bar is told to ask again");
  ({ root } = s.draw());
  assert.deepEqual(all(root, (e) => e.tag === "li" && (e.attrs.class ?? "").includes("manage-item")).map((e) => e.attrs["aria-labelledby"]), ["manage-comments", "manage-blog", "manage-core", "manage-shop", "manage-web-push"]);
  assert.equal(byId(root, "down-web-push").attrs["aria-disabled"], "true");
  fire(byId(root, "down-web-push"), "click");
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(s.said.at(-1), "web-push title is already last.");

  const hide = byId(root, "hide-shop");
  assert.equal(hide.attrs.role, "switch");
  assert.equal(hide.attrs["aria-describedby"], "cosmetic-shop");
  fire(hide, "change", { target: { checked: true } });
  await waitFor(() => s.said.at(-1)?.includes("hidden"));
  assert.equal(s.said.at(-1), "shop is now hidden from the navigation and the home page.");
  ({ root } = s.draw());
  assert.equal(byId(root, "hide-shop").checked, true);
  assert.match(textOf(byId(root, "manage-shop").ownerDocument ? root : root), /Hidden/);

  fire(byId(root, "label-comments-pl"), "input", { target: { value: "Komentarze" } });
  fire(byId(root, "note-comments"), "input", { target: { value: "Moderated by <b>Ada</b>" } });
  const box = walk(root, (e) => e.tag === "details" && (e.attrs.class ?? "").includes("edit-box") && textOf(e).includes("comments title"))!;
  fire(box, "toggle", { target: { open: true } });
  fire(walk(box, (e) => e.tag === "form")!, "submit");
  await waitFor(() => s.said.at(-1)?.startsWith("Saved"));
  assert.equal(s.said.at(-1), "Saved the name and note of comments title.");
  ({ root } = s.draw());
  assert.ok("open" in walk(root, (e) => e.tag === "details" && (e.attrs.class ?? "").includes("edit-box") && textOf(e).includes("comments title"))!.attrs, "the box stays open after the redraw");
  const text = textOf(root);
  assert.match(text, /Moderated by <b>Ada<\/b>/, "a note is text, never markup");
  assert.ok(!all(root, (e) => e.tag === "b").length);
  assert.match(text, /someone with the shared password edited the note on comments title\./);
  assert.match(text, /someone with the shared password hid shop\./);
  assert.match(text, /someone with the shared password changed the order\./);
  assert.match(text, /Last changed by someone with the shared password on/);
  fire(walk(root, (e) => e.tag === "form")!, "submit");
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(s.said.at(-1), "Nothing to save: the name and note are as they were.");

  s.app.lang = "pl";
  await s.app.manage.load();
  assert.match(s.draw().text, /Komentarze \(comments title \(pl\)\)/, "the label for the language, the module's own name beside it");
});

test("import: a file too large, a refused file, a preview, apply, and a stale preview", async () => {
  const db = fakeD1();
  const env = { ...PW_ENV, CORE_DB: db };
  const s = setup(env);
  await s.app.manage.load();
  const file = (text: string) => ({ size: text.length, text: async () => text });
  await s.app.manage.importFile(null);
  await s.app.manage.importFile({ size: 10_000_000, text: async () => "" });
  assert.match(s.draw().text, /too large to be a settings file/);
  await s.app.manage.importFile(file('{"kind":"other"}'));
  assert.match(s.draw().text, /core refused what was sent: the file is not a core settings export/);
  const good = JSON.stringify({ kind: "ssg-core-settings", format: 1, modules: [{ id: "web-push", hidden: true, position: 0, labels: { en: "Push" }, note: "Ask first." }] });
  await s.app.manage.importFile(file(good));
  assert.equal(s.said.at(-1), "The file would change 4 settings. Check them, then apply.");
  let { root, text } = s.draw();
  for (const line of ["web-push title: hidden", "web-push title: position 1", "web-push title: named en: Push", "web-push title: note “Ask first.”"]) assert.match(text, new RegExp(line.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  fire(byId(root, "import-cancel"), "click");
  assert.doesNotMatch(s.draw().text, /What the import would change/);

  await s.app.manage.importFile(file(good));
  await call(modulePatch.onRequest, { method: "PATCH", path: "/api/core/manage/modules/blog", env, headers: { authorization: basic(PASSWORD), "cf-connecting-ip": freshIP(), "x-core-admin": "1" }, body: '{"note":"meanwhile"}' });
  ({ root } = s.draw());
  fire(byId(root, "import-apply"), "click");
  await waitFor(() => s.said.at(-1) === "That did not work.");
  assert.match(s.draw().text, /The settings changed while you were looking at the preview/);

  await s.app.manage.importFile(file(good));
  assert.match(s.draw().text, /blog title: no note/, "the reset of blog's note is previewed");
  await s.app.manage.applyImport();
  assert.equal(s.said.at(-1), "Imported: 5 settings changed.");
  ({ text } = s.draw());
  assert.doesNotMatch(text, /What the import would change/);
  assert.match(text, /imported settings \(changes: 5\)/);
  await s.app.manage.importFile(file(JSON.stringify({ kind: "ssg-core-settings", format: 1, modules: [{ id: "web-push", hidden: true, position: 0, labels: { en: "Push" }, note: "Ask first." }] })));
  assert.match(s.draw().text, /Nothing would change/);
  assert.ok(!walk(s.draw().root, (e) => e.attrs.id === "import-apply"), "nothing to apply, only Cancel");
});

test("history and the change log in words; a refused account; a failed load offers Try again", async () => {
  const db = fakeD1();
  const env = { ...PW_ENV, CORE_DB: db };
  db.sqlite.exec("SELECT 1");
  const s = setup(env);
  await s.app.manage.load();
  db.sqlite.exec(`INSERT INTO core_status_history (id, module_id, status, reason, observed_at) VALUES ('1', 'comments', 'reachable', NULL, '2026-11-13T10:00:00Z'), ('2', 'comments', 'unreachable', 'timeout', '2026-11-13T14:10:00Z'), ('3', 'gone', 'unreachable', 'other', '2026-11-13T14:11:00Z')`);
  db.sqlite.exec(`INSERT INTO core_changes (id, at, actor, actor_id, module_id, action, detail) VALUES ('c1', '2026-11-13T15:00:00Z', 'ada@example.com', 'u1', 'shop', 'labels', '{}'), ('c2', '2026-11-13T15:01:00Z', 'access', NULL, NULL, 'later', '{}')`);
  await s.app.manage.load();
  const { text } = s.draw();
  assert.match(text, /Not answering since Nov 13, 2026, 2:10 PM\./);
  assert.match(text, /Nov 13, 2026, 2:10 PM: comments title stopped answering\. It did not answer within the time limit\./);
  assert.match(text, /Nov 13, 2026, 10:00 AM: comments title answered\./);
  assert.match(text, /gone stopped answering/, "a module no longer in the registry is named by its id");
  assert.match(text, /ada@example.com renamed shop\./);
  assert.match(text, /someone signed in through Cloudflare Access changed the settings\./);
  assert.match(text, /Kept for 90 days\./);

  const refused = setup(env);
  refused.app.manage = createManage({ api: { get: async () => Promise.reject(new ApiError("forbidden", 403, "forbidden", null)), send: async () => ({}) }, app: refused.app, redraw: () => undefined, announce: () => undefined });
  await refused.app.manage.load();
  assert.match(refused.draw().text, /needs the core:manage permission/);
  const down = setup(env);
  let asked = 0;
  down.app.manage = createManage({ api: { get: async () => (asked++, Promise.reject(new ApiError("network", 0, null, "offline"))), send: async () => ({}) }, app: down.app, redraw: () => undefined, announce: () => undefined });
  await down.app.manage.load();
  const r = down.draw();
  assert.match(r.text, /core did not answer/);
  fire(walk(r.root, (e) => e.tag === "button" && textOf(e) === "Try again")!, "click");
  await new Promise((res) => setTimeout(res, 5));
  assert.equal(asked, 2);
});

test("a failed change is shown at its module, and the view keeps working", async () => {
  const db = fakeD1();
  const s = setup({ ...PW_ENV, CORE_DB: db });
  await s.app.manage.load();
  db.state.failOn = /INSERT INTO core_module_settings/;
  await s.app.manage.toggle("blog", true);
  const { root } = s.draw();
  const card = walk(root, (e) => e.attrs["aria-labelledby"] === "manage-blog")!;
  assert.match(textOf(walk(card, (e) => e.attrs.role === "alert")!), /core failed while answering/);
  assert.equal(s.said.at(-1), "That did not work.");
  db.state.failOn = null;
  await s.app.manage.toggle("blog", true);
  assert.ok(!walk(s.draw().root, (e) => e.attrs.role === "alert"));
  assert.ok(s.draws() > 4);
});

test("the view's tree: an empty registry, an empty available list", async () => {
  useRegistry({ format: 1, modules: [], available: [], theme: null });
  const s = setup(PW_ENV);
  await s.app.manage.load();
  const { text } = s.draw();
  assert.match(text, /The build registered no module\./);
  assert.match(text, /already uses every module its ssg ships/);
  assert.match(text, /Nothing recorded yet\./);
  assert.match(text, /Nothing has been changed yet\./);
  assert.equal(typeof manageView, "function");
});

async function waitFor(cond: () => boolean | undefined, ms = 2000) {
  const end = Date.now() + ms;
  while (!cond()) {
    if (Date.now() > end) throw new Error("timed out waiting");
    await new Promise((r) => setTimeout(r, 2));
  }
}

test("edges: api.send carries the CSRF header and JSON (or text as it is); showing again; an unknown kind; busy; a stale preview line; a card down since", async () => {
  const { createApi, CSRF_HEADER } = await import("../public/core/js/api.js");
  const seen: [string, RequestInit][] = [];
  const fake = async (u: string, i: RequestInit) => (seen.push([u, i]), Response.json({ ok: 1 }));
  await createApi(fake).send("PUT", "manage/order", { order: ["a"] });
  await createApi(fake).send("POST", "manage/import", '{"raw":1}', { apply: "x" });
  assert.deepEqual(seen.map(([u, i]) => [u, i.method, (i.headers as Record<string, string>)[CSRF_HEADER], i.body, i.credentials]), [
    ["/api/core/manage/order", "PUT", "1", '{"order":["a"]}', "same-origin"],
    ["/api/core/manage/import?apply=x", "POST", "1", '{"raw":1}', "same-origin"],
  ]);

  const db = fakeD1();
  const s = setup({ ...PW_ENV, CORE_DB: db });
  await s.app.manage.load();
  await s.app.manage.toggle("blog", true);
  await s.app.manage.toggle("blog", false);
  assert.equal(s.said.at(-1), "blog title is shown again.");
  db.sqlite.exec("UPDATE core_module_settings SET updated_by = 'ada@example.com' WHERE module_id = 'blog'");
  await s.app.manage.load();
  assert.match(s.draw().text, /Last changed by ada@example\.com on/);
  s.app.manage.state.view.available = [{ id: "x", kind: null, title: "X", command: "ssg new worker x", snippet: "workers:" }];
  s.app.manage.state.busy = true;
  s.app.manage.state.preview = { fingerprint: "f", changes: [{ module: "gone", field: "hidden", from: false, to: true }] };
  const { root, text } = s.draw();
  assert.match(text, /How it runs is not stated/);
  assert.match(text, /gone: hidden/);
  assert.equal(walk(root, (e) => e.attrs["aria-labelledby"] === "manage-list-title")!.attrs["aria-busy"], "true");

  const { moduleCard } = await import("../public/core/js/views/modules.js");
  const card = moduleCard({ t, fmt, titleOf: (id: string) => id }, { id: "a", title: "A", panel: null, status: "registered", summary: null, unreachableSince: "2026-11-13T14:10:00Z" }, { status: "unreachable", reason: "timeout" });
  const doc = fakeDoc();
  assert.match(textOf(render(card, doc as never) as unknown as FakeEl), /Not answering since Nov 13, 2026, 2:10 PM\.It did not answer within the time limit\./);
});
