// /core/nav.js, the element: stylesheet first, then the bar; nothing drawn
// when core fails or the stylesheet does; redraws (language, `current`, the
// REDRAW event) and races; registration. On a stand-in DOM.
import { test } from "node:test";
import assert from "node:assert/strict";
import { all, fakeDoc, fire } from "./dom.ts";
import type { FakeDoc, FakeEl } from "./dom.ts";

// The element class extends HTMLElement when there is one: give it ours
// before the module is loaded, and a registry it can define itself in.
class FakeHTMLElement {
  attrs: Record<string, string> = {};
  shadowRoot: FakeEl | null = null;
  isConnected = true;
  ownerDocument!: FakeDoc;
  events: Event[] = [];
  getAttribute(k: string) {
    return this.attrs[k] ?? null;
  }
  setAttribute(k: string, v: string) {
    this.attrs[k] = v;
  }
  removeAttribute(k: string) {
    delete this.attrs[k];
  }
  dispatchEvent(e: Event) {
    this.events.push(e);
    return true;
  }
  attachShadow() {
    this.shadowRoot = this.ownerDocument.createElement("#shadow") as FakeEl;
    return this.shadowRoot;
  }
}
const defined = new Map<string, unknown>();
Object.assign(globalThis, { HTMLElement: FakeHTMLElement, customElements: { get: (n: string) => defined.get(n), define: (n: string, c: unknown) => defined.set(n, c) } });
const nav = await import("../public/core/nav.js");
const { AdminNav, REDRAW, stylesheet } = nav;

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

// mount builds a connected element on a fake document whose fetch answers `answer`.
function mount(answer: () => Promise<Response>, current = "comments", lang = "pl") {
  const doc = fakeDoc();
  doc.documentElement.setAttribute("lang", lang);
  const el = new AdminNav() as unknown as FakeHTMLElement & InstanceType<typeof AdminNav>;
  Object.assign(el, { ownerDocument: doc, attrs: { current } });
  const calls: string[] = [];
  globalThis.fetch = (async (u: string) => (calls.push(u), answer())) as typeof fetch;
  return { el, doc, calls };
}
// loaded fires `load` on the stylesheet link in the shadow root once it is there.
async function loaded(el: { shadowRoot: FakeEl | null }, ok = true) {
  for (let i = 0; i < 50; i++) {
    const link = el.shadowRoot?.children.find((c) => (c as FakeEl).tag === "link") as FakeEl | undefined;
    if (link) return fire(link, ok ? "load" : "error");
    await new Promise((r) => setImmediate(r));
  }
}

test("stylesheet: load → true; error or a timeout → false", async () => {
  const doc = fakeDoc();
  const mk = () => doc.createElement("link") as FakeEl;
  const a = mk();
  const pa = stylesheet(a, 1000);
  fire(a, "load");
  const b = mk();
  const pb = stylesheet(b, 1000);
  fire(b, "error");
  assert.deepEqual([await pa, await pb, await stylesheet(mk(), 5)], [true, false, false]);
});

test("element: draws after its stylesheet loaded; the toggle and Escape are wired; a redraw keeps the stylesheet", async () => {
  const real = globalThis.fetch;
  try {
    const { el, doc, calls } = mount(async () => json(BODY));
    const drawing = el.refresh();
    await loaded(el);
    await drawing;
    const [link, root] = el.shadowRoot!.children as FakeEl[];
    assert.deepEqual([link.tag, link.attrs.rel, link.attrs.href.endsWith("/core/nav.css")], ["link", "stylesheet", true]);
    assert.equal(root.tag, "nav");
    assert.equal(calls[0], "/api/core/modules?lang=pl");
    const toggle = root.querySelector(".toggle")!;
    fire(toggle, "click");
    assert.equal(toggle.attrs["aria-expanded"], "true");
    fire(toggle, "click");
    assert.equal(toggle.attrs["aria-expanded"], "false");
    fire(toggle, "click");
    fire(root, "keydown", { key: "Escape" });
    assert.equal(toggle.attrs["aria-expanded"], "false");
    await el.refresh(); // a second drawing: no new stylesheet to wait for
    assert.equal(el.shadowRoot!.children[0], link);
    assert.notEqual(el.shadowRoot!.children[1], root);
    assert.equal(doc.listeners[REDRAW], undefined, "listeners belong to connectedCallback");
  } finally {
    globalThis.fetch = real;
  }
});

test("element: draws nothing when core fails, when the stylesheet fails, and when a newer drawing overtook it", async () => {
  const real = globalThis.fetch;
  try {
    const failed = mount(async () => json({ error: "x" }, 503));
    await failed.el.refresh();
    assert.deepEqual(failed.el.shadowRoot!.children, []);

    const unstyled = mount(async () => json(BODY));
    const p = unstyled.el.refresh();
    await loaded(unstyled.el, false);
    await p;
    assert.deepEqual(unstyled.el.shadowRoot!.children, [], "no unstyled bar");

    const raced = mount(async () => json(BODY));
    const older = raced.el.refresh();
    const newer = raced.el.refresh();
    await loaded(raced.el);
    await Promise.all([older, newer]);
    await loaded(raced.el);
    await new Promise((r) => setImmediate(r));
    assert.ok(raced.el.shadowRoot!.children.length <= 2);

    // After a good drawing, core going away clears the bar.
    let up = true;
    const gone = mount(async () => (up ? json(BODY) : json({}, 404)));
    const first = gone.el.refresh();
    await loaded(gone.el);
    await first;
    up = false;
    await gone.el.refresh();
    assert.deepEqual(gone.el.shadowRoot!.children, []);
    assert.equal(gone.el.styled, null);

    // A throw anywhere (here: no document) is swallowed.
    const broken = new AdminNav() as unknown as { refresh(): Promise<void> };
    await broken.refresh();
  } finally {
    globalThis.fetch = real;
  }
});

test("element lifecycle: connect draws and listens; lang changes redraw only when the language changes; current changes redraw after the first drawing", async () => {
  const real = globalThis.fetch;
  const RealMO = globalThis.MutationObserver;
  const observers: { fn: () => void; disconnected: boolean }[] = [];
  globalThis.MutationObserver = class {
    fn: () => void;
    disconnected = false;
    constructor(fn: () => void) {
      this.fn = fn;
      observers.push(this);
    }
    observe() {}
    disconnect() {
      this.disconnected = true;
    }
  } as never;
  try {
    const { el, doc, calls } = mount(async () => json({ error: "x" }, 401), "comments", "de");
    el.attributeChangedCallback("current", null, "comments");
    assert.equal(calls.length, 0, "before the first drawing, a parser-set attribute does not draw");
    el.connectedCallback();
    await new Promise((r) => setImmediate(r));
    assert.equal(calls.length, 1);
    observers[0].fn(); // lang unchanged
    assert.equal(calls.length, 1);
    doc.documentElement.setAttribute("lang", "fr");
    observers[0].fn();
    doc.dispatch(REDRAW);
    el.attributeChangedCallback("current", "comments", "comments");
    el.attributeChangedCallback("current", "comments", "newsletter-ui");
    await new Promise((r) => setImmediate(r));
    assert.deepEqual(calls.slice(1), ["/api/core/modules?lang=fr", "/api/core/modules?lang=fr", "/api/core/modules?lang=fr"]);
    el.connectedCallback(); // reconnected: one observer, one listener
    assert.equal(observers.length, 1);
    el.disconnectedCallback();
    assert.equal(observers[0].disconnected, true);
    assert.deepEqual(doc.listeners[REDRAW], [], "the same listener, added twice, is removed by one disconnect (as in the DOM)");
    el.isConnected = false;
    el.attributeChangedCallback("current", "a", "b");
    await new Promise((r) => setImmediate(r)); // let the reconnect's drawing finish asking
    // Without MutationObserver (old engines) it still draws.
    globalThis.MutationObserver = undefined as never;
    const plain = mount(async () => json({}, 404));
    plain.el.connectedCallback();
    await new Promise((r) => setImmediate(r));
    assert.equal(plain.calls.length, 1);
  } finally {
    globalThis.fetch = real;
    globalThis.MutationObserver = RealMO;
  }
});

test("element: a drawing overtaken while its stylesheet loads gives way to the newer one", async () => {
  const real = globalThis.fetch;
  try {
    const { el } = mount(async () => json(BODY));
    const older = el.refresh();
    let first: FakeEl | undefined;
    for (let i = 0; i < 50 && !first; i++) {
      first = el.shadowRoot?.children.find((c) => (c as FakeEl).tag === "link") as FakeEl | undefined;
      await new Promise((r) => setImmediate(r));
    }
    const newer = el.refresh();
    for (let i = 0; i < 50 && el.shadowRoot!.children[0] === first; i++) await new Promise((r) => setImmediate(r));
    fire(first!, "load"); // the older drawing resumes, sees it was overtaken, and stops
    await older;
    await loaded(el);
    await newer;
    assert.equal((el.shadowRoot!.children[1] as FakeEl).tag, "nav");
    assert.notEqual(el.shadowRoot!.children[0], first);
  } finally {
    globalThis.fetch = real;
  }
});

test("baseClass and define: no HTMLElement or custom elements (a non-browser importer) breaks nothing; defined only once", () => {
  assert.equal(nav.baseClass({ HTMLElement: FakeHTMLElement }), FakeHTMLElement);
  assert.equal(typeof nav.baseClass({}), "function");
  assert.equal(nav.define(undefined as never), false);
  assert.equal(nav.define({ get: () => AdminNav, define: () => assert.fail("defined twice") } as never), false);
  const fresh = new Map<string, unknown>();
  assert.equal(nav.define({ get: (n: string) => fresh.get(n), define: (n: string, c: unknown) => fresh.set(n, c) } as never), true);
  assert.equal(fresh.get("ssg-admin-nav"), AdminNav);
});

test("ready signal: data-ready, data-signout and a bubbling, composed ssg-admin-nav:ready after a drawing; removed when nothing is drawn", async () => {
  const real = globalThis.fetch;
  try {
    const { el } = mount(async () => json(BODY));
    el.connectedCallback();
    await loaded(el);
    for (let i = 0; i < 50 && el.attrs["data-ready"] === undefined; i++) await new Promise((r) => setImmediate(r));
    assert.deepEqual([el.attrs["data-ready"], el.attrs["data-signout"]], ["", "/api/auth/logout"]);
    const ev = el.events.at(-1) as CustomEvent;
    assert.deepEqual([ev.type, ev.bubbles, ev.composed, ev.detail], [nav.READY, true, true, { user: { email: "ada@example.com" }, signOut: "/api/auth/logout" }]);
    // Password mode: ready, no sign-out.
    globalThis.fetch = (async () => json({ ...BODY, mode: "password", user: null })) as typeof fetch;
    await el.refresh();
    assert.deepEqual([el.attrs["data-ready"], el.attrs["data-signout"], (el.events.at(-1) as CustomEvent).detail], ["", undefined, { user: null, signOut: null }]);
    // core gone: nothing drawn, nothing announced.
    const before = el.events.length;
    globalThis.fetch = (async () => json({}, 404)) as typeof fetch;
    await el.refresh();
    assert.deepEqual([el.attrs["data-ready"], el.attrs["data-signout"], el.events.length], [undefined, undefined, before]);
    el.disconnectedCallback();
  } finally {
    globalThis.fetch = real;
  }
});

test("ready signal: a stylesheet that fails clears the signal", async () => {
  const real = globalThis.fetch;
  try {
    const { el } = mount(async () => json(BODY));
    el.attrs["data-ready"] = "";
    el.attrs["data-signout"] = "/x";
    el.connectedCallback();
    await loaded(el, false);
    for (let i = 0; i < 50 && el.attrs["data-ready"] !== undefined; i++) await new Promise((r) => setImmediate(r));
    assert.deepEqual([el.attrs["data-ready"], el.attrs["data-signout"], el.events.length], [undefined, undefined, 0]);
    el.disconnectedCallback();
  } finally {
    globalThis.fetch = real;
  }
});
