// A virtual node and the one function that turns it into DOM.
//
// Every view and chart describes its output with h(); render() is the only
// place that creates elements. Text is always a text node and every attribute
// goes through setAttribute, so data from the API is never parsed as HTML —
// there is no innerHTML anywhere in the panel. That also keeps the views and
// charts testable without a browser: a test inspects the tree h() returns.

export const SVG_NS = "http://www.w3.org/2000/svg";

const flat = (children) =>
  children.flat(Infinity).filter((c) => c !== null && c !== undefined && c !== false && c !== true);

// h("button", {class: "btn", on: {click}}, "Save") → {tag, props, children}.
export function h(tag, props, ...children) {
  return { tag, props: props || {}, children: flat(children) };
}

// text collects the text content of a tree: for tests and accessible names.
export function text(node) {
  if (typeof node === "string" || typeof node === "number") return String(node);
  return node.children.map(text).join("");
}

// find returns the first node (depth first) for which test(node) is true.
export function find(node, test) {
  if (typeof node !== "object") return null;
  if (test(node)) return node;
  for (const c of node.children) {
    const hit = find(c, test);
    if (hit) return hit;
  }
  return null;
}

// findAll returns every node for which test(node) is true.
export function findAll(node, test, out = []) {
  if (typeof node !== "object") return out;
  if (test(node)) out.push(node);
  for (const c of node.children) findAll(c, test, out);
  return out;
}

// Properties set as DOM properties, not attributes: their live value matters.
const PROPERTIES = new Set(["value", "checked", "selected", "indeterminate"]);

function setProp(el, key, value) {
  if (key === "on") {
    for (const [type, fn] of Object.entries(value)) el.addEventListener(type, fn);
  } else if (key === "ref") {
    value(el);
  } else if (key === "style") {
    // CSSOM, not the style attribute: allowed under a CSP without 'unsafe-inline'.
    for (const [k, v] of Object.entries(value)) el.style.setProperty(k, String(v));
  } else if (PROPERTIES.has(key)) {
    el[key] = value;
  } else if (value === true) {
    el.setAttribute(key, "");
  } else if (value !== false && value !== null && value !== undefined) {
    el.setAttribute(key, String(value));
  }
}

// render builds the DOM for a node; `ns` is inherited inside <svg>.
export function render(node, doc = globalThis.document, ns = null) {
  if (typeof node === "string" || typeof node === "number") return doc.createTextNode(String(node));
  const space = node.tag === "svg" ? SVG_NS : ns;
  const el = space ? doc.createElementNS(space, node.tag) : doc.createElement(node.tag);
  for (const [k, v] of Object.entries(node.props)) setProp(el, k, v);
  for (const c of node.children) el.appendChild(render(c, doc, space));
  return el;
}

// mount replaces the children of `parent` with the rendered node(s).
export function mount(parent, nodes, doc = globalThis.document) {
  parent.replaceChildren(...[nodes].flat(Infinity).filter((n) => n !== null && n !== undefined && n !== false).map((n) => render(n, doc)));
  return parent;
}

let counter = 0;
// uid makes ids unique on the page (chart patterns, aria wiring).
export const uid = (prefix) => `${prefix}-${++counter}`;
