// GO-173: the generic panel modules editor-ui shares with designer-ui (the
// source since GO-185; designer-ui's test/editor-copies.test.ts keeps its
// copies byte-identical) exercised here in full — core/ (vnode, router, i18n, api, prefs,
// session, diff, format, admin-nav) and panel/ (ui, dialog, publish-dialog,
// diff-view, conflict-view, revisions), plus panel/hunks-view.js, the one
// generic module editor-ui adds. Dialogs run on test/panel-dom.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import { find, findAll, h, mount, render, text, uid } from "../public/js/core/vnode.js";
import { createRouter, isRoute } from "../public/js/core/router.js";
import { LANGS, createI18n, pickLang, readConfig } from "../public/js/core/i18n.js";
import { ApiError, classify, createApi, query } from "../public/js/core/api.js";
import { applyTheme, createPrefs } from "../public/js/core/prefs.js";
import { readMe, renew } from "../public/js/core/session.js";
import { MAX_CELLS, hunks, lineDiff, stats } from "../public/js/core/diff.js";
import { createFormat } from "../public/js/core/format.js";
import { ABSENT, applyAdminNav, coverHeight, navVars, readAdminNav } from "../public/js/core/admin-nav.js";
import { busy, btn, emptyState, errorBlock, errorText, loading, pageHead, slot } from "../public/js/panel/ui.js";
import { openDialog } from "../public/js/panel/dialog.js";
import { publishDialog } from "../public/js/panel/publish-dialog.js";
import { diffView } from "../public/js/panel/diff-view.js";
import { conflictView } from "../public/js/panel/conflict-view.js";
import { revisionsPanel } from "../public/js/panel/revisions.js";
import { hunksView } from "../public/js/panel/hunks-view.js";
import { fakeDoc, fakeText, fire, walk } from "./panel-dom.ts";
import type { FakeEl } from "./panel-dom.ts";

type Node = { tag: string; props: Record<string, unknown>; children: (Node | string)[] };
const doc = fakeDoc();
(globalThis as Record<string, unknown>).document = doc;
const t = (key: string, vars: Record<string, unknown> = {}) => `${key}${Object.keys(vars).length ? JSON.stringify(vars) : ""}`;
const tick = () => new Promise((r) => setTimeout(r, 0));

test("vnode: h, text, find, render as text nodes and attributes, mount, uid", () => {
  const tree = h("div", null, "a", [h("b", { id: "x" }, 1), null, false, true, undefined], [[h("i", null, "c")]]);
  assert.equal(text(tree), "a1c");
  assert.equal(find(tree, (n: Node) => n.tag === "p"), null);
  assert.equal(find("leaf" as never, () => true), null);
  assert.equal(findAll(tree, (n: Node) => typeof n === "object").length, 3);
  let ref: unknown = null;
  let clicked = 0;
  const el = render(h("button", { class: "btn", on: { click: () => clicked++ }, ref: (e: unknown) => (ref = e), style: { width: "3px" }, value: "v", disabled: true, hidden: false, title: null }, "<b>no</b>", h("svg", null, h("rect"))), doc as never) as unknown as FakeEl & Record<string, unknown>;
  assert.deepEqual([el.attrs.class, el.attrs.disabled, "hidden" in el.attrs, el.value, (el.style as Record<string, string>).width, ref === el], ["btn", "", false, "v", "3px", true]);
  fire(el, "click");
  assert.equal(clicked, 1);
  assert.deepEqual(el.children[0], { text: "<b>no</b>" });
  assert.equal((el.children[1] as FakeEl).children[0] && ((el.children[1] as FakeEl).children[0] as FakeEl).ns, "http://www.w3.org/2000/svg");
  const parent = doc.createElement("div");
  mount(parent as never, [h("p", null, "x"), null, false, ["y"]]);
  assert.equal(parent.children.length, 2);
  assert.notEqual(uid("a"), uid("a"));
});

test("router, i18n, prefs, query", () => {
  const r = createRouter([["list", ""], ["edit", "edit/:path"]], { edit: "list" });
  assert.deepEqual(r.parse("#/edit/content%2Fa.md?x=1"), { name: "edit", params: { path: "content/a.md" }, query: { x: "1" }, section: "list" });
  assert.equal(r.parse("#/edit/%E0%A4%A").name, "notFound");
  assert.equal(r.parse(undefined as never).name, "list");
  assert.equal(r.href("edit", { path: "a/b.md" }, { q: "", s: null }), "#/edit/a%2Fb.md");
  assert.equal(r.href("list", undefined, { q: "x" }), "#/?q=x");
  assert.throws(() => r.href("nope"));
  assert.equal(createRouter([["a", "a"]]).parse("#/a").section, "a");
  assert.ok(isRoute("") && isRoute("#") && !isRoute("#main"));
  assert.deepEqual(LANGS, ["en", "pl", "hi", "de", "fr"]);
  assert.deepEqual([pickLang(["pt-BR", "pl-PL"]), pickLang([null, "xx"])], ["pl", "en"]);
  assert.deepEqual([readConfig('{"lang":"pl"}'), readConfig("[1]"), readConfig("nope"), readConfig(undefined)], [{ lang: "pl" }, {}, {}, {}]);
  const i = createI18n("pl", { en: { a: "A {x}", n: { one: "one", other: "{count} many" } }, pl: { n: { one: "jeden", few: "{count} kilka", other: "{count} inne" } } }, { pl: { o: "over" } }, (n: number) => `#${n}`);
  assert.deepEqual([i.t("a", { x: 1 }), i.t("a"), i.t("n", { count: 1 }), i.t("n", { count: 3 }), i.t("o"), i.t("missing")], ["A 1", "A {x}", "jeden", "#3 kilka", "over", "missing"]);
  assert.ok(i.has("a") && !i.has("missing"));
  assert.equal(createI18n("en", { en: { a: "x", n: { one: "1" } } }).t("n", { count: 2 }), "undefined");
  const m = new Map<string, string>();
  const p = createPrefs({ getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) }, "e");
  assert.equal(p.theme(), "auto");
  p.setTheme("dark");
  assert.equal(p.theme(), "dark");
  p.setTheme("auto");
  assert.equal(m.size, 0);
  const broken = createPrefs({ getItem: () => { throw new Error(); }, setItem: () => { throw new Error(); } }, "e");
  assert.deepEqual([broken.read("x"), broken.write("x", "1"), createPrefs(null, "e").read("x")], [null, false, null]);
  const attrs = new Map<string, string>();
  const root = { setAttribute: (k: string, v: string) => void attrs.set(k, v), removeAttribute: (k: string) => void attrs.delete(k) };
  assert.deepEqual([applyTheme(root, "light", true), applyTheme(root, "auto", true), applyTheme(root, "auto", false)], ["light", "dark", "light"]);
  assert.equal(query({}), "");
});

const res = (status: number, body: unknown, headers: Record<string, string> = { "content-type": "application/json" }) => new Response(body === undefined ? null : JSON.stringify(body), { status, headers });

test("api: paths, errors by kind, one renewal on 401; session: readMe and renew", async () => {
  const calls: string[] = [];
  const answers = [res(200, { ok: 1 }), res(401, {}), res(201, { done: 1 }), res(403, { error: "forbidden", permission: "editor-ui:media" })];
  const api = createApi({ base: "https://example.com/editor/", fetch: async (url: string, init: RequestInit) => (calls.push(`${init.method} ${url} ${(init.headers as Record<string, string>)["x-c"] ?? ""}`), answers.shift()!), onUnauthorized: async () => true, csrfHeader: "x-c" });
  assert.deepEqual(await api.get("content", { q: "a b", cursor: null }), { ok: 1 });
  assert.deepEqual(await api.post("changes", { a: 1 }), { done: 1 });
  await assert.rejects(api.post("changes"), (e: ApiError) => e.kind === "permission" && e.permission === "editor-ui:media");
  assert.deepEqual(calls.slice(0, 2), ["GET https://example.com/editor/api/content?q=a+b ", "POST https://example.com/editor/api/changes 1"]);
  await assert.rejects(createApi({ base: "https://example.com/", fetch: async () => { throw new Error("offline"); } }).get("me"), (e: ApiError) => e.kind === "network");
  await assert.rejects(createApi({ base: "https://example.com/", fetch: async () => { throw "plain"; } }).get("me"), (e: ApiError) => e.detail === "plain");
  await assert.rejects(createApi({ base: "https://example.com/", fetch: async () => res(500, undefined, { "content-type": "text/plain" }) }).get("me"), (e: ApiError) => e.kind === "server");
  assert.equal(await createApi({ base: "https://example.com/", fetch: async () => new Response("{", { headers: { "content-type": "application/json" } }) }).get("me"), null);
  await assert.rejects(createApi({ base: "https://example.com/", fetch: async () => res(401, {}), onUnauthorized: async () => false }).get("me"), (e: ApiError) => e.kind === "auth");
  assert.ok(createApi({ base: "https://example.com/" }).url("y").endsWith("/api/y"));
  assert.deepEqual([classify(403, { error: "cross_origin" }).kind, classify(429, null, "7").retryAfter, classify(429, null, null).retryAfter, classify(418, null).kind, classify(409, { detail: 3 }).detail], ["forbidden", 7, null, "invalid", null]);
  assert.equal(new ApiError("network", 0, null, null).message, "network");
  assert.equal(classify(400, { error: "x", detail: "why" }).detail, "why");
  await assert.rejects(createApi({ base: "https://example.com/", fetch: async () => new Response(null, { status: 502 }) }).get("me"), (e: ApiError) => e.kind === "server" && e.body === null);
  let reloaded = 0;
  let assigned = "";
  const location = { reload: () => reloaded++, assign: (u: string) => (assigned = u), pathname: "/editor/", search: "", hash: "#/", origin: "https://example.com" };
  assert.deepEqual(await readMe({ get: async () => ({ user: 1 }) }, location, null), { user: 1 });
  assert.equal(await readMe({ get: async () => { throw new ApiError("auth", 401, null, null); } }, location, "f"), "f");
  assert.equal(await readMe({ get: async () => { throw new ApiError("server", 500, null, null); } }, location, "f"), "f");
  const loginTarget = (me: { loginUrl?: string }, here: string) => (me.loginUrl ? `${me.loginUrl}?next=${encodeURIComponent(here)}` : null);
  assert.equal(await renew({ refreshUrl: "/r" }, { fetch: async () => new Response(null, { status: 204 }), location, loginTarget }), true);
  assert.equal(await renew({ refreshUrl: "/r", loginUrl: "/login" }, { fetch: async () => { throw new Error("x"); }, location, loginTarget }), false);
  assert.equal(assigned, "/login?next=%2Feditor%2F%23%2F");
  assert.equal(await renew({}, { fetch: async () => new Response(null), location, loginTarget }), false);
  assert.equal(reloaded, 2);
});

test("diff and format", () => {
  const ops = lineDiff("a\nb\nc\nd", "a\nB\nc\nd\ne");
  assert.deepEqual(stats(ops), { added: 2, removed: 1 });
  assert.deepEqual(hunks(lineDiff("x", "x"), 3), []);
  const long = Array.from({ length: 20 }, (_, i) => `l${i}`);
  assert.equal(hunks(lineDiff(long.join("\n"), long.map((l, i) => (i === 1 || i === 18 ? l + "!" : l)).join("\n")), 2).length, 2);
  assert.deepEqual(lineDiff("", "a").map((o: { op: string }) => o.op), ["add"]);
  assert.deepEqual(lineDiff("a\r\nb", "a\nb").map((o: { op: string }) => o.op), ["same", "same"]);
  assert.ok(MAX_CELLS > 0);
  const big = Array.from({ length: 2100 }, (_, i) => `x${i}`).join("\n");
  assert.equal(stats(lineDiff(big, big.replaceAll("x", "y"))).added, 2100, "beyond MAX_CELLS: removed then added");
  const f = createFormat("en", "UTC");
  assert.equal(f.number(1234), "1,234");
  assert.equal(f.number("x"), "0");
  assert.equal(f.percent(0.5), "50%");
  assert.equal(f.percent(Infinity), "0%");
  assert.equal(f.date("bad"), "—");
  assert.match(f.date("2026-11-15T10:00:00Z"), /2026/);
  assert.match(f.dateTime("2026-11-15T10:00:00Z"), /10:00/);
  assert.equal(f.dateTime(null as never), "—");
  assert.match(f.day("2026-11-15") + f.dayLong("2026-11-15") + f.month("2026-11-15"), /Nov/);
  assert.equal(f.list(["a", "b", "c"]), "a, b, and c");
  const now = Date.parse("2026-11-15T10:00:00Z");
  assert.deepEqual(["2026-11-15T09:59:30Z", "2026-11-15T09:30:00Z", "2026-11-15T07:00:00Z", "2026-11-13T10:00:00Z", "x"].map((d) => f.relative(d, now)), ["30 seconds ago", "30 minutes ago", "3 hours ago", "2 days ago", "—"]);
  assert.ok(f.relative("2026-11-15T09:59:30Z").length > 0);
  assert.deepEqual([f.duration(30), f.duration(4980), f.duration(120), f.duration(null), f.duration(-5)], ["30 sec", "1 hr 23 min", "2 min", "—", "0 sec"]);
  assert.ok(createFormat("pl").number(1) === "1");
});

test("admin-nav: absent, drawn, sticky, sign-out; cover height; variables applied", () => {
  const el = (height: number, signOut: boolean | null) => ({ getBoundingClientRect: () => ({ height }), shadowRoot: signOut === null ? null : { querySelector: () => (signOut ? {} : null) } });
  assert.equal(readAdminNav(null, () => ({})), ABSENT);
  assert.equal(readAdminNav(el(0, null), () => ({})), ABSENT);
  assert.deepEqual(readAdminNav(el(40.2, true), () => ({ position: "sticky" })), { height: 41, sticky: 41, signOut: true });
  assert.deepEqual(readAdminNav(el(40, null), () => ({ position: "fixed" })).signOut, false);
  assert.deepEqual(readAdminNav(el(40, false), () => ({ position: "static" })), { height: 40, sticky: 0, signOut: false });
  assert.equal(coverHeight({ bar: 60, notice: 10 }), 60);
  assert.equal(coverHeight({ bar: 60, notice: 10, nav: { height: 40, sticky: 40, signOut: false } }), 100);
  assert.deepEqual(navVars(ABSENT), { "--admin-nav-height": null, "--admin-nav-sticky": null });
  assert.deepEqual(navVars({ height: 40, sticky: 40, signOut: false }), { "--admin-nav-height": "40px", "--admin-nav-sticky": "40px" });
  const props = new Map<string, string>();
  const classes = new Set<string>();
  const root = { style: { setProperty: (k: string, v: string) => void props.set(k, v), removeProperty: (k: string) => void props.delete(k) }, classList: { toggle: (c: string, on: boolean) => void (on ? classes.add(c) : classes.delete(c)) } };
  applyAdminNav(root, { height: 40, sticky: 0, signOut: true });
  assert.deepEqual([props.get("--admin-nav-height"), props.has("--admin-nav-sticky"), classes.has("admin-nav-signout")], ["40px", false, true]);
  applyAdminNav(root, ABSENT);
  assert.deepEqual([props.size, classes.size], [0, 0]);
});

test("ui: heading, errors in words with the code, loading, empty, buttons, busy, slot", async () => {
  const notes: string[] = [];
  const app = { t, has: (k: string) => k === "code.known", notify: (m: string) => notes.push(m) };
  assert.equal(text(pageHead("T", { lead: "L", actions: btn("A", () => {}) })), "TAL");
  assert.equal(text(pageHead("T")), "T");
  assert.equal(errorText(app, { message: "m" }), 'error.unexpected{"detail":"m"}');
  assert.equal(await busy(app, null, async () => 1), 1);
  assert.equal(await busy(app, null, () => { throw new Error("sync"); }), undefined);
  assert.equal(await busy(app, doc.createElement("button"), async () => { throw new Error("no"); }), undefined);
  assert.equal(errorText(app, null), 'error.unexpected{"detail":"null"}');
  assert.equal(errorText(app, new ApiError("invalid", 400, "code.unknown_x" as never, "d")), 'error.invalid error.detail{"detail":"d"}');
  assert.equal(errorText(app, { kind: "x", code: "known", detail: null }), 'code.known{"detail":"","permission":""}');
  assert.equal(errorText(app, { kind: "rate", retryAfter: null }), 'error.rate{"seconds":60}');
  assert.equal(errorText(app, { kind: "permission", permission: "p" }), 'error.permission{"permission":"p"}');
  assert.equal(errorText(app, { kind: "network", detail: "x" }), "error.network");
  const block = errorBlock(app, { kind: "server", code: "boom" }, () => {});
  assert.match(text(block), /error.code/);
  assert.equal(find(errorBlock(app, { kind: "server" }, null), (n: Node) => n.tag === "button"), null);
  assert.equal(loading(app).props.role, "status");
  assert.equal(find(emptyState({ img: "no-results", title: "T", body: "B" }), (n: Node) => n.tag === "img")!.props.src, "img/no-results.svg");
  assert.equal(btn("x", () => {}, { variant: "primary" }).props.class, "btn btn-primary");
  const button = doc.createElement("button");
  assert.equal(await busy(app, button, async () => 7), 7);
  assert.equal(await busy(app, null, async () => { throw new ApiError("server", 500, null, null); }), undefined);
  assert.deepEqual(notes, ['error.unexpected{"detail":"sync"}', 'error.unexpected{"detail":"no"}', "error.server"]);
  assert.equal(button.attrs["aria-busy"], undefined);
  const s = slot("x");
  s.set(h("p"));
  render(s.node, doc as never);
  s.set(h("p", null, "now"));
  assert.equal((s.el as FakeEl).children.length, 1);
  assert.equal(slot().node.props.class, null);
});

// openIn: start a dialog, then act on it once it is in the document.
async function openIn<T>(open: () => Promise<T>, act: (dialog: FakeEl) => void): Promise<T> {
  const before = doc.body.children.length;
  const p = open();
  const dialog = doc.body.children[before] as FakeEl;
  assert.equal(dialog.props.open, true);
  act(dialog);
  return p;
}
const buttons = (dlg: FakeEl) => {
  const out: FakeEl[] = [];
  walk(dlg, (e) => (e.tag === "button" && out.push(e), false));
  return out;
};

test("dialog: buttons close with their value, Escape with null, submit checks first, focus returns", async () => {
  const opener = doc.createElement("button");
  doc.body.appendChild(opener);
  opener.focus();
  assert.equal(await openIn(() => openDialog({ title: "T", actions: [{ label: "No", value: null }, { label: "Yes", value: "y", variant: "primary" }] }), (d) => fire(buttons(d)[1], "click")), "y");
  assert.equal(doc.activeElement, opener, "focus back on the opener");
  assert.equal(await openIn(() => openDialog({ title: "T", wide: true, labelledBy: "x", actions: [{ label: "No", value: null }] }), (d) => fire(d, "cancel")), null);
  let allowed = false;
  const p = openIn(() => openDialog({ title: "T", actions: [{ label: "Go", value: "go", submit: true, check: () => allowed }] }), (d) => {
    const form = walk(d, (e) => e.tag === "form")!;
    fire(form, "submit");
    assert.equal(d.props.open, true, "check() false keeps it open");
    allowed = true;
    fire(form, "submit");
  });
  assert.equal(await p, "go");
  assert.equal(await openIn(() => openDialog({ title: "T", actions: [{ label: "Go", value: "go", submit: true }, { label: "x", value: null }] }), (d) => fire(walk(d, (e) => e.tag === "form")!, "submit")), "go");
  doc.activeElement = null;
  assert.equal(await openIn(() => openDialog({ title: "T", actions: [{ label: "a", value: 1 }] }), (d) => fire(buttons(d)[0], "click")), 1);
  assert.equal(await openIn(() => openDialog({ title: "T", actions: [] }), (d) => {
    fire(walk(d, (e) => e.tag === "form")!, "submit");
    assert.equal(d.props.open, true, "no submit action: Enter does nothing");
    fire(d, "cancel");
  }), null);
});

test("publish dialog: lines, files, confirmations, the message must not be empty", async () => {
  const typed = (d: FakeEl, value: string) => fire(walk(d, (e) => e.tag === "textarea")!, "input", { target: { value } });
  const got = await openIn(() => publishDialog(t, { lines: [{ label: "Title", from: "a", to: "b", note: "n" }, { label: "X", from: "1", to: "2" }], files: [{ path: "a.md", text: "+1" }], message: "Update", author: "ana@example.com", confirm: [{ id: "c", text: "Sure" }] }), (d) => {
    const form = walk(d, (e) => e.tag === "form")!;
    typed(d, "  ");
    fire(form, "submit");
    assert.equal(walk(d, (e) => e.attrs.class === "form-problem")!.textContent, "publish.messageEmpty");
    typed(d, " Fixed it ");
    fire(form, "submit");
    assert.equal(walk(d, (e) => e.attrs.class === "form-problem")!.textContent, "publish.confirmMissing");
    const box = walk(d, (e) => e.attrs.type === "checkbox")!;
    fire(box, "change", { target: { checked: true } });
    fire(box, "change", { target: { checked: false } });
    fire(box, "change", { target: { checked: true } });
    fire(form, "submit");
  });
  assert.deepEqual(got, { message: "Fixed it" });
  assert.equal(await openIn(() => publishDialog(t, {}), (d) => fire(buttons(d)[0], "click")), null);
});

test("diff view, conflict view and the worker's hunks look alike, in words as well as colour", () => {
  assert.equal(text(diffView(t, "a", "a")), "diff.none");
  assert.match(text(diffView(t, "a\nb", "a\n\nb")), /diff.added  /, "an empty added line still shows");
  const v = diffView(t, "a\nb", "a\nc", { label: "L" });
  assert.match(text(v), /^L — diff.label/);
  assert.ok(find(v, (n: Node) => n.props.class === "diff-add") && find(v, (n: Node) => n.props.class === "diff-del"));
  const long = Array.from({ length: 30 }, (_, i) => `l${i}`);
  assert.ok(find(diffView(t, long.join("\n"), long.map((l, i) => (i === 0 || i === 29 ? "x" : l)).join("\n")), (n: Node) => n.props.class === "diff-gap"));
  const c = conflictView(t, { head: "abcdef123", files: [{ path: "a.md", mine: "x", theirs: "y" }, { path: "b.png", mine: null, theirs: null }], keys: ["Title"], actions: [btn("Go", () => {})] });
  assert.match(text(c), /conflict.commit\{"commit":"abcdef1"\}/);
  assert.equal(conflictView(t, {}).children.length, 2);
  assert.equal(text(hunksView(t, { identical: true })), "diff.none");
  assert.equal(text(hunksView(t, { identical: false, hunks: [] })), "diff.none");
  const hv = hunksView(t, { identical: false, added: 1, removed: 1, hunks: [{ lines: [{ kind: "same", text: "", a: 1, b: 1 }, { kind: "removed", text: "old", a: 2, b: null }] }, { lines: [{ kind: "added", text: "new", a: null, b: 9, noNewline: true }, { kind: "odd", text: "?", a: 3, b: 3 }] }] }, { label: "L" });
  assert.deepEqual(findAll(hv, (n: Node) => n.tag === "tr").map((n: Node) => n.props.class), ["diff-same", "diff-del", "diff-add", "diff-same"]);
  assert.match(text(hv), /diff.removed old/);
  assert.match(text(hv), /diff.noNewline/);
  assert.ok(find(hv, (n: Node) => n.props.class === "diff-gap"));
  assert.doesNotMatch(text(hunksView(t, { hunks: [{ lines: [] }] })), /—/);
});

test("revisions panel: first page, View, Show older, a failing load in words", async () => {
  const viewed: string[] = [];
  const app = { t, has: () => false, notify: () => {}, fmt: createFormat("en", "UTC") };
  const pages: Record<string, { items: Record<string, string>[]; nextCursor: string | null }> = {
    first: { items: [{ commitId: "c2", message: "Second\nbody", authorName: "Ana", authorEmail: "a@example.com", date: "2026-11-15T10:00:00Z" }], nextCursor: "p2" },
    p2: { items: [{ commitId: "c1", message: "First", authorName: "", authorEmail: "b@example.com", date: "2026-11-14T10:00:00Z" }], nextCursor: null },
  };
  const node = revisionsPanel(app, { title: "History", load: async (cursor: string | null) => pages[cursor ?? "first"], onView: (r: { commitId: string }) => viewed.push(r.commitId) });
  const el = render(node, doc as never) as unknown as FakeEl;
  await tick();
  const list = el.children[1] as FakeEl;
  const all = () => { const out: FakeEl[] = []; walk(list, (e) => (e.tag === "button" && out.push(e), false)); return out; };
  fire(all()[0], "click");
  assert.deepEqual(viewed, ["c2"]);
  fire(all()[1], "click", { currentTarget: all()[1] });
  await tick();
  await tick();
  assert.equal(all().length, 2, "two revisions, no more pages");
  const empty = render(revisionsPanel(app, { title: "H", load: async () => ({ items: [], nextCursor: null }), onView: () => {} }), doc as never) as unknown as FakeEl;
  await tick();
  assert.match(fakeText(empty), /rev.empty/);
  const failing = render(revisionsPanel(app, { title: "H", load: async () => { throw new ApiError("server", 500, "x", null); }, onView: () => {} }), doc as never) as unknown as FakeEl;
  await tick();
  assert.match(fakeText(failing), /error.title/);
});
