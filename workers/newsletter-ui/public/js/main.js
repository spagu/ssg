// The panel's entry point: language, theme, the shell's words, and routing.
// DOM glue only — the logic it wires lives in core/, models/ and charts/.

import { ABSENT, applyAdminNav, coverHeight as coverHeightOf, readAdminNav } from "./core/admin-nav.js";
import { createApi } from "./core/api.js";
import { createFormat } from "./core/format.js";
import { readMe, renew } from "./core/session.js";
import { LANGS, createI18n, pickLang, readConfig } from "./core/i18n.js";
import { href, isRoute, parse } from "./core/router.js";
import { LANG_KEY, THEMES, applyTheme, readStored, readTheme, writeStored, writeTheme } from "./core/theme.js";
import { h, mount } from "./core/vnode.js";
import { ALL, account, allows, navFor, viewLacks } from "./models/access.js";
import { errorBlock, pageHead } from "./views/ui.js";

const VIEWS = {
  dashboard: () => import("./views/dashboard.js"),
  subscribers: () => import("./views/subscribers.js"),
  subscriber: () => import("./views/subscriber.js"),
  import: () => import("./views/import.js"),
  campaigns: () => import("./views/campaigns.js"),
  campaignNew: () => import("./views/editor.js"),
  campaign: () => import("./views/campaign.js"),
  suppressions: () => import("./views/suppressions.js"),
  settings: () => import("./views/settings.js"),
  activity: () => import("./views/activity.js"),
  help: () => import("./views/help.js"),
  notFound: () => import("./views/help.js"),
};
const NAV = ["dashboard", "subscribers", "import", "campaigns", "suppressions", "settings", "activity", "help"];

const $ = (id) => document.getElementById(id);
const storage = (() => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
})();
const config = readConfig($("newsletter-admin-config")?.textContent);
const dark = window.matchMedia("(prefers-color-scheme: dark)");

async function strings(lang) {
  const en = (await import("./strings/en.js")).default;
  return lang === "en" ? { en } : { en, [lang]: (await import(`./strings/${lang}.js`)).default };
}

// app.me: who the panel talks for (GO-137, GET api/me); app.can(action) asks
// models/access.js. Until api/me answers, and with the password or Access,
// everything is allowed — the worker enforces every permission anyway.
const app = { me: ALL, can: (action) => allows(app.me, action) };
app.api = createApi({ base: new URL(".", location.href).href, onUnauthorized: () => renew(app.me, { fetch: (...a) => fetch(...a), location }) });
let leave = [];

async function setLanguage(lang) {
  const tables = await strings(lang);
  const fmt = createFormat(lang);
  const i18n = createI18n(lang, tables, config.i18n ?? {}, fmt.number);
  Object.assign(app, { lang, t: i18n.t, has: i18n.has, fmt });
  document.documentElement.lang = lang;
  shell();
}

// shell writes the navigation and preferences in the current language.
function shell() {
  const { t } = app;
  for (const el of document.querySelectorAll(".brand-name")) el.textContent = t("app.name");
  $("skip").textContent = t("app.skip");
  $("menu-toggle").textContent = t("app.menu");
  $("nav").setAttribute("aria-label", t("app.nav"));
  $("prefs").setAttribute("aria-label", t("pref.title"));
  mount(
    $("nav-list"),
    navFor(app.me, NAV).map((name) => h("li", null, h("a", { href: href(name), "data-view": name, class: `nav-${name}` }, t(`nav.${name}`)))),
  );
  const theme = readTheme(storage);
  mount($("prefs"), [
    h("label", { for: "pref-lang" }, t("pref.language")),
    h(
      "select",
      { id: "pref-lang", on: { change: (e) => (writeStored(storage, LANG_KEY, e.target.value), setLanguage(e.target.value).then(route)) } },
      LANGS.map((l) => h("option", { value: l, selected: l === app.lang }, t(`pref.lang.${l}`))),
    ),
    h("label", { for: "pref-theme" }, t("pref.theme")),
    h(
      "select",
      { id: "pref-theme", on: { change: (e) => (writeTheme(storage, e.target.value), applyTheme(document.documentElement, e.target.value, dark.matches)) } },
      THEMES.map((v) => h("option", { value: v, selected: v === theme }, t(`pref.theme.${v}`))),
    ),
    signedIn(),
  ]);
}

// signedIn: in `auth` mode, who is signed in and a sign-out button (a form
// POST to the auth worker, which then sends the browser back here — and on to
// the login page). Nothing in the other modes.
function signedIn() {
  const who = account(app.me);
  if (!who) return null;
  return h(
    "form",
    { class: "account", method: "post", action: who.logoutUrl },
    h("p", null, app.t("account.signedIn", { email: who.email })),
    h("input", { type: "hidden", name: "next", value: location.pathname }),
    h("button", { type: "submit", class: "btn btn-quiet" }, app.t("account.signOut")),
  );
}

// notify shows an action's result in the status region (polite live region).
// A notice belongs to the view it was made in: navigation clears it, unless
// it was set just before the navigation it announces ("Draft created").
let notifiedAt = 0;
app.notify = (message, level = "ok") => {
  notifiedAt = message ? Date.now() : 0;
  mount($("notice"), message ? h("div", { class: `callout ${level}` }, h("p", null, message), h("button", { type: "button", class: "btn btn-quiet", on: { click: () => app.notify("") } }, app.t("action.dismiss"))) : []);
};
// The sticky bar (below 900px) and a sticky notice can cover the top of the
// page. scroll-padding-top keeps a focused control clear of them (WCAG 2.2
// 2.4.11), so it follows their real height: a two-line notice on a phone is
// twice as tall as the CSS default assumes. Set through the CSSOM, which the
// CSP allows (it blocks style attributes, not element.style). The site's
// shared admin bar (GO-146, from `core`) is measured whenever its size changes
// (core/admin-nav.js); absent, nothing changes.
const narrow = window.matchMedia("(max-width: 900px)");
const adminNav = document.querySelector("ssg-admin-nav");
let nav = ABSENT;
function coverHeight() {
  // The bar row plus the sidebar's 10px padding and 1px border; not the
  // sidebar itself, which is the whole open menu while the drawer is open.
  const bar = narrow.matches ? document.querySelector(".bar").offsetHeight + 21 : 0;
  const notice = $("notice").childElementCount ? parseFloat(getComputedStyle($("notice")).top) + $("notice").offsetHeight : 0;
  document.documentElement.style.setProperty("--sticky-cover", `${coverHeightOf({ bar, notice, nav })}px`);
}
function adminNavChanged() {
  nav = readAdminNav(adminNav, getComputedStyle);
  applyAdminNav(document.documentElement, nav);
  coverHeight();
}
if (window.ResizeObserver) {
  new ResizeObserver(coverHeight).observe($("notice"));
  if (adminNav) new ResizeObserver(adminNavChanged).observe(adminNav);
}
narrow.addEventListener?.("change", coverHeight);
app.go = (hash) => (location.hash === hash ? route() : (location.hash = hash));
app.onLeave = (fn) => leave.push(fn);

let current = 0;
async function route() {
  if (!isRoute(location.hash)) return;
  const r = parse(location.hash);
  const token = ++current;
  for (const fn of leave.splice(0)) fn();
  if (Date.now() - notifiedAt > 1500) app.notify("");
  document.body.classList.remove("nav-open");
  $("menu-toggle").setAttribute("aria-expanded", "false");
  for (const a of document.querySelectorAll("#nav-list a")) {
    if (a.dataset.view === r.section) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  const view = $("view");
  // A view the account may not open (GO-137): say which permission is missing.
  const missing = viewLacks(app.me, r.name);
  try {
    if (missing) mount(view, [pageHead(app, app.t("error.permission.title")), errorBlock(app, { kind: "permission", permission: missing }, null)]);
    else await show(r, view, token);
  } catch (e) {
    if (token !== current) return;
    mount(view, errorBlock(app, e, route));
  }
  if (token !== current) return;
  const title = view.querySelector("h1");
  document.title = `${title ? title.textContent : app.t("app.name")} · ${app.t("app.name")}`;
  // Focus the new view's heading so a screen reader announces where we are;
  // not on the very first load, where the browser's own focus is right.
  if (route.started && title) title.focus({ preventScroll: false });
  route.started = true;
}

// show loads a view's module and renders it, unless the route changed meanwhile.
async function show(r, view, token) {
  const mod = await VIEWS[r.name]();
  if (token === current) await mod.default(app, view, r);
}

async function boot() {
  applyTheme(document.documentElement, readTheme(storage), dark.matches);
  app.me = await readMe(app.api, location);
  const lang = pickLang([config.lang, readStored(storage, LANG_KEY), document.documentElement.lang !== "en" ? document.documentElement.lang : null, ...(navigator.languages ?? [navigator.language]), "en"].filter(Boolean));
  await setLanguage(lang);
  $("menu-toggle").addEventListener("click", (e) => {
    const open = document.body.classList.toggle("nav-open");
    e.currentTarget.setAttribute("aria-expanded", String(open));
  });
  document.addEventListener("visibilitychange", () => app.visibility?.());
  window.addEventListener("hashchange", route);
  await route();
}

// Nothing is translated yet if boot itself fails (the strings did not load),
// so this one message is plain English; the router handles everything after.
boot().catch((e) => {
  console.error(e);
  $("view").textContent = "The panel could not start. Reload the page; if it keeps failing, check that its files are deployed.";
});
