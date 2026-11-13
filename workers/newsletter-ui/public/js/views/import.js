// Import a subscriber list exported from any mailing tool: pick the file,
// preview what the worker detected (delimiter, which column is which, what
// every row will become), correct the mapping if needed, then run it. The
// file stays in this page's memory only — never in browser storage.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { CLASSES, ROLES, SKIPS, delimiterKey, duplicateColumn, headersOf, ignoredKey, importBody, mappingFrom, needsConsent, readFileText } from "../models/lists.js";
import { busy, confirmDialog, emptyState, errorBlock, errorText, pageHead, slot } from "./ui.js";

const counts = (app, list) =>
  h("ul", { class: "counts" }, list.map(([key, n]) => h("li", { class: n ? "" : "zero" }, h("span", { class: "count-n" }, app.fmt.number(n)), h("span", null, app.t(`import.${key}`)))));

function mappingTable(app, detected, chosen, onChange) {
  const { t } = app;
  const headers = headersOf(detected);
  return h(
    "div",
    { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "map-caption" },
    h(
      "table",
      { class: "data mapping" },
      h("caption", { id: "map-caption" }, t("import.mapping")),
      h("thead", null, h("tr", null, h("th", { scope: "col" }, t("col.role")), h("th", { scope: "col" }, t("col.column")))),
      h(
        "tbody",
        null,
        ROLES.map((role) =>
          h(
            "tr",
            null,
            h("th", { scope: "row" }, h("label", { for: `map-${role}` }, t(`role.${role}`))),
            h(
              "td",
              { "data-label": t("col.column") },
              h(
                "select",
                { id: `map-${role}`, on: { change: (e) => onChange(role, e.target.value) } },
                h("option", { value: "", selected: !(chosen[role] ?? detected.columns[role]) }, t("import.noColumn")),
                headers.map((hd) => h("option", { value: hd, selected: (role in chosen ? chosen[role] : detected.columns[role]) === hd }, hd)),
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

function previewBlock(app, p, state, rerun, run) {
  const { t, fmt } = app;
  const d = p.detected;
  const ignored = d.ignored.length ? h("ul", { class: "plain" }, d.ignored.map((i) => h("li", null, h("strong", null, i.header), " — ", t(ignoredKey(i.reason), { reason: i.reason.replace(/^ignored: /, "") })))) : h("p", { class: "muted" }, t("import.noIgnored"));
  return h(
    "section",
    { class: "card", "aria-labelledby": "pv-title" },
    h("h2", { id: "pv-title" }, t("import.preview", { count: p.total })),
    h("p", null, t(delimiterKey(d.delimiter)), " · ", t(d.headerless ? "import.headerless" : "import.header")),
    d.ambiguous.map((a) => h("p", { class: "callout info" }, t("import.ambiguous", { role: t(`role.${a.role}`), chosen: a.chosen, also: fmt.list(a.alsoMatched) }))),
    mappingTable(app, d, state.chosen, (role, v) => (state.chosen[role] = v)),
    h("button", { type: "button", class: "btn", on: { click: (e) => rerun(e.currentTarget) } }, t("import.applyMapping")),
    h("h3", null, t("import.ignored")),
    ignored,
    h("h3", null, t("import.willDo")),
    counts(app, CLASSES.map((k) => [k, p.classCounts[k]])),
    p.classCounts.toSuppressExisting ? h("p", null, t("import.toSuppressExisting", { count: p.classCounts.toSuppressExisting })) : null,
    h("h3", null, t("import.skipped")),
    counts(app, SKIPS.map((k) => [k, p.counts[k]])),
    p.invalid.length ? h("details", null, h("summary", null, t("import.invalidList", { count: p.counts.invalid })), h("ul", { class: "plain" }, p.invalid.map((i) => h("li", null, t("import.line", { line: i.line }), " — ", t(`import.invalid.${i.reason}`), ": ", i.value)))) : null,
    p.dateWarningCount ? h("details", null, h("summary", null, t("import.dateWarnings", { count: p.dateWarningCount })), h("ul", { class: "plain" }, p.dateWarnings.map((w) => h("li", null, t("import.line", { line: w.line }), ` · ${w.column}: ${w.value} — `, t(`import.date.${w.reason}`))))) : null,
    h("button", { type: "button", class: "btn btn-primary", disabled: p.counts.new === 0, on: { click: (e) => run(e.currentTarget) } }, t("import.run", { count: p.counts.new })),
    p.counts.new === 0 ? h("p", { class: "muted" }, t("import.nothing")) : null,
  );
}

export default async function importView(app, root) {
  const { t, fmt, api } = app;
  const state = { csv: null, name: "", chosen: {}, dateOrder: null, preview: null };
  const out = slot();
  const ask = slot();
  const form = { consent: null, confirmed: null };

  const body = () => importBody({ csv: state.csv, consentSource: form.consent.value, confirmed: form.confirmed.checked, dateOrder: state.dateOrder, mapping: state.preview ? mappingFrom(state.preview.detected, state.chosen) : {} });

  const preview = async (button) => {
    if (!state.csv) return app.notify(t("import.pickFirst"), "warning");
    const dup = duplicateColumn({ ...(state.preview?.detected.columns ?? {}), ...state.chosen });
    if (dup) return app.notify(t("import.dupColumn", { column: dup }), "critical");
    try {
      if (button) button.disabled = true;
      state.preview = await api.post("import/preview", body());
      ask.set([]);
      out.set(previewBlock(app, state.preview, state, preview, run));
      app.notify(t("import.previewReady", { count: state.preview.total }));
    } catch (e) {
      if (e.code === "ambiguous_date_format") return askDateOrder(e);
      out.set(errorBlock(app, e));
    } finally {
      if (button) button.disabled = false;
    }
  };

  // The worker never guesses 03/04/2024: it asks which order the file uses.
  const askDateOrder = (e) => {
    const cols = (e.body && e.body.columns) || [];
    ask.set(
      h(
        "fieldset",
        { class: "callout warning ask" },
        h("legend", null, t("import.dateOrder.title")),
        h("p", null, t("import.dateOrder.body", { columns: fmt.list(cols) })),
        ["dmy", "mdy"].map((o) => h("label", { class: "radio" }, h("input", { type: "radio", name: "dateOrder", value: o, on: { change: () => (state.dateOrder = o) } }), t(`import.dateOrder.${o}`))),
        h("button", { type: "button", class: "btn btn-primary", on: { click: (ev) => (state.dateOrder ? preview(ev.currentTarget) : app.notify(t("import.dateOrder.pick"), "warning")) } }, t("import.previewAgain")),
      ),
    );
    out.set([]);
    ask.el.querySelector("legend")?.scrollIntoView({ block: "nearest" });
  };

  const run = async (button) => {
    const p = state.preview;
    if (needsConsent(p) && !form.consent.value.trim()) {
      form.consent.focus();
      return app.notify(t("import.needConsent"), "critical");
    }
    const ok = await confirmDialog(app, {
      title: t("import.confirm.title", { count: p.counts.new, file: state.name }),
      body: t("import.confirm.body", { confirm: fmt.number(p.classCounts.toConfirm), pending: fmt.number(p.classCounts.toPending), unsub: fmt.number(p.classCounts.toUnsubscribed), suppress: fmt.number(p.classCounts.toSuppress) }),
      confirm: t("import.confirm.go"),
      danger: false,
    });
    if (!ok) return;
    const r = await busy(app, button, () => api.post("import", body()));
    if (!r) return;
    out.set(
      h(
        "section",
        { class: "callout ok", "aria-labelledby": "done-title" },
        h("h2", { id: "done-title" }, t("import.done", { count: r.imported })),
        h("p", null, t("import.done.body", { suppressed: fmt.number(r.suppressionsAdded), status: t(`status.${r.status}`) })),
        r.classCounts.toPending ? h("p", null, t("import.done.pending"), " ", h("a", { href: href("subscribers", {}, { status: "pending" }) }, t("import.done.pendingGo"))) : null,
      ),
    );
    app.notify(t("import.done", { count: r.imported }));
    state.csv = null;
  };

  const pick = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    try {
      state.csv = await readFileText(file);
    } catch (err) {
      return app.notify(errorText(app, err), "critical");
    }
    Object.assign(state, { name: file.name, chosen: {}, dateOrder: null, preview: null });
    await preview(null);
  };

  mount(root, [
    pageHead(app, t("import.title"), { lead: t("import.lead"), back: { href: href("subscribers"), label: t("nav.subscribers") } }),
    h(
      "section",
      { class: "card", "aria-labelledby": "file-title" },
      h("h2", { id: "file-title" }, t("import.file")),
      h("div", { class: "field" }, h("label", { for: "imp-file" }, t("import.file.label")), h("input", { id: "imp-file", type: "file", accept: ".csv,.tsv,.txt,text/csv,text/plain", "aria-describedby": "imp-file-hint", on: { change: pick } }), h("p", { id: "imp-file-hint", class: "hint" }, t("import.file.hint"))),
      h("div", { class: "field" }, h("label", { for: "imp-consent" }, t("import.consent")), h("input", { id: "imp-consent", ref: (el) => (form.consent = el), autocomplete: "off", "aria-describedby": "imp-consent-hint", placeholder: t("import.consent.example") }), h("p", { id: "imp-consent-hint", class: "hint" }, t("import.consent.hint"))),
      h("div", { class: "check" }, h("input", { id: "imp-confirmed", type: "checkbox", ref: (el) => (form.confirmed = el), "aria-describedby": "imp-confirmed-hint", on: { change: () => state.csv && preview(null) } }), h("label", { for: "imp-confirmed" }, t("import.confirmed")), h("p", { id: "imp-confirmed-hint", class: "hint" }, t("import.confirmed.hint"))),
    ),
    ask.node,
    out.node,
  ]);
  out.set(emptyState({ img: "import", title: t("import.empty.title"), body: t("import.empty.body") }));
}
