// The suppression list: addresses no campaign may mail (bounced, complained,
// added by hand, erased). Search, filter by reason, add one, lift one, import
// a CSV of bounces (for SMTP users, whose bounces arrive as mail).

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { suppressionFigure } from "../models/dashboard.js";
import { REASONS, liftNeedsConfirm, readFileText, suppressionName } from "../models/lists.js";
import { btn, busy, chip, confirmDialog, emptyState, errorBlock, loading, pageHead, slot } from "./ui.js";

function addForm(app, done) {
  const { t, api } = app;
  const submit = async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const email = form.elements.email.value.trim();
    const reason = form.elements.reason.value;
    const r = await busy(app, form.querySelector("[type=submit]"), () => api.post("suppressions", { email, reason }));
    if (r) {
      app.notify(t("sup.added", { email, reason: t(`reason.${r.suppression.reason}`) }));
      form.reset();
      done();
    }
  };
  return h(
    "form",
    { class: "inline-form", on: { submit }, "aria-labelledby": "sup-add-title" },
    h("h2", { id: "sup-add-title" }, t("sup.add")),
    h("div", { class: "field" }, h("label", { for: "sup-email" }, t("col.email")), h("input", { id: "sup-email", name: "email", type: "email", required: true, autocomplete: "off" })),
    h("div", { class: "field" }, h("label", { for: "sup-reason" }, t("col.reason")), h("select", { id: "sup-reason", name: "reason", "aria-describedby": "sup-reason-hint" }, ["manual", "hard", "complaint"].map((r) => h("option", { value: r }, t(`reason.${r}`)))), h("p", { id: "sup-reason-hint", class: "hint" }, t("sup.reason.hint"))),
    h("button", { type: "submit", class: "btn btn-primary" }, t("sup.addGo")),
  );
}

function importBox(app, done) {
  const { t, fmt, api } = app;
  const out = slot();
  let csv = null;
  const pick = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    csv = await readFileText(file);
    const p = await busy(app, null, () => api.post("suppressions/import/preview", { csv }));
    if (!p) return;
    out.set(
      h(
        "div",
        { class: "callout info", role: "status" },
        h("p", null, t("sup.import.preview", { count: p.total, fresh: fmt.number(p.counts.new), existing: fmt.number(p.counts.existing), duplicate: fmt.number(p.counts.duplicate), invalid: fmt.number(p.counts.invalid) })),
        p.counts.new
          ? btn(t("sup.import.go", { count: p.counts.new }), async (ev) => {
              const ok = await confirmDialog(app, { title: t("sup.import.confirm", { count: p.counts.new, file: file.name }), body: t("sup.import.body"), confirm: t("sup.import.go", { count: p.counts.new }) });
              if (!ok) return;
              const r = await busy(app, ev.currentTarget, () => api.post("suppressions/import", { csv }));
              if (r) {
                out.set(h("p", { class: "callout ok", role: "status" }, t("sup.import.done", { count: r.imported })));
                done();
              }
            }, { variant: "primary" })
          : null,
      ),
    );
  };
  return h(
    "details",
    { class: "card" },
    h("summary", null, t("sup.import")),
    h("div", { class: "field" }, h("label", { for: "sup-file" }, t("import.file.label")), h("input", { id: "sup-file", type: "file", accept: ".csv,text/csv,text/plain", "aria-describedby": "sup-file-hint", on: { change: pick } }), h("p", { id: "sup-file-hint", class: "hint" }, t("sup.import.hint"))),
    out.node,
  );
}

export default async function suppressions(app, root, route) {
  const { t, fmt, api } = app;
  const reason = REASONS.includes(route.query.reason) ? route.query.reason : null;
  const q = (route.query.q ?? "").trim();
  const list = slot();
  const chart = slot();
  const more = slot();
  // A reload that fails is shown in the view with a Retry, like a first load.
  const reload = () => suppressions(app, root, route).catch((e) => mount(root, errorBlock(app, e, reload)));
  const search = (e) => {
    e.preventDefault();
    const d = Object.fromEntries(new FormData(e.currentTarget));
    app.go(href("suppressions", {}, { reason: d.reason || null, q: d.q.trim() || null }));
  };
  mount(root, [
    pageHead(app, t("sup.title"), { lead: t("sup.lead") }),
    chart.node,
    h(
      "form",
      { class: "filters", role: "search", "aria-label": t("sup.filters"), on: { submit: search } },
      h("div", { class: "field" }, h("label", { for: "sq" }, t("sup.search")), h("input", { id: "sq", name: "q", type: "search", value: q, autocomplete: "off" })),
      h("div", { class: "field" }, h("label", { for: "sr" }, t("col.reason")), h("select", { id: "sr", name: "reason" }, h("option", { value: "" }, t("sup.anyReason")), REASONS.map((r) => h("option", { value: r, selected: r === reason }, t(`reason.${r}`))))),
      h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("action.filter"))),
    ),
    list.node,
    more.node,
    app.can("suppress") || app.can("importSuppressions")
      ? h("div", { class: "two-col" }, app.can("suppress") ? h("section", { class: "card" }, addForm(app, reload)) : null, app.can("importSuppressions") ? importBox(app, reload) : null)
      : null,
  ]);
  list.set(loading(app));
  api.get("stats/suppressions", { days: 90 }).then(
    (s) => chart.set(suppressionFigure({ stats: s, t, fmt, headingLevel: "h2" })),
    (e) => chart.set(errorBlock(app, e)),
  );

  const items = [];
  let cursor = null;
  const lift = (s) => async (e) => {
    const name = suppressionName(s, t);
    const complaint = liftNeedsConfirm(s);
    const ok = await confirmDialog(app, { title: t("sup.lift.title", { email: name }), body: t(complaint ? "sup.lift.complaint" : "sup.lift.body", { email: name }), confirm: t(complaint ? "sup.lift.complaintGo" : "sup.lift.go") });
    if (!ok) return;
    if ((await busy(app, e.currentTarget, () => api.del(`suppressions/${s.emailHash}`, complaint ? { confirm: true } : undefined))) !== undefined) {
      app.notify(t("sup.lifted", { email: name }));
      await reload();
    }
  };
  const table = () =>
    h(
      "div",
      { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "sup-caption" },
      h(
        "table",
        { class: "data list" },
        h("caption", { id: "sup-caption" }, t("sup.caption", { count: items.length })),
        h("thead", null, h("tr", null, ["col.address", "col.reason", "col.source", "col.since", "col.action"].map((k) => h("th", { scope: "col" }, t(k))))),
        h(
          "tbody",
          null,
          items.map((s) =>
            h(
              "tr",
              null,
              h("th", { scope: "row" }, suppressionName(s, t)),
              h("td", { "data-label": t("col.reason") }, chip(app, s.reason, "reason"), s.reason === "soft" ? ` ×${s.count}` : null),
              h("td", { "data-label": t("col.source") }, s.source ?? "—"),
              h("td", { "data-label": t("col.since") }, fmt.date(s.createdAt)),
              h("td", { "data-label": t("col.action") }, app.can("lift") ? h("button", { type: "button", class: "btn btn-quiet", "aria-label": t("sup.lift.for", { email: suppressionName(s, t) }), on: { click: lift(s) } }, t("sup.lift")) : "—"),
            ),
          ),
        ),
      ),
    );
  const page = async () => {
    const r = await api.get("suppressions", { reason, q, cursor, limit: 50 });
    items.push(...r.items);
    cursor = r.nextCursor;
    list.set(items.length ? table() : emptyState({ img: "empty-suppressions", title: t(reason || q ? "sup.none.filtered" : "sup.none"), body: t("sup.none.body") }));
    more.set(cursor ? btn(t("action.loadMore"), (e) => busy(app, e.currentTarget, page)) : []);
  };
  try {
    await page();
  } catch (e) {
    list.set(errorBlock(app, e, reload));
  }
}
