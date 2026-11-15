// The look preview in the panel (GO-188): the frame's address (the page, the
// scheme, the language, the tokens as edited) and the section's controls —
// the frame is sandboxed with every restriction, a bad path never reaches
// the worker, edits reload the frame once it is shown, after a pause.
import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { render } from "../public/js/core/vnode.js";
import { isSitePath, previewHref, schemeValues } from "../public/js/models/preview-url.js";
import { previewSection } from "../public/js/views/preview.js";

type El = { tag: string; attrs: Record<string, string>; children: El[]; listeners: Record<string, (e: unknown) => void>; textContent: string; checked?: boolean; value?: string };
function fakeEl(tag: string): El {
  const el = {
    tag, attrs: {} as Record<string, string>, children: [] as El[], listeners: {} as Record<string, (e: unknown) => void>, textContent: "",
    setAttribute: (k: string, v: string) => void (el.attrs[k] = v),
    appendChild: (c: El) => (el.children.push(c), c),
    replaceChildren: (...c: El[]) => void (el.children = c),
    addEventListener: (type: string, fn: (e: unknown) => void) => void (el.listeners[type] = fn),
    style: { setProperty: () => {} },
  };
  return el;
}
(globalThis as Record<string, unknown>).document = { createElement: fakeEl, createElementNS: (_: string, tag: string) => fakeEl(tag), createTextNode: (s: string) => ({ tag: "#text", textContent: s, children: [], attrs: {} }) };

const all = (el: El, out: El[] = []): El[] => (out.push(el), el.children.forEach((c) => all(c, out)), out);
const byId = (root: El, id: string) => all(root).find((e) => e.attrs.id === id) as El;
const frameOf = (root: El) => all(root).find((e) => e.tag === "iframe");

const FIELDS = [
  { name: "--color-accent", kind: "colour", light: "#0050a6", dark: "#99bfeb" },
  { name: "--color-only-light", kind: "colour", light: "#111111", dark: null },
  { name: "--font-sans", kind: "font", light: '"Geist", system-ui,\n    sans-serif', dark: null },
  { name: "--font-odd", kind: "font", light: "url(x.woff)", dark: null },
  { name: "--font-semicolon", kind: "font", light: "a; b", dark: null },
];

test("isSitePath: a path on this host, never a URL or a dot segment", () => {
  for (const p of ["/", "/blog/my-post/", "/a?b=1&c=%20", "/p~x/@y:z"]) assert.equal(isSitePath(p), true, p);
  for (const p of ["", "blog/", "//example.org/", "https://example.org/", "/a/../b", "/a/./b", "/%2E%2e/x", "/a b", "/\\x", "/#x", null, "/" + "a".repeat(2048)]) assert.equal(isSitePath(p), false, String(p));
});

test("schemeValues and previewHref: the scheme's values, usable ones only, in the address", () => {
  assert.deepEqual(schemeValues(FIELDS, "light"), [["--color-accent", "#0050a6"], ["--color-only-light", "#111111"], ["--font-sans", '"Geist", system-ui, sans-serif']]);
  assert.deepEqual(schemeValues(FIELDS, "dark").slice(0, 2), [["--color-accent", "#99bfeb"], ["--color-only-light", "#111111"]]);
  assert.deepEqual(schemeValues([{ name: "--x", light: null, dark: "#000" }], "light"), [["--x", "#000"]]);
  assert.equal(previewHref({ page: "home", path: "/ignored/", scheme: "light", lang: "pl", values: [["--a", "#fff"]] }), "api/preview?path=%2F&scheme=light&lang=pl&--a=%23fff");
  assert.equal(previewHref({ page: "path", path: "/blog/x/", scheme: "dark", lang: "en", values: [] }), "api/preview?path=%2Fblog%2Fx%2F&scheme=dark&lang=en");
  assert.equal(previewHref({ page: "latest", path: "/", scheme: "dark", lang: "en", values: [] }), "api/preview?page=latest&scheme=dark&lang=en");
});

test("the section: nothing loads until asked; then a fully sandboxed frame, per page, scheme and width", () => {
  const app = { t: (k: string) => k, lang: "de" };
  let fields = FIELDS;
  const p = previewSection(app, () => fields);
  const root = render(p.node) as unknown as El;
  const slotEl = all(root).find((e) => e.attrs.class === "preview-frame-slot") as El;
  p.refresh(); // not shown yet: nothing happens
  assert.equal(frameOf(root), undefined);
  const radios = all(root).filter((e) => e.attrs.type === "radio");
  radios[1].listeners.change({}); // latest, still not shown
  assert.equal(frameOf(root), undefined);

  byId(root, "preview-show").listeners.click({});
  let frame = frameOf(slotEl) as El;
  assert.equal(frame.attrs.sandbox, "", "sandbox with no allowance: no scripts, opaque origin");
  assert.equal(frame.attrs.referrerpolicy, "no-referrer");
  assert.equal(frame.attrs.title, "preview.frameTitle");
  assert.equal(frame.attrs.class, "preview-frame");
  assert.match(frame.attrs.src, /^api\/preview\?page=latest&scheme=light&lang=de&--color-accent=%230050a6/);

  byId(root, "preview-scheme").listeners.change({ target: { value: "dark" } });
  byId(root, "preview-phone").listeners.change({ target: { checked: true } });
  frame = frameOf(slotEl) as El;
  assert.equal(frame.attrs.class, "preview-frame phone");
  assert.match(frame.attrs.src, /scheme=dark&lang=de&--color-accent=%2399bfeb/);

  // A path: typed, then committed; a bad one is said, not sent.
  const input = byId(root, "preview-path");
  const problem = byId(root, "preview-path-problem");
  input.listeners.input({ target: { value: "//example.org/" } });
  input.listeners.change({});
  assert.equal(radios[2].checked, true);
  assert.equal(problem.textContent, "preview.badPath");
  assert.match(frameOf(slotEl)!.attrs.src, /page=latest/, "the frame stays as it was");
  input.listeners.input({ target: { value: " /blog/x/ " } });
  input.listeners.change({});
  assert.equal(problem.textContent, "");
  assert.match(frameOf(slotEl)!.attrs.src, /^api\/preview\?path=%2Fblog%2Fx%2F&scheme=dark/);
  radios[0].listeners.change({});
  assert.match(frameOf(slotEl)!.attrs.src, /^api\/preview\?path=%2F&/);

  // Edits reload the frame once, after the pause.
  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    fields = [{ name: "--color-accent", kind: "colour", light: "#b42318", dark: "#ffb4a9" }];
    p.refresh();
    p.refresh();
    assert.match(frameOf(slotEl)!.attrs.src, /%2399bfeb/);
    mock.timers.tick(800);
    assert.match(frameOf(slotEl)!.attrs.src, /--color-accent=%23ffb4a9$/);
  } finally {
    mock.timers.reset();
  }
});
