// Service accounts: the identities scripts use (CI, cron), each with roles
// and tokens of its own; a form for a new one.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { act, chip, empty, field, loading, pageHead, tableRegion } from "./ui.js";

export default async function services(app, view) {
  const { t, fmt } = app;
  mount(view, loading(app));
  const list = await app.api.get("admin/service-accounts");
  const rows = list.items.map((a) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("a", { href: href("service", { id: a.id }) }, a.name), a.description ? h("span", { class: "sub" }, a.description) : null),
      h("td", null, chip(t(`service.status.${a.status}`), a.status === "active" ? "ok" : "critical")),
      h("td", null, fmt.date(a.createdAt)),
    ),
  );
  async function create(e) {
    e.preventDefault();
    const r = await act(app, e.submitter, () => app.api.post("admin/service-accounts", { name: e.target.name.value.trim(), description: e.target.description.value.trim() }));
    if (r) {
      app.notify(t("services.created", { name: r.serviceAccount.name }));
      app.go(href("service", { id: r.serviceAccount.id }));
    }
  }
  mount(view, [
    pageHead(app, t("nav.services"), { lead: t("services.lead") }),
    rows.length ? tableRegion(app, t("services.caption"), [t("services.col.name"), t("services.col.status"), t("services.col.created")], rows) : empty(t("services.none")),
    h(
      "form",
      { class: "card form", "aria-labelledby": "new-sa", on: { submit: (e) => void create(e) } },
      h("h2", { id: "new-sa" }, t("services.new")),
      field("sa-name", t("services.name"), h("input", { type: "text", name: "name", required: true, maxlength: "64", pattern: "[a-z][a-z0-9-]*", autocomplete: "off", spellcheck: "false" }), t("roles.nameHint")),
      field("sa-desc", t("services.description"), h("input", { type: "text", name: "description", maxlength: "500", autocomplete: "off" })),
      h("button", { type: "submit", class: "btn btn-primary" }, t("services.create")),
    ),
  ]);
}
