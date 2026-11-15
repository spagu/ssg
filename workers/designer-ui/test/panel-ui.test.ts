// panel/ui.js — the building blocks every view shares (heading, errors in
// words, loading, empty state, buttons, busy, slot). Generic: a byte copy of
// editor-ui's (test/editor-copies.test.ts), and this suite covers every
// branch of it here as well.

import { test } from "node:test";
import assert from "node:assert/strict";
import { find, findAll, h, mount, render, text, uid } from "../public/js/core/vnode.js";
import { busy, btn, emptyState, errorBlock, errorText, loading, pageHead, slot } from "../public/js/panel/ui.js";

type Node = { tag: string; props: Record<string, unknown>; children: (Node | string)[] };
const t = (key: string, vars: Record<string, unknown> = {}) => `${key}${Object.keys(vars).length ? JSON.stringify(vars) : ""}`;

// A stand-in for the few DOM calls render() and mount() make.
function fakeEl(tag: string, ns: string | null = null) {
  const el = {
    tag, ns, attrs: {} as Record<string, string>, children: [] as unknown[], disabled: false, value: "",
    listeners: {} as Record<string, unknown>, style: {} as Record<string, string> & { setProperty?: (k: string, v: string) => void },
    setAttribute: (k: string, v: string) => void (el.attrs[k] = v),
    removeAttribute: (k: string) => void delete el.attrs[k],
    appendChild: (c: unknown) => (el.children.push(c), c),
    replaceChildren: (...c: unknown[]) => void (el.children = c),
    addEventListener: (type: string, fn: unknown) => void (el.listeners[type] = fn),
  };
  el.style.setProperty = (k: string, v: string) => void (el.style[k] = v);
  return el;
}
(globalThis as Record<string, unknown>).document = {
  createElement: (tag: string) => fakeEl(tag),
  createElementNS: (ns: string, tag: string) => fakeEl(tag, ns),
  createTextNode: (s: string) => ({ text: s }),
};

// vnode.js comes with ui.js (also a byte copy of editor-ui's).
test("vnode: h, text, find, findAll, render with properties and SVG, mount, uid", () => {
  const tree = h("div", null, "a", [h("b", { id: "x" }, 1), null, false, true, undefined], h("svg", null, h("rect")));
  assert.equal(text(tree), "a1");
  assert.equal(find(tree, (n: Node) => n.tag === "p"), null);
  assert.equal(find("leaf" as never, () => true), null);
  assert.equal(find(tree, (n: Node) => n.tag === "rect").tag, "rect");
  assert.equal(findAll(tree, (n: Node) => typeof n === "object").length, 4);
  assert.deepEqual(findAll("leaf" as never, () => true), []);
  let ref: unknown = null;
  const click = () => {};
  const el = render(h("input", { on: { click }, ref: (e: unknown) => (ref = e), style: { width: "3px" }, value: "v", disabled: true, hidden: false, title: null, alt: undefined, name: "n" }, h("svg", null, h("rect")))) as ReturnType<typeof fakeEl>;
  assert.deepEqual([el.listeners.click === click, ref === el, el.style.width, el.value, el.attrs], [true, true, "3px", "v", { disabled: "", name: "n" }]);
  const svg = el.children[0] as ReturnType<typeof fakeEl>;
  assert.deepEqual([svg.ns, (svg.children[0] as ReturnType<typeof fakeEl>).ns], ["http://www.w3.org/2000/svg", "http://www.w3.org/2000/svg"]);
  const parent = fakeEl("div");
  mount(parent, [h("p", null, "x"), null, [false, "y"]]);
  assert.deepEqual(parent.children.map((c) => (c as { tag?: string; text?: string }).tag ?? (c as { text: string }).text), ["p", "y"]);
  assert.notEqual(uid("a"), uid("a"));
});

test("ui: page heading with and without lead and actions", () => {
  assert.equal(text(pageHead("T", { lead: "L", actions: btn("A", () => {}) })), "TAL");
  assert.equal(text(pageHead("T")), "T");
});

test("ui: errors in words: code, rate, permission, kind with and without detail", () => {
  const app = { t, has: (k: string) => k === "code.known" };
  assert.equal(errorText(app, { message: "m" }), 'error.unexpected{"detail":"m"}');
  assert.equal(errorText(app, null), 'error.unexpected{"detail":"null"}');
  assert.equal(errorText(app, { kind: "x", code: "known", detail: null }), 'code.known{"detail":"","permission":""}');
  assert.equal(errorText(app, { kind: "x", code: "known", detail: "d", permission: "p" }), 'code.known{"detail":"d","permission":"p"}');
  assert.equal(errorText(app, { kind: "rate", retryAfter: null }), 'error.rate{"seconds":60}');
  assert.equal(errorText(app, { kind: "rate", retryAfter: 5 }), 'error.rate{"seconds":5}');
  assert.equal(errorText(app, { kind: "permission", permission: "p" }), 'error.permission{"permission":"p"}');
  assert.equal(errorText(app, { kind: "network", detail: "x" }), "error.network");
  assert.equal(errorText(app, { kind: "server", code: "other", detail: "x" }), 'error.server error.detail{"detail":"x"}');
  assert.equal(errorText(app, { kind: "server" }), "error.server");
  assert.match(text(errorBlock(app, { kind: "server", code: "boom" }, () => {})), /error\.code/);
  assert.equal(find(errorBlock(app, { kind: "server" }, null), (n: Node) => n.tag === "button"), null);
});

test("ui: loading, empty state, buttons", () => {
  assert.equal(loading({ t }).props.role, "status");
  const empty = emptyState({ img: "x", title: "T", body: "B", action: btn("go", () => {}) });
  assert.equal(find(empty, (n: Node) => n.tag === "img")!.props.src, "img/x.svg");
  assert.equal(btn("x", () => {}, { variant: "primary" }).props.class, "btn btn-primary");
  assert.equal(btn("x", () => {}).props.class, "btn");
});

test("ui: busy: the button disabled while it runs, failures reported, sync throws caught", async () => {
  const notes: string[] = [];
  const app = { t, has: () => false, notify: (m: string, kind: string) => notes.push(`${kind}:${m}`) };
  const button = fakeEl("button");
  let during: unknown[] = [];
  assert.equal(await busy(app, button, async () => ((during = [button.disabled, button.attrs["aria-busy"]]), 7)), 7);
  assert.deepEqual([during, button.disabled, "aria-busy" in button.attrs], [[true, "true"], false, false]);
  assert.equal(await busy(app, button, async () => { throw new Error("no"); }), undefined);
  assert.equal(button.disabled, false);
  assert.equal(await busy(app, null, () => { throw new Error("sync"); }), undefined);
  assert.equal(await busy(app, null, () => 1), 1);
  assert.deepEqual(notes, ['critical:error.unexpected{"detail":"no"}', 'critical:error.unexpected{"detail":"sync"}']);
});

test("ui: slot re-renders its own element, and does nothing before it is in the page", () => {
  const s = slot("box");
  assert.equal(s.node.props.class, "box");
  assert.equal(slot().node.props.class, null);
  assert.equal(s.set("early"), null);
  const el = fakeEl("div");
  (s.node.props.ref as (e: unknown) => void)(el);
  assert.equal(s.el, el);
  s.set(["a", "b"]);
  assert.deepEqual(el.children, [{ text: "a" }, { text: "b" }]);
});
