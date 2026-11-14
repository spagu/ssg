// "Modules": a card per module — its picture, name, state in a word, up to
// four numbers, a chart when its summary has a series, what is wrong and
// what to check when it is not working, and the way into its panel.

import { h } from "../vnode.js";
import { chartFigure } from "../chart.js";
import { cardState, illustration, metricsOf, reasonKey, seriesOf } from "../model.js";

// explain: the sentences under the state, for the states that need them.
function explain(app, item, st) {
  const { t } = app;
  if (st.state === "unavailable") {
    const why = t(reasonKey(st.reason), { status: st.httpStatus ?? "?", detail: st.detail ?? "" });
    const check = st.reason === "refused_target" ? "check.refused_target" : st.reason === "invalid_manifest" ? "check.invalid_manifest" : "check.unavailable";
    // With CORE_DB, core remembers when it last saw the module stop answering (GO-149).
    const since = item.unreachableSince ? h("p", { class: "why" }, t("state.unavailable.since", { time: app.fmt.dateTime(item.unreachableSince) })) : null;
    return [since, h("p", { class: "why" }, why), h("p", { class: "check" }, t(check))];
  }
  if (st.state === "needsConfig") return [h("p", { class: "why" }, t("state.needsConfig.text", { count: st.problems }))];
  if (st.state === "denied") return [h("p", { class: "why" }, t("state.denied.text"))];
  if (st.state === "unchecked") return [h("p", { class: "why muted" }, t("state.unchecked.text"))];
  if (item.summary?.state === "invalid") return [h("p", { class: "why muted" }, t("summary.invalid", { detail: item.summary.detail ?? item.summary.reason }))];
  return [];
}

export function moduleCard(app, item, status) {
  const { t, fmt } = app;
  const st = cardState(item, status);
  const metrics = metricsOf(item);
  const series = seriesOf(item);
  const titleId = `mod-${item.id}`;
  return h(
    "li",
    { class: `card module state-${st.state}`, "aria-labelledby": titleId },
    h(
      "div",
      { class: "module-head" },
      h("img", { class: "module-art", src: illustration(item.id), alt: "", width: "56", height: "56" }),
      h("div", null, h("h3", { id: titleId }, item.title), h("p", { class: "state" }, h("span", { class: "state-mark", "aria-hidden": "true" }), t(`state.${st.state}`))),
    ),
    explain(app, item, st),
    metrics.length
      ? h("dl", { class: "metrics" }, metrics.map((m) => h("div", null, h("dt", null, m.label), h("dd", { class: "num" }, fmt.metric(m.value, m.unit)))))
      : null,
    item.summary?.metricsIn ? h("p", { class: "muted small" }, t("metrics.in", { module: app.titleOf(item.summary.metricsIn) })) : null,
    series ? chartFigure({ t, fmt, title: series.label, points: series.points }) : null,
    item.panel
      ? h("p", { class: "module-go" }, h("a", { class: "btn", href: item.panel }, t("card.open", { module: item.title })))
      : h("p", { class: "muted small" }, t("card.noPanel")),
  );
}

// canManage: the overview says whether this caller may arrange modules; the
// link is offered only then (the manage routes refuse anyone else regardless).
export function modulesSection(app, items, statusById, canManage = false) {
  const { t } = app;
  return h(
    "section",
    { class: "modules", "aria-labelledby": "modules-title" },
    h("div", { class: "section-head" }, h("h2", { id: "modules-title" }, t("modules.title")), canManage ? h("a", { href: "#modules", id: "arrange-link" }, t("manage.arrange")) : null),
    items.length
      ? h("ul", { class: "cards" }, items.map((i) => moduleCard(app, i, statusById.get(i.id))))
      : h("p", { class: "callout" }, t("modules.none")),
  );
}
