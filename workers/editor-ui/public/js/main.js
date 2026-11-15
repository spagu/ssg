// The editor panel's entry point: who is signed in, language and theme, the
// shell's words, the drafts store, and routing. DOM glue only — the logic
// lives in core/, models/, store/ and panel/.

import { ABSENT, applyAdminNav, readAdminNav } from "./core/admin-nav.js";
import { createApi } from "./core/api.js";
import { createFormat } from "./core/format.js";
import { LANGS, createI18n, pickLang, readConfig } from "./core/i18n.js";
import { applyTheme, createPrefs, THEMES } from "./core/prefs.js";
import { createRouter, isRoute } from "./core/router.js";
import { readMe, renew } from "./core/session.js";
import { h, mount } from "./core/vnode.js";
import { NOBODY, account, capabilities, loginTarget } from "./models/access.js";
import { errorBlock, loading } from "./panel/ui.js";
import { openStore } from "./store/drafts-db.js";

const ROUTES = [["list", ""], ["edit", "edit/:path"], ["new", "new/:key"]];
const VIEWS = { list: () => import("./views/list.js"), edit: () => import("./views/editor.js"), new: () => import("./views/editor.js") };
const router = createRouter(ROUTES, { edit: "list", new: "list" });

const $ = (id) => document.getElementById(id);
const storage = (() => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
})();
// No override block: this page carries no script without src; readConfig
// answers {} and the strings files are the only words.
const config = readConfig($("panel-config")?.textContent);
const dark = window.matchMedia("(prefers-color-scheme: dark)");

const app = { me: NOBODY, caps: capabilities(NOBODY), prefs: createPrefs(storage, "ssg-editor"), storage, site: null, view: null };
app.href = (name, params = {}, query = {}) => router.href(name, params, query);
app.api = createApi({ base: new URL(".", location.href).href, onUnauthorized: () => renew(app.me, { fetch: (...a) => fetch(...a), location, loginTarget }) });
// loadSite: GET api/site once — sections, languages, media folder, limits.
app.loadSite = async () => (app.site ??= await app.api.get("site"));
app.go = (hash) => (location.hash === hash ? route() : (location.hash = hash));
app.refresh = () => void route();

async function setLanguage(lang) {
  const table = async (l) => (await import(`./strings/${l}.js`)).default;
  const en = await table("en");
  const tables = lang === "en" ? { en } : { en, [lang]: await table(lang) };
  app.fmt = createFormat(lang);
  Object.assign(app, createI18n(lang, tables, config.i18n ?? {}, app.fmt.number));
  document.documentElement.lang = lang;
  shell();
}

// shell writes the navigation, preferences and account in the language.
function shell() {
  const { t } = app;
  for (const el of document.querySelectorAll("[data-t]")) el.textContent = t(el.dataset.t);
  $("nav").setAttribute("aria-label", t("app.nav"));
  $("prefs").setAttribute("aria-label", t("pref.title"));
  mount($("nav-list"), h("li", null, h("a", { href: app.href("list"), "data-view": "list" }, t("nav.list"))));
  const who = account(app.me);
  mount($("prefs"), [
    h("label", { for: "pref-lang" }, t("pref.language")),
    h("select", { id: "pref-lang", on: { change: (e) => (app.prefs.write("lang", e.target.value), void setLanguage(e.target.value).then(route)) } }, LANGS.map((l) => h("option", { value: l, selected: l === app.lang }, t(`pref.lang.${l}`)))),
    h("label", { for: "pref-theme" }, t("pref.theme")),
    h("select", { id: "pref-theme", on: { change: (e) => (app.prefs.setTheme(e.target.value), applyTheme(document.documentElement, e.target.value, dark.matches)) } }, THEMES.map((v) => h("option", { value: v, selected: v === app.prefs.theme() }, t(`pref.theme.${v}`)))),
    who
      ? h("form", { class: "account", method: "post", action: who.logoutUrl }, h("p", null, t("account.signedIn", { email: who.email })), h("input", { type: "hidden", name: "next", value: location.pathname }), h("button", { type: "submit", class: "btn btn-quiet" }, t("account.signOut")))
      : null,
  ]);
}

// notify: an action's result in the polite live region; navigation clears it
// unless it was set just before the navigation it announces.
let notifiedAt = 0;
app.notify = (message, level = "ok") => {
  notifiedAt = message ? Date.now() : 0;
  const body = Array.isArray(message) ? message : [message];
  mount($("notice"), message ? h("div", { class: `callout ${level}` }, h("p", null, body), h("button", { type: "button", class: "btn btn-quiet", on: { click: () => app.notify("") } }, app.t("action.dismiss"))) : []);
};

const adminNav = document.querySelector("ssg-admin-nav");
const navChanged = () => applyAdminNav(document.documentElement, adminNav ? readAdminNav(adminNav, getComputedStyle) : ABSENT);
if (window.ResizeObserver && adminNav) new ResizeObserver(navChanged).observe(adminNav);

let current = 0;
async function route() {
  if (!isRoute(location.hash)) return;
  // The open editor saves its draft before the view changes.
  if (app.view?.leave) await app.view.leave().catch((e) => console.error(e));
  app.view = null;
  const r = router.parse(location.hash);
  const name = VIEWS[r.name] ? r.name : "list";
  const token = ++current;
  if (Date.now() - notifiedAt > 1500) app.notify("");
  document.body.classList.remove("nav-open");
  $("menu-toggle").setAttribute("aria-expanded", "false");
  for (const a of document.querySelectorAll("#nav-list a")) a.setAttribute("aria-current", "page");
  const view = $("view");
  mount(view, loading(app));
  try {
    const mod = await VIEWS[name]();
    if (token === current) app.view = (await mod.default(app, view, { ...r, name })) ?? null;
  } catch (e) {
    if (token !== current) return;
    mount(view, errorBlock(app, e, route));
  }
  if (token !== current) return;
  const title = view.querySelector("h1");
  document.title = `${title ? title.textContent : app.t("app.name")} \u00b7 ${app.t("app.name")}`;
  if (route.started && title) title.focus({ preventScroll: false });
  route.started = true;
}

async function boot() {
  applyTheme(document.documentElement, app.prefs.theme(), dark.matches);
  app.store = await openStore();
  app.me = await readMe(app.api, location, NOBODY);
  app.caps = capabilities(app.me);
  const lang = pickLang([config.lang, app.prefs.read("lang"), ...(navigator.languages ?? [navigator.language]), "en"].filter(Boolean));
  await setLanguage(lang);
  $("menu-toggle").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-expanded", String(document.body.classList.toggle("nav-open"))));
  // Leaving the page: the open draft is written at once (a synchronous copy
  // too), and a draft not yet stored asks first.
  window.addEventListener("beforeunload", (e) => {
    if (app.view?.unsaved?.()) e.preventDefault();
    app.view?.rescue?.();
  });
  window.addEventListener("pagehide", () => app.view?.rescue?.());
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") void app.view?.save?.().catch((e) => console.error(e));
  });
  window.addEventListener("hashchange", () => void route());
  await route();
}

// Nothing is translated yet if boot itself fails, so this one message is
// plain English; the router handles everything after.
boot().catch((e) => {
  console.error(e);
  $("view").textContent = "The editor could not start. Reload the page; if it keeps failing, check that its files are deployed and that you are signed in.";
});
