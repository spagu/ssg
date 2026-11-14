// <ssg-admin-nav current="newsletter-ui"></ssg-admin-nav> — the navigation
// shared by every admin panel of the site (GO-146). One file, no imports, no
// build step. A panel adopts it with one line:
//
//   <script type="module" src="/core/nav.js"></script><ssg-admin-nav current="<module id>"></ssg-admin-nav>
//
// It asks GET /api/core/modules (the `core` worker) and draws: the site's name
// or logo (admin_theme: in .ssg.yaml, GO-152), a link to the admin home page,
// one link per module panel the caller may see with the current one marked,
// and, when signed in through the `auth` worker, who is signed in and a
// sign-out button. Whatever goes wrong — no core, a refusal, a timeout, a
// stylesheet that does not load — it draws NOTHING: no empty bar, no message,
// no thrown error. The panel underneath works exactly as without it.
//
// Shadow DOM, chosen for two reasons: the host panel's own CSS (`a`, `nav`,
// `button` rules) cannot reach in and the bar's cannot leak out; and custom
// properties DO cross the boundary, so --nav-bg, --font-ui… set by the panel
// or by /core/theme.css restyle it (nav.css lists them). The styles come from
// a <link> to /core/nav.css inside the shadow root — a same-origin file, so a
// panel's `style-src 'self'` allows it without 'unsafe-inline'. Nothing is
// drawn before that stylesheet has loaded, so the bar never flashes unstyled.
//
// A public signal for the panel underneath (which should not look inside the
// shadow root): after each drawing the element carries `data-ready` (and
// `data-signout="<path>"` when it shows a sign-out button) and dispatches a
// bubbling, composed `ssg-admin-nav:ready` event with {user, signOut} in its
// detail; when it draws nothing, both attributes are removed and no event is
// sent. The shadow structure stays as it is.
//
// Everything that decides something is a plain exported function (tested in
// node); the element class only wires them to the DOM.

export const API = "/api/core/modules";
export const TIMEOUT_MS = 6000;
// QUIET: core leaves the password challenge out of a refusal to this
// request, so drawing the bar never makes the browser ask for a password.
export const QUIET = { "x-core-quiet": "1" };

export const STRINGS = {
  en: { label: "Site administration", brand: "Site admin", home: "Admin home", modules: "Modules", signedIn: "Signed in as {email}", signOut: "Sign out" },
  pl: { label: "Administracja strony", brand: "Administracja", home: "Strona główna administracji", modules: "Moduły", signedIn: "Zalogowano jako {email}", signOut: "Wyloguj" },
  hi: { label: "साइट प्रशासन", brand: "साइट एडमिन", home: "एडमिन होम", modules: "मॉड्यूल", signedIn: "{email} के रूप में साइन इन", signOut: "साइन आउट" },
  de: { label: "Website-Verwaltung", brand: "Verwaltung", home: "Verwaltungsstartseite", modules: "Module", signedIn: "Angemeldet als {email}", signOut: "Abmelden" },
  fr: { label: "Administration du site", brand: "Administration", home: "Accueil de l'administration", modules: "Modules", signedIn: "Connecté en tant que {email}", signOut: "Se déconnecter" },
};

// pickLang: the host page's <html lang>, by primary subtag, else English.
export function pickLang(raw) {
  const primary = String(raw || "").toLowerCase().split(/[-_]/)[0];
  return Object.hasOwn(STRINGS, primary) ? primary : "en";
}

export const say = (lang, key, vars = {}) => STRINGS[lang][key].replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));

// localPath accepts only a path on this host: one leading slash, no second
// one, no backslash or whitespace. A link from the API that is anything else
// is not drawn (core already refuses them; the bar does not take its word).
export const localPath = (v) => (typeof v === "string" && v.length <= 512 && /^\/(?![/\\])[^\s\\]*$/.test(v) ? v : null);
const shortText = (v, max) => (typeof v === "string" && v.trim() ? v.trim().slice(0, max) : null);

// navModel turns the API answer into what is drawn, or null for "draw
// nothing": an answer that is not one, or nothing to link to.
export function navModel(body, current) {
  if (!body || typeof body !== "object" || !Array.isArray(body.items)) return null;
  const home = localPath(body.home);
  const items = [];
  for (const m of body.items) {
    const href = m && m.id !== "core" ? localPath(m.panel) : null;
    const label = href ? shortText(m.title, 80) : null;
    if (label) items.push({ id: String(m.id), label, href, current: m.id === current });
  }
  if (!items.length && !home) return null;
  const auth = body.mode === "auth";
  const email = auth && body.user ? shortText(body.user.email, 254) : null;
  const brand = body.brand && typeof body.brand === "object" ? body.brand : {};
  return {
    // The bar is a dark surface in both schemes, so a logo made for dark
    // backgrounds is the one it shows when there is one.
    brand: { name: shortText(brand.name, 80), logo: localPath(brand.logoDark) ?? localPath(brand.logo) },
    home: home ? { href: home, current: current === "core" } : null,
    items,
    user: email ? { email } : null,
    signOut: auth ? localPath(body.signOut) : null,
  };
}

// loadModel asks core and never throws: any failure is null. A redirect
// (Access sending the request to its login) is a failure, never followed.
export async function loadModel(fetchFn, lang, current, timeoutMs = TIMEOUT_MS) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const body = await Promise.resolve()
    .then(() => fetchFn(`${API}?lang=${lang}`, { headers: { accept: "application/json", ...QUIET }, credentials: "same-origin", redirect: "manual", signal: controller.signal }))
    .then((res) => (res.ok && (res.headers.get("content-type") || "").includes("json") ? res.json() : null))
    .catch(() => null);
  clearTimeout(timer);
  return navModel(body, current);
}

// el describes a node: {tag, attrs, children}; strings are text.
const el = (tag, attrs, ...children) => ({ tag, attrs: attrs || {}, children: children.flat().filter((c) => c !== null && c !== false) });
const current = (on) => (on ? { "aria-current": "page" } : {});

// view: the tree drawn for a model, in lang.
export function view(model, lang) {
  const name = model.brand.name || say(lang, "brand");
  // A logo stands for the name: its alt text is the name, and it is shown
  // only through <img> (an SVG is never put into the page), in a fixed box
  // that fits any shape (nav.css). If it does not load, the name is shown
  // (brandFallback).
  const brandInner = model.brand.logo ? el("img", { src: model.brand.logo, alt: name, class: "logo" }) : el("span", { class: "name" }, name);
  const links = [
    model.home ? el("li", null, el("a", { href: model.home.href, class: "home", ...current(model.home.current) }, say(lang, "home"))) : null,
    ...model.items.map((m) => el("li", null, el("a", { href: m.href, "data-module": m.id, ...current(m.current) }, m.label))),
  ];
  const who = model.user || model.signOut
    ? el(
        "div",
        { class: "who" },
        model.user ? el("span", { class: "user" }, say(lang, "signedIn", { email: model.user.email })) : null,
        // A plain form: auth's logout accepts a same-origin form POST (its
        // CSRF rule checks Origin / Sec-Fetch-Site), so it works without
        // script and the panel's form-action 'self' allows it.
        model.signOut ? el("form", { method: "post", action: model.signOut, class: "out" }, el("button", { type: "submit" }, say(lang, "signOut"))) : null,
      )
    : null;
  return el(
    "nav",
    { class: "bar", "aria-label": say(lang, "label") },
    el(
      "div",
      { class: "row" },
      model.home ? el("a", { class: "brand", href: model.home.href }, brandInner) : el("span", { class: "brand" }, brandInner),
      el("button", { type: "button", class: "toggle", "aria-expanded": "false", "aria-controls": "ssg-nav-menu" }, say(lang, "modules")),
    ),
    el("div", { class: "menu", id: "ssg-nav-menu" }, el("ul", { class: "links" }, links), who),
  );
}

// build makes DOM from a tree: text is always a text node and attributes go
// through setAttribute — a module's title can never become markup.
export function build(doc, node) {
  if (typeof node === "string") return doc.createTextNode(node);
  const out = doc.createElement(node.tag);
  for (const [k, v] of Object.entries(node.attrs)) out.setAttribute(k, String(v));
  for (const c of node.children) out.appendChild(build(doc, c));
  return out;
}

// brandFallback: when the logo fails to load, the bar shows the name instead
// — a broken-image icon never stands where the site's name should be.
export function brandFallback(doc, root, name) {
  const brand = root.querySelector(".brand");
  const img = brand.querySelector(".logo");
  if (img) img.addEventListener("error", () => brand.replaceChildren(build(doc, el("span", { class: "name" }, name))));
}

// setOpen shows or hides the menu on narrow screens (a disclosure button).
export function setOpen(root, open) {
  root.querySelector(".toggle").setAttribute("aria-expanded", String(open));
  root.querySelector(".menu").classList.toggle("open", open);
}

// onKey: Escape closes an open menu and returns focus to its button.
export function onKey(root, ev) {
  const toggle = root.querySelector(".toggle");
  if (ev.key !== "Escape" || toggle.getAttribute("aria-expanded") !== "true") return;
  setOpen(root, false);
  toggle.focus();
}

// stylesheet resolves true once the shadow root's stylesheet has loaded,
// false when it fails or takes longer than timeoutMs.
export function stylesheet(link, timeoutMs = TIMEOUT_MS) {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve(false), timeoutMs);
    const done = (ok) => () => (clearTimeout(timer), resolve(ok));
    link.addEventListener("load", done(true));
    link.addEventListener("error", done(false));
  });
}

// READY: the event announce dispatches after each drawing.
export const READY = "ssg-admin-nav:ready";

// announce reflects the drawing on the host element (attributes) and tells
// the page (an event); model null means nothing is drawn.
export function announce(host, model, Event = globalThis.CustomEvent) {
  if (!model) {
    host.removeAttribute("data-ready");
    host.removeAttribute("data-signout");
    return;
  }
  host.setAttribute("data-ready", "");
  if (model.signOut) host.setAttribute("data-signout", model.signOut);
  else host.removeAttribute("data-signout");
  host.dispatchEvent(new Event(READY, { bubbles: true, composed: true, detail: { user: model.user, signOut: model.signOut } }));
}

// baseClass: HTMLElement in a browser; a plain class where there is none (a
// test, a server-side import), so loading this file never throws.
export const baseClass = (g = globalThis) => g.HTMLElement ?? class {};
const Base = baseClass();

// REDRAW: an event a page can fire on its document to have the bar ask again
// — the home page does once it has signed in, because with a shared password
// the browser only sends the password after its first challenge, and the bar
// itself never triggers one.
export const REDRAW = "ssg-admin-nav:redraw";

export class AdminNav extends Base {
  static observedAttributes = ["current"];

  connectedCallback() {
    const doc = this.ownerDocument;
    this.redraw ??= () => void this.refresh();
    doc.addEventListener(REDRAW, this.redraw);
    if (globalThis.MutationObserver && !this.watch) {
      // The host page may switch language without reloading (the home page
      // does); a redraw only when the language really changed.
      this.watch = new MutationObserver(() => {
        if (pickLang(doc.documentElement.getAttribute("lang")) !== this.lang) void this.refresh();
      });
      this.watch.observe(doc.documentElement, { attributes: true, attributeFilter: ["lang"] });
    }
    void this.refresh();
  }

  disconnectedCallback() {
    this.ownerDocument.removeEventListener(REDRAW, this.redraw);
    this.watch?.disconnect();
    this.watch = null;
  }

  // Only a change after the first drawing: the parser sets `current` before
  // the element is connected, and connectedCallback draws anyway.
  attributeChangedCallback(_name, before, after) {
    if (this.drawn !== undefined && before !== after && this.isConnected) void this.refresh();
  }

  // refresh draws the bar, or nothing; it never rejects.
  async refresh() {
    const token = (this.drawn = (this.drawn ?? 0) + 1);
    try {
      const doc = this.ownerDocument;
      const lang = (this.lang = pickLang(doc.documentElement.getAttribute("lang")));
      const model = await loadModel((...a) => globalThis.fetch(...a), lang, this.getAttribute("current"));
      if (token !== this.drawn) return;
      const shadow = this.shadowRoot ?? this.attachShadow({ mode: "open" });
      if (!model) {
        this.styled = null;
        announce(this, null);
        return void shadow.replaceChildren();
      }
      if (!this.styled) {
        // Loaded once; a redraw (another language) swaps only the bar.
        const link = build(doc, { tag: "link", attrs: { rel: "stylesheet", href: new URL("nav.css", import.meta.url).href }, children: [] });
        const ready = stylesheet(link);
        shadow.replaceChildren(link);
        const ok = await ready;
        if (token !== this.drawn) return;
        if (!ok) return void (announce(this, null), shadow.replaceChildren());
        this.styled = link;
      }
      const root = build(doc, view(model, lang));
      root.querySelector(".toggle").addEventListener("click", (e) => setOpen(root, e.currentTarget.getAttribute("aria-expanded") !== "true"));
      root.addEventListener("keydown", (e) => onKey(root, e));
      brandFallback(doc, root, model.brand.name || say(lang, "brand"));
      shadow.replaceChildren(this.styled, root);
      announce(this, model);
    } catch {
      // Drawing the bar is a courtesy: a failure leaves the panel as it was.
    }
  }
}

// define registers the element once; a page that loads the file twice, or a
// context without custom elements, is left as it is.
export function define(registry = globalThis.customElements) {
  if (!registry || registry.get("ssg-admin-nav")) return false;
  registry.define("ssg-admin-nav", AdminNav);
  return true;
}
define();
