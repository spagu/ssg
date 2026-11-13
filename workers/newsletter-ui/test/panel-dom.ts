// A tiny stand-in for the parts of the DOM the panel's logic modules touch
// (render, the chart toggles, the cursor), so they are tested without a browser.

export interface FakeEl {
  tag: string;
  ns: string | null;
  attrs: Record<string, string>;
  props: Record<string, unknown>;
  style: Record<string, string> & { setProperty(k: string, v: string): void };
  children: (FakeEl | { text: string })[];
  listeners: Record<string, ((e: unknown) => void)[]>;
  classList: { add(c: string): void; has(c: string): boolean };
  ownerDocument: FakeDoc;
  hidden?: boolean;
  textContent?: string;
  setAttribute(k: string, v: string): void;
  getAttribute(k: string): string | null;
  addEventListener(t: string, fn: (e: unknown) => void): void;
  appendChild<T>(c: T): T;
  replaceChildren(...c: (FakeEl | { text: string })[]): void;
}
export type FakeDoc = ReturnType<typeof fakeDoc>;

export function fakeDoc() {
  const byId = new Map<string, FakeEl>();
  const doc = {
    byId,
    createTextNode: (text: string) => ({ text }),
    createElement: (tag: string) => el(tag, null),
    createElementNS: (ns: string, tag: string) => el(tag, ns),
    getElementById: (id: string) => byId.get(id) ?? null,
  };
  function el(tag: string, ns: string | null): FakeEl {
    const classes = new Set<string>();
    const e: FakeEl = {
      tag,
      ns,
      attrs: {},
      props: {},
      children: [],
      listeners: {},
      ownerDocument: doc as FakeDoc,
      style: Object.assign(Object.create(null), { setProperty(k: string, v: string) { (e.style as Record<string, unknown>)[k] = v; } }),
      classList: { add: (c: string) => void classes.add(c), has: (c: string) => classes.has(c) },
      setAttribute(k, v) {
        e.attrs[k] = v;
        if (k === "id") byId.set(v, proxy);
        if (k === "hidden") e.props.hidden = true; // the attribute reflects, as in the DOM
      },
      getAttribute: (k) => e.attrs[k] ?? null,
      addEventListener(t, fn) {
        (e.listeners[t] ??= []).push(fn);
      },
      appendChild(c) {
        e.children.push(c as never);
        return c;
      },
      replaceChildren(...c) {
        e.children = c;
      },
    };
    const proxy: FakeEl = new Proxy(e, {
      set(target, key, value) {
        if (key in target || typeof key === "symbol") (target as never)[key] = value as never;
        else target.props[key as string] = value;
        return true;
      },
      get(target, key) {
        return key in target ? (target as never)[key] : target.props[key as string];
      },
    });
    return proxy;
  }
  return doc;
}

// fire calls the element's listeners for `type` with an event object.
export function fire(el: FakeEl, type: string, ev: Record<string, unknown> = {}) {
  for (const fn of el.listeners[type] ?? []) fn({ currentTarget: el, target: el, preventDefault() {}, ...ev });
}

// walk finds the first fake element matching test.
export function walk(el: FakeEl | { text: string }, test: (e: FakeEl) => boolean): FakeEl | null {
  if ("text" in el && !("tag" in el)) return null;
  const e = el as FakeEl;
  if (test(e)) return e;
  for (const c of e.children) {
    const hit = walk(c, test);
    if (hit) return hit;
  }
  return null;
}
