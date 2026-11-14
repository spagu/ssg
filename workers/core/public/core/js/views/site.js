// "Site": the build (from the site's own build.json, or how to publish one),
// the sign-in mode, the modules' versions and manifest contracts, and every
// configuration problem a module reports — without a value of any secret
// (core never has one to show, and summaries name what is missing).

import { h } from "../vnode.js";
import { authHint, buildLines, modeKey, statusKey } from "../model.js";

export function siteSection(app, info, items) {
  const { t, fmt } = app;
  const user = info.user ? h("p", null, t("site.user", { email: info.user.email })) : null;
  return h(
    "section",
    { class: "site", "aria-labelledby": "site-title" },
    h("h2", { id: "site-title" }, t("site.title")),
    h(
      "div",
      { class: "site-grid" },
      h("div", { class: "card" }, h("h3", null, t("site.build")), buildLines(info, fmt, app.now()).map(([key, vars]) => h("p", null, t(key, vars)))),
      h(
        "div",
        { class: "card" },
        h("h3", null, t("site.signin")),
        h("p", null, t(modeKey(info.mode))),
        user,
        authHint(info.mode, items) ? h("p", { class: "muted" }, t("site.hint")) : null,
      ),
    ),
    h(
      "div",
      { class: "card" },
      h("h3", { id: "problems-title" }, t("site.problems")),
      info.problems.length
        ? h("ul", { class: "plain", "aria-labelledby": "problems-title" }, info.problems.map((p) => h("li", null, h("strong", null, p.title), ": ", h("span", null, p.text), " ", h("code", null, p.code))))
        : h("p", null, t("site.problems.none")),
    ),
    h(
      "div",
      { class: "table-scroll" },
      h(
        "table",
        { class: "data list" },
        h("caption", null, t("site.versions")),
        h("thead", null, h("tr", null, ["module", "version", "contract", "status"].map((c) => h("th", { scope: "col" }, t(`col.${c}`))))),
        h(
          "tbody",
          null,
          info.modules.map((m) =>
            h(
              "tr",
              null,
              h("th", { scope: "row" }, m.title),
              h("td", { "data-label": t("col.version") }, m.version ?? "—"),
              h("td", { "data-label": t("col.contract") }, m.contract === null ? "—" : String(m.contract)),
              h("td", { "data-label": t("col.status") }, t(statusKey(m.status))),
            ),
          ),
        ),
      ),
    ),
    h("p", { class: "note" }, t("site.registry", { count: info.registry.modules, format: info.registry.format })),
    // GO-149: the modules' visibility, order and labels; GO-152: the panels'
    // look, read-only.
    h("p", null, h("a", { href: "#modules", id: "manage-link" }, t("manage.link"))),
    h("p", null, h("a", { href: "#appearance", id: "appearance-link" }, t("appearance.link"))),
  );
}
