// Subscribers: filter, search, page through; CSV export; mail the
// confirmation to pending imports. The filters live in the URL
// (#/subscribers?status=pending&q=…), so a filtered list can be bookmarked.

import { href } from "../core/router.js";
import { runConfirmations } from "../core/tasks.js";
import { h, mount } from "../core/vnode.js";
import { SUB_STATUSES, cleanFilters } from "../models/lists.js";
import { btn, busy, chip, confirmDialog, emptyState, errorBlock, loading, pageHead, slot } from "./ui.js";

function row(app, s) {
  const { t, fmt } = app;
  return h(
    "tr",
    null,
    h("th", { scope: "row" }, h("a", { href: href("subscriber", { id: s.id }) }, s.email)),
    h("td", { "data-label": t("col.status") }, chip(app, s.status)),
    h("td", { "data-label": t("col.language") }, s.language ?? "—"),
    h("td", { "data-label": t("col.tags") }, s.tags.length ? s.tags.join(", ") : "—"),
    h("td", { "data-label": t("col.signedUp") }, fmt.date(s.createdAt)),
  );
}

function filtersForm(app, f) {
  const { t } = app;
  const submit = (e) => {
    e.preventDefault();
    const d = new FormData(e.currentTarget);
    app.go(href("subscribers", {}, cleanFilters(Object.fromEntries(d))));
  };
  return h(
    "form",
    { class: "filters", role: "search", "aria-label": t("subs.filters"), on: { submit } },
    h("div", { class: "field" }, h("label", { for: "f-q" }, t("subs.search")), h("input", { id: "f-q", name: "q", type: "search", value: f.q ?? "", autocomplete: "off", placeholder: "anna@" })),
    h(
      "div",
      { class: "field" },
      h("label", { for: "f-status" }, t("col.status")),
      h("select", { id: "f-status", name: "status" }, h("option", { value: "" }, t("subs.anyStatus")), SUB_STATUSES.map((s) => h("option", { value: s, selected: f.status === s }, t(`status.${s}`)))),
    ),
    h("div", { class: "field narrow" }, h("label", { for: "f-lang" }, t("col.language")), h("input", { id: "f-lang", name: "language", value: f.language ?? "", placeholder: "pl", autocomplete: "off" })),
    h("div", { class: "field narrow" }, h("label", { for: "f-tag" }, t("subs.tag")), h("input", { id: "f-tag", name: "tag", value: f.tag ?? "", placeholder: "news", autocomplete: "off" })),
    h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("action.filter")), Object.keys(f).length ? h("a", { href: href("subscribers"), class: "btn btn-quiet" }, t("action.clear")) : null),
  );
}

// confirmRunner: mails the double opt-in confirmation to every pending
// subscriber never mailed, batch by batch, showing progress as it goes.
function confirmRunner(app) {
  const { t, fmt } = app;
  const out = slot("runner");
  const signal = { aborted: false };
  app.onLeave(() => (signal.aborted = true));
  const start = async (e) => {
    const ok = await confirmDialog(app, { title: t("confirm.run.title"), body: t("confirm.run.body"), confirm: t("confirm.run.go"), danger: false });
    if (!ok) return;
    const button = e.target;
    button.disabled = true;
    const show = (p) =>
      out.set(
        h(
          "div",
          { class: "callout info", role: "status" },
          h("p", null, t("confirm.run.progress", { sent: fmt.number(p.sent), failed: fmt.number(p.failed), skipped: fmt.number(p.skipped), remaining: p.remaining === null ? "…" : fmt.number(p.remaining) })),
          p.waiting ? h("p", null, t("confirm.run.waiting", { seconds: p.waiting })) : null,
          !p.done ? btn(t("action.stop"), () => (signal.aborted = true)) : null,
        ),
      );
    try {
      const total = await runConfirmations(app.api, { onProgress: show, signal });
      out.set(
        h(
          "div",
          { class: `callout ${total.failed ? "warning" : "ok"}`, role: "status" },
          h("p", null, t(total.done ? "confirm.run.done" : "confirm.run.stopped", { sent: fmt.number(total.sent), failed: fmt.number(total.failed), skipped: fmt.number(total.skipped) })),
          total.failures.length ? h("ul", { class: "failures" }, total.failures.slice(0, 10).map((f) => h("li", null, h("a", { href: href("subscriber", { id: f.id }) }, f.id.slice(0, 8)), " — ", f.detail))) : null,
        ),
      );
    } catch (err) {
      out.set(errorBlock(app, err));
    } finally {
      button.disabled = false;
    }
  };
  return { node: out.node, button: btn(t("confirm.run.button"), start) };
}

export default async function subscribers(app, root, route) {
  const { t, api } = app;
  const f = cleanFilters(route.query);
  const list = slot("list");
  const more = slot("more");
  const runner = confirmRunner(app);
  const exportStatus = f.status ?? "confirmed";
  mount(root, [
    pageHead(app, t("subs.title"), {
      lead: t("subs.lead"),
      actions: [
        h("a", { class: "btn", href: api.url("subscribers.csv", { status: exportStatus }), download: "" }, t("subs.export", { status: t(`status.${exportStatus}`) })),
        h("a", { class: "btn", href: href("import") }, t("nav.import")),
        runner.button,
      ],
    }),
    runner.node,
    filtersForm(app, f),
    list.node,
    more.node,
  ]);
  list.set(loading(app));

  const rows = [];
  let cursor = null;
  const table = () =>
    h(
      "div",
      { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "subs-caption" },
      h(
        "table",
        { class: "data list" },
        h("caption", { id: "subs-caption" }, t("subs.caption", { count: rows.length })),
        h("thead", null, h("tr", null, ["col.email", "col.status", "col.language", "col.tags", "col.signedUp"].map((k) => h("th", { scope: "col" }, t(k))))),
        h("tbody", null, rows.map((s) => row(app, s))),
      ),
    );
  const page = async (button) => {
    const r = await api.get("subscribers", { ...f, cursor, limit: 50 });
    rows.push(...r.items);
    cursor = r.nextCursor;
    if (!rows.length) {
      const filtered = Object.keys(f).length > 0;
      list.set(
        emptyState({
          img: "empty-subscribers",
          title: t(filtered ? "subs.none.filtered" : "subs.none"),
          body: t(filtered ? "subs.none.filtered.body" : "subs.none.body"),
          action: h("a", { class: "btn btn-primary", href: filtered ? href("subscribers") : href("import") }, t(filtered ? "action.clear" : "nav.import")),
        }),
      );
    } else list.set(table());
    more.set(cursor ? h("button", { type: "button", class: "btn", on: { click: (e) => loadMore(e.currentTarget) } }, t("action.loadMore")) : rows.length ? h("p", { class: "muted" }, t("list.end")) : []);
    if (button) app.notify(t("list.loaded", { count: r.items.length }));
  };
  const loadMore = (button) => busy(app, button, () => page(button));
  try {
    await page(null);
  } catch (e) {
    list.set(errorBlock(app, e, () => subscribers(app, root, route)));
  }
}
