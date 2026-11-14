// A small stand-in for the DOM the home page's modules and the nav element
// touch — enough to render trees, fire events and attach a shadow root —
// so they are tested in node without a browser.

export interface FakeNode {
  text?: string;
}
export interface FakeEl extends FakeNode {
  tag: string;
  ns: string | null;
  attrs: Record<string, string>;
  children: (FakeEl | FakeNode)[];
  listeners: Record<string, ((e: unknown) => void)[]>;
  style: Record<string, string> & { setProperty(k: string, v: string): void };
  classList: { add(c: string): void; toggle(c: string, on?: boolean): void; contains(c: string): boolean };
  ownerDocument: FakeDoc;
  hidden?: boolean;
  textContent?: string;
  focused?: boolean;
  [k: string]: unknown;
  setAttribute(k: string, v: string): void;
  getAttribute(k: string): string | null;
  removeAttribute(k: string): void;
  addEventListener(t: string, fn: (e: unknown) => void): void;
  removeEventListener(t: string, fn: (e: unknown) => void): void;
  appendChild<T>(c: T): T;
  append(...c: (FakeEl | FakeNode)[]): void;
  replaceChildren(...c: (FakeEl | FakeNode)[]): void;
  querySelector(sel: string): FakeEl | null;
  focus(): void;
}
export type FakeDoc = ReturnType<typeof fakeDoc>;

const isEl = (n: FakeEl | FakeNode): n is FakeEl => "tag" in n;

// matches: ".cls", "tag", "tag.cls" — what the code under test asks for.
function matches(e: FakeEl, sel: string): boolean {
  const [tag, cls] = sel.startsWith(".") ? ["", sel.slice(1)] : sel.split(".");
  return (!tag || e.tag === tag) && (!cls || (e.attrs.class ?? "").split(/\s+/).includes(cls));
}

export function walk(n: FakeEl | FakeNode, test: (e: FakeEl) => boolean): FakeEl | null {
  if (!isEl(n)) return null;
  if (test(n)) return n;
  for (const c of n.children) {
    const hit = walk(c, test);
    if (hit) return hit;
  }
  return null;
}

export function all(n: FakeEl | FakeNode, test: (e: FakeEl) => boolean, out: FakeEl[] = []): FakeEl[] {
  if (!isEl(n)) return out;
  if (test(n)) out.push(n);
  for (const c of n.children) all(c, test, out);
  return out;
}

export const textOf = (n: FakeEl | FakeNode): string => (isEl(n) ? n.children.map(textOf).join("") : (n.text ?? ""));

export function fakeDoc() {
  const byId = new Map<string, FakeEl>();
  const docListeners: Record<string, ((e: unknown) => void)[]> = {};
  const doc = {
    byId,
    documentElement: null as unknown as FakeEl,
    createTextNode: (text: string) => ({ text }),
    createElement: (tag: string) => el(tag, null),
    createElementNS: (ns: string, tag: string) => el(tag, ns),
    getElementById: (id: string) => byId.get(id) ?? null,
    addEventListener: (t: string, fn: (e: unknown) => void) => void (docListeners[t] ??= []).push(fn),
    removeEventListener: (t: string, fn: (e: unknown) => void) => void (docListeners[t] = (docListeners[t] ?? []).filter((f) => f !== fn)),
    dispatch: (t: string) => (docListeners[t] ?? []).forEach((fn) => fn({ type: t })),
    listeners: docListeners,
  };
  function el(tag: string, ns: string | null): FakeEl {
    const classes = () => new Set((e.attrs.class ?? "").split(/\s+/).filter(Boolean));
    const setClasses = (s: Set<string>) => (e.attrs.class = [...s].join(" "));
    const e: FakeEl = {
      tag,
      ns,
      attrs: {},
      children: [],
      listeners: {},
      ownerDocument: doc as FakeDoc,
      style: Object.assign(Object.create(null), { setProperty(k: string, v: string) { (e.style as Record<string, unknown>)[k] = v; } }),
      classList: {
        add: (c) => setClasses(classes().add(c)),
        toggle(c, on) {
          const s = classes();
          if (on ?? !s.has(c)) s.add(c);
          else s.delete(c);
          setClasses(s);
        },
        contains: (c) => classes().has(c),
      },
      setAttribute(k, v) {
        e.attrs[k] = v;
        if (k === "id") byId.set(v, e);
        if (k === "hidden") e.hidden = true;
      },
      getAttribute: (k) => e.attrs[k] ?? null,
      removeAttribute: (k) => void delete e.attrs[k],
      addEventListener: (t, fn) => void (e.listeners[t] ??= []).push(fn),
      removeEventListener: (t, fn) => void (e.listeners[t] = (e.listeners[t] ?? []).filter((f) => f !== fn)),
      appendChild(c) {
        e.children.push(c as never);
        return c;
      },
      append: (...c) => void e.children.push(...c),
      replaceChildren: (...c) => void (e.children = c),
      querySelector: (sel) => walk({ ...e, attrs: {}, tag: "" } as FakeEl, (x) => x !== e && matches(x, sel)) ?? null,
      focus: () => void (e.focused = true),
    };
    return e;
  }
  doc.documentElement = el("html", null);
  return doc;
}

// fire calls the element's listeners for `type` with an event object.
export function fire(el: FakeEl, type: string, ev: Record<string, unknown> = {}) {
  for (const fn of el.listeners[type] ?? []) fn({ currentTarget: el, target: el, preventDefault() {}, ...ev });
}
