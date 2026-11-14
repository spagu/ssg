// The accounts admin panel's entry point (GO-138): what the shell's data
// block says about the account, language, theme, the navigation, routing and
// the step-up flow. DOM glue only — the logic lives in core/ and models/.

import { createApi } from "./core/api.js";
import { createFormat } from "./core/format.js";
import { LANGS, createI18n, pickLang, readConfig } from "./core/i18n.js";
import { LANG_KEY, THEMES, THEME_KEY, applyTheme, readStored, readTheme, writeStored } from "./core/prefs.js";
import { href, isRoute, parse } from "./core/router.js";
import { h, mount } from "./core/vnode.js";
import { home, navFor, shown, viewLacks } from "./models/access.js";
import { arrival, next, stepUpHref } from "./models/stepup.js";
import { confirmDialog, errorBlock, pageHead } from "./views/ui.js";

const VIEWS = {
  people: () => import("./views/people.js"),
  person: () => import("./views/person.js"),
  invitations: () => import("./views/invitations.js"),
  groups: () => import("./views/groups.js"),
  group: () => import("./views/group.js"),
  roles: () => import("./views/roles.js"),
  role: () => import("./views/role.js"),
  matrix: () => import("./views/matrix.js"),
  sessions: () => import("./views/sessions.js"),
  services: () => import("./views/services.js"),
  service: () => import("./views/service.js"),
  mappings: () => import("./views/mappings.js"),
  audit: () => import("./views/audit.js"),
  status: () => import("./views/status.js"),
};

const $ = (id) => document.getElementById(id);
const storage = (() => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
})();
const config = readConfig($("auth-admin-config")?.textContent);
const dark = window.matchMedia("(prefers-color-scheme: dark)");
const app = {
  me: { perms: [], access: "absent", mfa: "off", mail: "none", ...config },
  api: createApi(),
  stepState: "idle",
};
let current = 0;

// loginHref: the sign-in page, coming back to this view.
app.loginHref = () => `${app.me.loginUrl || "/auth/login"}?next=${encodeURIComponent(location.pathname + location.hash)}`;

// registry: the known permissions (GET /api/auth/permissions), asked once.
let registry = null;
app.registry = () => (registry ??= app.api.get("permissions").catch((e) => ((registry = null), Promise.reject(e))));

async function setLanguage(lang) {
  const en = (await import("./strings/en.js")).default;
  const tables = lang === "en" ? { en } : { en, [lang]: (await import(`./strings/${lang}.js`)).default };
  const fmt = createFormat(lang);
  const i18n = createI18n(lang, tables, fmt.number);
  Object.assign(app, { lang, t: i18n.t, has: i18n.has, fmt });
  document.documentElement.lang = lang;
  shell();
}

// shell writes the navigation and the preferences in the current language.
function shell() {
  const { t } = app;
  mount($("nav-list"), navFor(app.me).map((name) => h("li", null, h("a", { href: href(name), "data-view": name }, t(`nav.${name}`)))));
  const theme = readTheme(storage);
  mount($("prefs"), [
    h("label", { for: "pref-lang" }, t("pref.language")),
    h("select", { id: "pref-lang", on: { change: (e) => void changeLanguage(e.target.value) } }, LANGS.map((l) => h("option", { value: l, selected: l === app.lang, lang: l }, t(`pref.lang.${l}`)))),
    h("label", { for: "pref-theme" }, t("pref.theme")),
    h(
      "select",
      { id: "pref-theme", on: { change: (e) => (writeStored(storage, THEME_KEY, e.target.value === "auto" ? null : e.target.value), applyTheme(document.documentElement, e.target.value, dark.matches)) } },
      THEMES.map((v) => h("option", { value: v, selected: v === theme }, t(`pref.theme.${v}`))),
    ),
  ]);
}

async function changeLanguage(lang) {
  writeStored(storage, LANG_KEY, lang);
  await setLanguage(lang);
  await route();
}

// notify: an action's result in the status region (a polite live region).
app.notify = (message, level = "ok") => {
  mount($("notice"), message ? h("div", { class: `callout ${level}` }, h("p", null, message), h("button", { type: "button", class: "btn btn-quiet", on: { click: () => app.notify("") } }, app.t("action.dismiss"))) : []);
};
app.go = (hash) => (location.hash === hash ? route() : void (location.hash = hash));

// stepUp: an action was refused for want of a recent second factor. Say so,
// and on consent go to the step-up page, which brings the browser back to
// this view — where nothing has been done yet (models/stepup.js).
app.stepUp = async (err) => {
  app.stepState = next(app.stepState, "refused");
  const minutes = err.body?.minutes ?? app.me.stepUpMinutes ?? 10;
  const ok = await confirmDialog(app, { title: app.t("stepup.title"), body: app.t("stepup.body", { count: minutes }), confirm: app.t("stepup.go"), danger: false });
  app.stepState = next(app.stepState, ok ? "go" : "cancel");
  if (ok) location.assign(stepUpHref(err.body?.stepUpUrl || app.me.stepUpUrl || "/auth/mfa/step-up", location.pathname, location.hash));
};

async function route() {
  if (!isRoute(location.hash)) return;
  const r = parse(location.hash);
  const token = ++current;
  document.body.classList.remove("nav-open");
  $("menu-toggle").setAttribute("aria-expanded", "false");
  if (r.name === "home") return void location.replace(href(home(app.me)));
  for (const a of document.querySelectorAll("#nav-list a")) {
    if (a.dataset.view === r.section) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  const view = $("view");
  const missing = viewLacks(app.me, r.name);
  try {
    if (missing || !shown(app.me, r.name)) mount(view, [pageHead(app, app.t("error.permission.title")), errorBlock(app, { kind: "permission", permission: missing ?? "", code: missing ? null : "access_absent" }, null)]);
    else if (r.name === "notFound") mount(view, [pageHead(app, app.t("notFound.title")), h("p", null, app.t("notFound.body")), h("p", null, h("a", { href: href(home(app.me)) }, app.t("notFound.home")))]);
    else {
      const mod = await VIEWS[r.name]();
      if (token === current) await mod.default(app, view, r);
    }
  } catch (e) {
    if (token !== current) return;
    mount(view, errorBlock(app, e, () => void route()));
  }
  if (token !== current) return;
  const title = view.querySelector("h1");
  document.title = `${title ? title.textContent : app.t("app.name")} · ${app.t("app.name")}`;
  if (route.started && title) title.focus();
  route.started = true;
}

async function boot() {
  applyTheme(document.documentElement, readTheme(storage), dark.matches);
  const back = arrival(location.hash);
  if (back.returned) history.replaceState(null, "", location.pathname + location.search + back.hash);
  await setLanguage(pickLang([readStored(storage, LANG_KEY), config.lang, "en"].filter(Boolean)));
  if (back.returned) {
    app.stepState = "returned";
    app.notify(app.t("stepup.returned"));
  }
  $("menu-toggle").addEventListener("click", (e) => {
    const open = document.body.classList.toggle("nav-open");
    e.currentTarget.setAttribute("aria-expanded", String(open));
  });
  window.addEventListener("hashchange", () => void route());
  await route();
}

// Nothing is translated if boot itself fails (the strings did not load), so
// this one message is plain English.
boot().catch((e) => {
  console.error(e);
  $("view").textContent = "The accounts admin could not start. Reload the page; if it keeps failing, check that its files are deployed.";
});
