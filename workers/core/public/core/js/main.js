// The home page's entry point: DOM glue only. Language, theme, mounting what
// page.js builds, keeping focus where it was across a redraw, and announcing
// a refresh. Every decision lives in tested modules (page.js, model.js, the
// views, chart.js); this file wires them to the document.

import { createApi } from "./api.js";
import { createFormat } from "./format.js";
import { routeOf } from "./route.js";
import { createManage } from "./manage-state.js";
import { createI18n, pickLang, readConfig } from "./i18n.js";
import { loadAll, pageTree } from "./page.js";
import { LANG_KEY, applyTheme, readStored, readTheme, writeStored, writeTheme } from "./prefs.js";
import { mount } from "./vnode.js";

const $ = (id) => document.getElementById(id);
const storage = (() => {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
})();
const config = readConfig($("core-admin-config")?.textContent);
const dark = window.matchMedia("(prefers-color-scheme: dark)");
const api = createApi();
const app = { config, here: location.pathname, now: () => Date.now(), theme: readTheme(storage), route: routeOf(location.hash) };
let data = null;
let busy = false;
let asked = 0;

// The Modules view (GO-149) keeps its own state; changes announce through
// the page's polite live region and make the shared bar ask again.
app.manage = createManage({
  api,
  app,
  redraw: () => draw(),
  announce: (text) => void ($("notice").textContent = text),
  changed: () => document.dispatchEvent(new Event("ssg-admin-nav:redraw")),
});

async function strings(lang) {
  const en = (await import("./strings/en.js")).default;
  return lang === "en" ? { en } : { en, [lang]: (await import(`./strings/${lang}.js`)).default };
}

// draw re-renders the page; the element that had focus (by id) gets it back.
function draw() {
  const focused = document.activeElement?.id;
  mount($("view"), pageTree(app, data, busy));
  if (focused) $(focused)?.focus();
}

async function setLanguage(lang) {
  const fmt = createFormat(lang);
  const i18n = createI18n(lang, await strings(lang), config.i18n ?? {}, fmt.number);
  Object.assign(app, { lang, t: i18n.t, fmt });
  document.documentElement.lang = lang;
  document.title = i18n.t("app.title");
  $("skip").textContent = i18n.t("app.skip");
}

async function refresh() {
  const token = ++asked;
  busy = true;
  draw();
  const got = await loadAll(api, app.lang);
  if (token !== asked) return;
  data = got;
  busy = false;
  draw();
  // Only the home page announces its refresh: on another view the live region
  // carries that view's own sentences (the Modules view's moves and saves).
  if (app.route === "home") $("notice").textContent = got.error ? "" : app.t("updated", { time: app.fmt.time(new Date(app.now()).toISOString()) });
  // Signed in now: the shared bar may ask again (see REDRAW in /core/nav.js).
  if (!got.error) document.dispatchEvent(new Event("ssg-admin-nav:redraw"));
}

app.refresh = () => void refresh();
app.setTheme = (theme) => {
  writeTheme(storage, theme);
  app.theme = theme;
  applyTheme(document.documentElement, theme, dark.matches);
};
// Titles and sentences come from core in the asked language, so a new
// language asks again.
app.setLang = (lang) => {
  writeStored(storage, LANG_KEY, lang);
  // A language whose strings fail to load leaves the page as it was.
  void setLanguage(lang).then(() => Promise.all([refresh(), app.route === "modules" ? app.manage.load() : null]), () => undefined);
};

// The Appearance view is /core/#appearance: switching draws from the answers
// already loaded (nothing is asked again) and moves focus to the new heading.
// The Modules view (/core/#modules) asks for its own answer each time it opens.
window.addEventListener("hashchange", () => {
  app.route = routeOf(location.hash);
  draw();
  document.querySelector("h1")?.focus();
  if (app.route === "modules") void app.manage.load();
});

async function boot() {
  applyTheme(document.documentElement, app.theme, dark.matches);
  const lang = pickLang([config.lang, readStored(storage, LANG_KEY), ...(navigator.languages ?? [navigator.language]), "en"].filter(Boolean));
  await setLanguage(lang);
  await Promise.all([refresh(), app.route === "modules" ? app.manage.load() : null]);
}

// Nothing is translated if boot itself fails (the strings did not load), so
// this one message is plain English.
boot().catch(() => {
  $("view").textContent = "The admin home page could not start. Reload the page; if it keeps failing, check that the core worker's files are deployed.";
});
