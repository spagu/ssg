// /core/nav.js, its pure part: what the bar draws from /api/core/modules,
// that any failure means nothing to draw (never a throw), that a module's
// text stays text, the disclosure and Escape. The element itself: nav.test.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import { all, fakeDoc, textOf, walk } from "./dom.ts";
import type { FakeDoc, FakeEl } from "./dom.ts";

// The element class extends HTMLElement when there is one: give it ours
// before the module is loaded, and a registry it can define itself in.
class FakeHTMLElement {
  attrs: Record<string, string> = {};
  shadowRoot: FakeEl | null = null;
  isConnected = true;
  ownerDocument!: FakeDoc;
  getAttribute(k: string) {
    return this.attrs[k] ?? null;
  }
  attachShadow() {
    this.shadowRoot = this.ownerDocument.createElement("#shadow") as FakeEl;
    return this.shadowRoot;
  }
}
const defined = new Map<string, unknown>();
Object.assign(globalThis, { HTMLElement: FakeHTMLElement, customElements: { get: (n: string) => defined.get(n), define: (n: string, c: unknown) => defined.set(n, c) } });
const nav = await import("../public/core/nav.js");
const { AdminNav, API, QUIET, STRINGS, brandFallback, build, loadModel, localPath, navModel, onKey, pickLang, say, setOpen, view } = nav;

const BODY = {
  mode: "auth",
  user: { id: "u1", email: "ada@example.com" },
  home: "/core/",
  signOut: "/api/auth/logout",
  items: [
    { id: "core", title: "Admin core", panel: "/core/" },
    { id: "comments", title: "Comments", panel: "/comments-admin.html" },
    { id: "newsletter", title: "Newsletter sign-up", panel: null },
    { id: "hostile", title: '<img src=x onerror="alert(1)">', panel: "/h/" },
    { id: "offsite", title: "Off", panel: "//evil.example.com/" },
    { id: "blank", title: "  ", panel: "/b/" },
    null,
  ],
};
const json = (body: unknown, status = 200, type = "application/json") => new Response(JSON.stringify(body), { status, headers: { "content-type": type } });

test("define: the element registers itself once", () => {
  assert.equal(defined.get("ssg-admin-nav"), AdminNav);
  assert.equal(API, "/api/core/modules");
});

test("pickLang and say: the host's <html lang>, five languages, English otherwise", () => {
  assert.deepEqual(["pl-PL", "hi", "DE_at", "fr", "xx", null, ""].map(pickLang), ["pl", "hi", "de", "fr", "en", "en", "en"]);
  assert.equal(say("pl", "signedIn", { email: "a@example.com" }), "Zalogowano jako a@example.com");
  assert.equal(say("en", "signedIn"), "Signed in as {email}");
  const keys = Object.keys(STRINGS.en);
  for (const [lang, table] of Object.entries(STRINGS)) {
    assert.deepEqual(Object.keys(table), keys, lang);
    for (const v of Object.values(table)) assert.ok((v as string).trim(), lang);
  }
});

test("localPath: only a path on this host", () => {
  const ok = ["/", "/core/", "/a/b.html#x"];
  const bad = ["//evil.example.com", "/\\evil", "https://x", "javascript:alert(1)", "/a b", "", 7, null, "/" + "a".repeat(600)];
  assert.deepEqual(ok.map(localPath), ok);
  for (const v of bad) assert.equal(localPath(v), null, String(v));
});

test("navModel: modules with a panel, current marked, core and unusable entries left out; auth adds who and sign-out", () => {
  const m = navModel(BODY, "comments")!;
  assert.deepEqual(m.items.map((i: { id: string; current: boolean }) => `${i.id}:${i.current}`), ["comments:true", "hostile:false"]);
  assert.deepEqual([m.home, m.user, m.signOut, m.brand], [{ href: "/core/", current: false }, { email: "ada@example.com" }, "/api/auth/logout", { name: null, logo: null }]);
  assert.equal(navModel(BODY, "core")!.home.current, true);
  const pw = navModel({ ...BODY, mode: "password", user: null }, "x")!;
  assert.deepEqual([pw.user, pw.signOut], [null, null]);
  const branded = navModel({ ...BODY, brand: { name: " Kuchnia Anny ", logo: "/img/logo.svg" } }, "x")!;
  assert.deepEqual(branded.brand, { name: "Kuchnia Anny", logo: "/img/logo.svg" });
  assert.deepEqual(navModel({ ...BODY, brand: { logo: "https://evil.example.com/x.svg" } }, "x")!.brand, { name: null, logo: null });
  assert.equal(navModel({ ...BODY, mode: "auth", user: {} }, "x")!.user, null);
  const noHome = navModel({ ...BODY, home: null }, "comments")!;
  assert.equal(noHome.home, null);
});

test("navModel: nothing to draw → null (not an answer, no links at all)", () => {
  for (const body of [null, "x", {}, { items: {} }, { items: [], home: null }, { items: [{ id: "core", title: "c", panel: "/core/" }] }]) assert.equal(navModel(body, "x"), null);
  assert.ok(navModel({ items: [], home: "/core/" }, "x"), "home alone is worth a bar");
});

test("loadModel: asks quietly, same-origin, no redirects; any failure is null and never a throw", async () => {
  const seen: [string, RequestInit][] = [];
  const ok = await loadModel(async (u: string, i: RequestInit) => (seen.push([u, i]), json(BODY)), "pl", "comments");
  assert.equal(ok.items[0].id, "comments");
  assert.equal(seen[0][0], "/api/core/modules?lang=pl");
  assert.deepEqual([seen[0][1].credentials, seen[0][1].redirect, (seen[0][1].headers as Record<string, string>)["x-core-quiet"]], ["same-origin", "manual", QUIET["x-core-quiet"]]);
  const failures = [
    async () => json({ error: "unauthorized" }, 401),
    async () => json(BODY, 200, "text/html"),
    async () => new Response("<html>", { status: 200 }),
    async () => new Response(null, { status: 200 }),
    async () => new Response("{", { status: 200, headers: { "content-type": "application/json" } }),
    async () => new Response(null, { status: 0 } as never).clone(),
    async () => {
      throw new TypeError("offline");
    },
  ];
  for (const f of failures) assert.equal(await loadModel(f as never, "en", "x").catch(() => "threw"), null);
  // A request that never ends is given up on.
  const hang = (_u: string, i: RequestInit) => new Promise<Response>((_, reject) => i.signal!.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError"))));
  assert.equal(await loadModel(hang as never, "en", "x", 5), null);
});

test("view + build: the bar's structure; every title is a text node, never markup", () => {
  const doc = fakeDoc();
  const root = build(doc, view(navModel(BODY, "comments"), "en")) as FakeEl;
  assert.equal(root.tag, "nav");
  assert.equal(root.attrs["aria-label"], "Site administration");
  const links = all(root, (e) => e.tag === "a");
  assert.deepEqual(links.map((a) => [a.attrs.href, a.attrs["aria-current"] ?? null]), [["/core/", null], ["/core/", null], ["/comments-admin.html", "page"], ["/h/", null]]);
  assert.equal(all(root, (e) => e.tag === "img").length, 0, "the hostile title made no element");
  assert.equal(textOf(links[3]), '<img src=x onerror="alert(1)">');
  const form = walk(root, (e) => e.tag === "form")!;
  assert.deepEqual([form.attrs.method, form.attrs.action, textOf(form)], ["post", "/api/auth/logout", "Sign out"]);
  assert.match(textOf(root), /Signed in as ada@example\.com/);
  const toggle = walk(root, (e) => e.tag === "button" && e.attrs.class === "toggle")!;
  assert.deepEqual([toggle.attrs["aria-expanded"], toggle.attrs["aria-controls"]], ["false", "ssg-nav-menu"]);
  // No home, a logo, a sign-out without a known user, a password mode without either.
  const bare = build(doc, view({ brand: { name: "Kuchnia", logo: "/l.svg" }, home: null, items: [{ id: "a", label: "A", href: "/a", current: false }], user: null, signOut: "/out" }, "de")) as FakeEl;
  assert.equal(walk(bare, (e) => e.attrs.class === "brand")!.tag, "span");
  assert.equal(walk(bare, (e) => e.tag === "img")!.attrs.src, "/l.svg");
  assert.equal(walk(bare, (e) => e.attrs.class === "user"), null);
  const anonymousOut = build(doc, view({ brand: { name: null, logo: null }, home: null, items: [{ id: "a", label: "A", href: "/a", current: false }], user: { email: "b@example.com" }, signOut: null }, "en")) as FakeEl;
  assert.deepEqual([walk(anonymousOut, (e) => e.tag === "form"), textOf(walk(anonymousOut, (e) => e.attrs.class === "who")!)], [null, "Signed in as b@example.com"]);
  const quiet = build(doc, view({ brand: { name: null, logo: null }, home: null, items: [{ id: "a", label: "A", href: "/a", current: true }], user: null, signOut: null }, "fr")) as FakeEl;
  assert.equal(walk(quiet, (e) => e.attrs.class === "who"), null);
  assert.match(textOf(quiet), /Administration/);
});

test("disclosure: the button opens and closes; Escape closes and returns focus; other keys do nothing", () => {
  const doc = fakeDoc();
  const root = build(doc, view(navModel(BODY, "x"), "en")) as FakeEl;
  const toggle = root.querySelector(".toggle")!;
  const menu = root.querySelector(".menu")!;
  setOpen(root, true);
  assert.deepEqual([toggle.attrs["aria-expanded"], menu.classList.contains("open")], ["true", true]);
  onKey(root, { key: "Tab" });
  assert.equal(toggle.attrs["aria-expanded"], "true");
  onKey(root, { key: "Escape" });
  assert.deepEqual([toggle.attrs["aria-expanded"], menu.classList.contains("open"), toggle.focused], ["false", false, true]);
  toggle.focused = false;
  onKey(root, { key: "Escape" });
  assert.equal(toggle.focused, false, "Escape on a closed menu leaves focus alone");
});


test("brand (GO-152): the logo stands for the name — alt is the name, the dark-background logo first, the name when it fails to load", () => {
  const model = navModel({ ...BODY, brand: { name: "Harbour Kitchen", logo: "/img/logo.svg", logoDark: "/img/logo-dark.svg" } }, "x")!;
  assert.deepEqual(model.brand, { name: "Harbour Kitchen", logo: "/img/logo-dark.svg" });
  assert.equal(navModel({ ...BODY, brand: { name: "H", logo: "/img/logo.svg", logoDark: "javascript:alert(1)" } }, "x")!.brand.logo, "/img/logo.svg");
  const doc = fakeDoc();
  const root = build(doc, view(model, "en")) as FakeEl;
  const brand = root.querySelector(".brand")!;
  const img = brand.querySelector(".logo")!;
  assert.deepEqual([img.tag, img.attrs.src, img.attrs.alt, brand.querySelector(".name")], ["img", "/img/logo-dark.svg", "Harbour Kitchen", null]);
  brandFallback(doc, root, "Harbour Kitchen");
  for (const fn of img.listeners.error) fn({});
  assert.deepEqual([brand.querySelector(".logo"), textOf(brand.querySelector(".name")!)], [null, "Harbour Kitchen"]);
  // No logo: the name, and nothing to fall back from.
  const plain = build(doc, view(navModel({ ...BODY, brand: { name: "Plum & Pine" } }, "x")!, "en")) as FakeEl;
  brandFallback(doc, plain, "Plum & Pine");
  assert.equal(textOf(plain.querySelector(".brand")!), "Plum & Pine");
});
