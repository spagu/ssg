// The Options tab: every setting of the site, grouped as the schema groups
// them, with Advanced (keys that run code or name a secret) last; a search by
// name and description and an "only changed" filter; one group at a time,
// or every group with a match while searching or filtering.

import { h, mount } from "../core/vnode.js";
import { ADVANCED, counts, filterForm } from "../models/form-model.js";
import { groupText, labelOf, searchTexts } from "../models/labels.js";
import { emptyState, pageHead, slot } from "../panel/ui.js";
import { fieldView } from "./field.js";
import { problemsBlock, settingsHistory } from "./options-extra.js";

export default async function options(app, view, route) {
  const { t } = app;
  const form = (await app.loadSettings()).form;
  const ids = form.groups.map((g) => g.id);
  const current = ids.includes(route.params.group) ? route.params.group : ids[0];
  const state = { query: route.query.q ?? "", changed: route.query.changed === "1" };
  const results = slot("option-results");
  const status = slot();
  const side = slot("side");
  const n = counts(form);

  const draw = () => {
    const filtering = state.query.trim() !== "" || state.changed;
    const groups = filterForm(form, { ...state, pending: app.changes.paths(), textsOf: (e) => searchTexts(app, e) });
    const shown = filtering ? groups : groups.filter((g) => g.id === current);
    const total = shown.reduce((s, g) => s + g.count, 0);
    status.set(filtering ? t("options.found", { count: total }) : "");
    if (!shown.length) {
      const empty = filtering
        ? { img: "no-results", title: t("options.noneTitle"), body: t(state.changed ? "options.noneChanged" : "options.noneFound") }
        : { img: "no-results", title: t("options.emptyGroupTitle"), body: t("options.emptyGroup") };
      results.set(emptyState(empty));
      return;
    }
    results.set(shown.map((g) => groupBlock(app, g)));
  };
  let timer = null;
  const onSearch = (e) => {
    state.query = e.target.value;
    clearTimeout(timer);
    timer = setTimeout(draw, 180);
  };
  const groupLabel = (g) => `${groupText(app, g).title} (${app.fmt.number(n[g.id].fields)})`;

  mount(view, [
    pageHead(t("options.title"), {
      lead: t("options.lead", { overlay: app.settings.view.overlayFile, config: app.settings.view.configFile ?? ".ssg.yaml" }),
      actions: h("button", { type: "button", class: "btn", "aria-expanded": "false", "aria-controls": "settings-history", on: { click: (e) => settingsHistory(app, side, e.currentTarget) } }, t("rev.open")),
    }),
    problemsBlock(app, app.settings.view),
    h(
      "div",
      { class: "toolbar", role: "search" },
      h("div", { class: "field grow" }, h("label", { for: "opt-q" }, t("options.search")), h("input", { type: "search", id: "opt-q", value: state.query, "aria-describedby": "opt-q-hint", on: { input: onSearch } }), h("p", { id: "opt-q-hint", class: "hint" }, t("options.searchHint"))),
      h("div", { class: "check inline-check" }, h("input", { type: "checkbox", id: "opt-changed", checked: state.changed, on: { change: (e) => ((state.changed = e.target.checked), draw()) } }), h("label", { for: "opt-changed" }, t("options.onlyChanged"))),
    ),
    h("div", { role: "status", class: "visually-hidden" }, status.node),
    h(
      "div",
      { class: "options-layout" },
      h(
        "nav",
        { class: "group-nav", "aria-label": t("options.groups") },
        h("div", { class: "field group-select" }, h("label", { for: "opt-group" }, t("options.group")), h("select", { id: "opt-group", on: { change: (e) => app.go(app.href("options", { group: e.target.value })) } }, form.groups.map((g) => h("option", { value: g.id, selected: g.id === current }, groupLabel(g))))),
        h("ul", { class: "group-list" }, form.groups.map((g) => h("li", null, h("a", { href: app.href("options", { group: g.id }), "aria-current": g.id === current ? "page" : null, class: g.id === ADVANCED ? "group-advanced" : null }, groupText(app, g).title, " ", h("span", { class: "count" }, app.fmt.number(n[g.id].fields)))))),
      ),
      results.node,
    ),
    side.node,
  ]);
  draw();
}

// groupBlock: a group's heading, what it is about, and its fields; the
// Advanced group says why it is apart and who may change it, and lists its
// keys under the schema group each belongs to.
function groupBlock(app, g) {
  const { t } = app;
  const text = groupText(app, g);
  const item = (i) =>
    i.type === "section"
      ? h(i.depth ? "h4" : "h3", { class: "section-head" }, labelOf(app, i.entry), " ", h("code", null, i.entry.segments.join(".")))
      : fieldView(app, i.entry);
  let body = g.items.map(item);
  if (g.id === ADVANCED) {
    const order = app.settings.view.groups.map((x) => x.id);
    const rank = (i) => order.indexOf(i.entry.node.group);
    const sorted = [...g.items].sort((a, b) => rank(a) - rank(b));
    body = sorted.map((i, n) => [
      n === 0 || rank(sorted[n - 1]) !== rank(i) ? h("h3", { class: "subgroup" }, groupText(app, app.settings.view.groups[rank(i)] ?? { id: "other", title: "", description: "" }).title) : null,
      i.type === "section" ? h(i.depth ? "h5" : "h4", { class: "section-head" }, labelOf(app, i.entry), " ", h("code", null, i.entry.segments.join("."))) : fieldView(app, i.entry),
    ]);
  }
  return h(
    "section",
    { class: "group", "aria-labelledby": `g-${g.id}` },
    h("h2", { id: `g-${g.id}` }, text.title),
    text.description ? h("p", { class: "lead" }, text.description[0].toUpperCase() + text.description.slice(1)) : null,
    g.id === ADVANCED ? h("div", { class: "callout warning" }, h("p", null, t("advanced.why")), h("p", null, t(app.caps.advanced ? "advanced.youCan" : "advanced.youCannot"))) : null,
    body,
  );
}
