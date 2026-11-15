// The designer panel's entry point: who is signed in, language and theme,
// the shell's words, what is loaded and edited (shared by the views), and
// routing. DOM glue only — the logic lives in core/, models/ and panel/.

import { ABSENT, applyAdminNav, readAdminNav } from "./core/admin-nav.js";
import { createApi } from "./core/api.js";
import { createFormat } from "./core/format.js";
import { LANGS, createI18n, pickLang, readConfig } from "./core/i18n.js";
import { applyTheme, createPrefs, THEMES } from "./core/prefs.js";
import { createRouter, isRoute } from "./core/router.js";
import { readMe, renew } from "./core/session.js";
import { h, mount } from "./core/vnode.js";
import { NOBODY, account, capabilities, loginTarget } from "./models/access.js";
import { fromDraft, toDraft } from "./models/controls.js";
import { buildForm } from "./models/form-model.js";
import { createChanges, keyOf } from "./models/settings-changes.js";
import { createDrafts } from "./models/theme-files.js";
import { errorBlock, loading } from "./panel/ui.js";
import { pendingBar } from "./views/pending.js";

const ROUTES = [["options", ""], ["optionsGroup", "options/:group"], ["appearance", "appearance"], ["files", "files"]];
const VIEWS = { options: () => import("./views/options.js"), optionsGroup: () => import("./views/options.js"), appearance: () => import("./views/appearance.js"), files: () => import("./views/files.js") };
const NAV = ["options", "appearance", "files"];
const router = createRouter(ROUTES, { optionsGroup: "options" });

const $ = (id) => document.getElementById(id);
const storage = (() => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
})();
// No override block: this page carries no script without src (module.test.ts);
// readConfig answers {} and the strings files are the only words.
const config = readConfig(document.getElementById("panel-config")?.textContent);
const dark = window.matchMedia("(prefers-color-scheme: dark)");

const app = {
  me: NOBODY,
  caps: capabilities(NOBODY),
  prefs: createPrefs(storage, "ssg-designer"),
  changes: createChanges(),
  drafts: new Map(),
  baselines: new Map(),
  problems: new Map(),
  themeDrafts: createDrafts(),
  files: new Map(),
  settings: null,
  theme: null,
};
// href: "options" has two patterns — the group's is "optionsGroup".
app.href = (name, params = {}, query = {}) => router.href(name === "options" && params.group ? "optionsGroup" : name, params, query);
app.api = createApi({ base: new URL(".", location.href).href, onUnauthorized: () => renew(app.me, { fetch: (...a) => fetch(...a), location, loginTarget }) });

// loadSettings: GET api/settings once (again with reload). keep: carry the
// pending edits over onto the new answer (after a conflict).
app.loadSettings = async ({ reload = false, keep = false } = {}) => {
  if (app.settings && !reload) return app.settings;
  const view = await app.api.get("settings");
  const form = buildForm(view);
  const old = keep ? app.changes.items() : [];
  app.discardSettings();
  for (const c of old) {
    const e = form.entries.get(c.entry.segments.join("."));
    if (!e) continue;
    const k = keyOf(e.segments);
    app.baselines.set(k, fromDraft(toDraft(e.node, e.value)).value);
    if (c.value === null) app.changes.revert(e);
    else (app.changes.set(e, c.value, app.baselines.get(k)), app.drafts.set(k, toDraft(e.node, c.value)));
  }
  app.settings = { view, form };
  app.changed();
  return app.settings;
};
app.discardSettings = () => {
  for (const m of [app.drafts, app.baselines, app.problems]) m.clear();
  app.changes.clear();
  app.changed();
};
app.loadTheme = async ({ reload = false } = {}) => {
  if (app.theme && !reload) return app.theme;
  app.theme = await app.api.get("theme");
  app.files.clear();
  return app.theme;
};
app.readFile = async (path, { reload = false } = {}) => {
  if (!app.files.has(path) || reload) app.files.set(path, await app.api.get(`theme/files/${encodeURIComponent(path)}`));
  return app.files.get(path);
};
app.changed = () => mount($("pending"), pendingBar(app));
app.refresh = () => void route();
app.go = (hash) => (location.hash === hash ? route() : (location.hash = hash));

async function setLanguage(lang) {
  // A language's table: the panel's words plus the names of settings.
  const table = async (l) => ({ ...(await import(`./strings/${l}.js`)).default, ...(await import(`./strings/keys/${l}.js`)).default });
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
  mount($("nav-list"), NAV.map((name) => h("li", null, h("a", { href: app.href(name), "data-view": name }, t(`nav.${name}`)))));
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
  app.changed();
}

// notify: an action's result in the polite live region; navigation clears it
// unless it was set just before the navigation it announces.
let notifiedAt = 0;
app.notify = (message, level = "ok") => {
  notifiedAt = message ? Date.now() : 0;
  mount($("notice"), message ? h("div", { class: `callout ${level}` }, h("p", null, message), h("button", { type: "button", class: "btn btn-quiet", on: { click: () => app.notify("") } }, app.t("action.dismiss"))) : []);
};

const adminNav = document.querySelector("ssg-admin-nav");
const navChanged = () => applyAdminNav(document.documentElement, adminNav ? readAdminNav(adminNav, getComputedStyle) : ABSENT);
if (window.ResizeObserver && adminNav) new ResizeObserver(navChanged).observe(adminNav);

let current = 0;
async function route() {
  if (!isRoute(location.hash)) return;
  const r = router.parse(location.hash);
  const name = VIEWS[r.name] ? r.name : "options";
  const section = r.section && VIEWS[r.section] ? r.section : "options";
  const token = ++current;
  if (Date.now() - notifiedAt > 1500) app.notify("");
  document.body.classList.remove("nav-open");
  $("menu-toggle").setAttribute("aria-expanded", "false");
  for (const a of document.querySelectorAll("#nav-list a")) {
    if (a.dataset.view === section) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  const view = $("view");
  mount(view, loading(app));
  try {
    const mod = await VIEWS[name]();
    if (token === current) await mod.default(app, view, r);
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
  app.me = await readMe(app.api, location, NOBODY);
  app.caps = capabilities(app.me);
  const lang = pickLang([config.lang, app.prefs.read("lang"), ...(navigator.languages ?? [navigator.language]), "en"].filter(Boolean));
  await setLanguage(lang);
  $("menu-toggle").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-expanded", String(document.body.classList.toggle("nav-open"))));
  // Unpublished edits live only in this page: leaving it asks first.
  window.addEventListener("beforeunload", (e) => {
    if (app.changes.size || app.themeDrafts.size) e.preventDefault();
  });
  window.addEventListener("hashchange", () => void route());
  await route();
}

// Nothing is translated yet if boot itself fails, so this one message is
// plain English; the router handles everything after.
boot().catch((e) => {
  console.error(e);
  $("view").textContent = "The designer could not start. Reload the page; if it keeps failing, check that its files are deployed and that you are signed in.";
});
