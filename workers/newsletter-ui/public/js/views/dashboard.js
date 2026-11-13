// Dashboard: warnings first, then the numbers and charts. Glue: the charts
// and warnings are computed in models/dashboard.js.

import { createPoller } from "../core/tasks.js";
import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { progress } from "../charts/bars.js";
import { campaignsFigure, growthFigure, languageFigure, statusFigure, suppressionFigure, warnings } from "../models/dashboard.js";
import { chip, errorBlock, loading, pageHead, slot } from "./ui.js";

const RANGES = [30, 90, 365];

function warningList(app, list) {
  const { t } = app;
  if (!list.length) {
    return h("div", { class: "callout ok all-clear" }, h("img", { src: "img/all-clear.svg", alt: "", width: "72", height: "54" }), h("p", null, t("warn.none")));
  }
  return h(
    "ul",
    { class: "warnings", "aria-label": t("warn.title") },
    list.map((w) =>
      h(
        "li",
        { class: `callout ${w.level}` },
        h("p", { class: "callout-title" }, t(`level.${w.level}`)),
        h("p", null, t(w.key, w.vars)),
        w.href ? h("a", { href: w.href }, t(`${w.key}.go`)) : null,
      ),
    ),
  );
}

function tiles(app, subs, sup) {
  const { t, fmt } = app;
  const tile = (key, value, note, view) =>
    h("li", { class: `tile tile-${key}` }, h("a", { href: view }, h("span", { class: "tile-label" }, t(`tile.${key}`)), h("span", { class: "tile-value" }, fmt.number(value)), h("span", { class: "tile-note" }, note)));
  return h(
    "ul",
    { class: "tiles" },
    tile("confirmed", subs.byStatus.confirmed ?? 0, t("tile.confirmed.note"), href("subscribers", {}, { status: "confirmed" })),
    tile("pending", subs.byStatus.pending ?? 0, t("tile.pending.note"), href("subscribers", {}, { status: "pending" })),
    tile("unsubscribed", subs.byStatus.unsubscribed ?? 0, t("tile.unsubscribed.note"), href("subscribers", {}, { status: "unsubscribed" })),
    tile("suppressed", sup.total, t("tile.suppressed.note"), href("suppressions")),
  );
}

function engineBlock(app, engine) {
  const { t, fmt } = app;
  return h(
    "section",
    { class: "card engine", "aria-labelledby": "engine-title" },
    h("h2", { id: "engine-title" }, t("engine.title")),
    h(
      "p",
      { class: "muted" },
      t("engine.rate", { used: fmt.number(engine.rate.used), rate: fmt.number(engine.rate.perMinute), batch: fmt.number(engine.batchSize) }),
      " ",
      engine.deadLetters.count ? t("engine.dead", { count: engine.deadLetters.count, at: fmt.dateTime(engine.deadLetters.lastAt) }) : t("engine.noDead"),
    ),
    engine.campaigns.length
      ? h(
          "ul",
          { class: "live-list" },
          engine.campaigns.map((c) =>
            h(
              "li",
              null,
              h("p", { class: "live-head" }, h("a", { href: href("campaign", { id: c.id }) }, c.subject || t("campaign.untitled")), " ", chip(app, c.status)),
              progress({ p: c, t, fmt }),
              c.error ? h("p", { class: "engine-error" }, t("engine.pausedBecause", { error: c.error })) : null,
            ),
          ),
        )
      : h("p", null, t("engine.idle"), " ", h("a", { href: href("campaigns") }, t("engine.idle.go"))),
  );
}

export default async function dashboard(app, root) {
  const { t, api } = app;
  const days = Number(sessionRange()) || 30;
  const range = slot("range-charts");
  const engineSlot = slot();
  mount(root, [pageHead(app, t("dash.title"), { lead: t("dash.lead") }), loading(app)]);

  const load = async () => {
    const [settings, engine, subs, list, sup] = await Promise.all([
      api.get("settings"),
      api.get("engine/status"),
      api.get("stats/subscribers", { days }),
      api.get("stats/campaigns", { limit: 8 }),
      api.get("stats/suppressions", { days }),
    ]);
    return { settings, engine, subs, list, sup };
  };
  let data;
  try {
    data = await load();
  } catch (e) {
    mount(root, [pageHead(app, t("dash.title")), errorBlock(app, e, () => dashboard(app, root))]);
    return;
  }

  const ctx = { t, fmt: app.fmt, lang: app.lang };
  const rangeCharts = (subs, sup) => [growthFigure({ ...ctx, stats: subs }), suppressionFigure({ ...ctx, stats: sup })];
  const switcher = h(
    "fieldset",
    { class: "segmented" },
    h("legend", null, t("dash.range")),
    RANGES.map((d) =>
      h(
        "label",
        null,
        h("input", {
          type: "radio",
          name: "range",
          value: String(d),
          checked: d === days,
          on: {
            change: async () => {
              sessionRange(d);
              try {
                const [s2, p2] = await Promise.all([api.get("stats/subscribers", { days: d }), api.get("stats/suppressions", { days: d })]);
                range.set(rangeCharts(s2, p2));
                app.notify(t("dash.rangeShown", { days: d }));
              } catch (e) {
                range.set(errorBlock(app, e));
              }
            },
          },
        }),
        h("span", null, t("dash.days", { count: d })),
      ),
    ),
  );

  mount(root, [
    pageHead(app, t("dash.title"), { lead: t("dash.lead") }),
    h("section", { "aria-labelledby": "warn-title", class: "warn-block" }, h("h2", { id: "warn-title", class: "visually-hidden" }, t("warn.title")), warningList(app, warnings({ settings: data.settings, engine: data.engine, campaigns: data.list.items }))),
    tiles(app, data.subs, data.sup),
    engineSlot.node,
    h("div", { class: "toolbar" }, switcher),
    h("div", { class: "chart-grid" }, range.node, statusFigure({ ...ctx, stats: data.subs }), languageFigure({ ...ctx, stats: data.subs }), campaignsFigure({ ...ctx, list: data.list })),
  ]);
  range.set(rangeCharts(data.subs, data.sup));
  engineSlot.set(engineBlock(app, data.engine));

  // While anything is sending, the engine block refreshes every 5 seconds
  // (not while the tab is hidden).
  if (data.engine.campaigns.some((c) => c.status === "sending")) {
    const poller = createPoller({
      interval: 5000,
      visible: () => document.visibilityState === "visible",
      tick: async () => {
        const engine = await api.get("engine/status");
        engineSlot.set(engineBlock(app, engine));
        return !engine.campaigns.some((c) => c.status === "sending");
      },
    });
    setTimeout(() => poller.start(), 5000);
    app.visibility = () => poller.wake();
    app.onLeave(() => (poller.stop(), (app.visibility = null)));
  }
}

// The chosen range survives navigation within the tab (not a secret, not stored long-term).
function sessionRange(v) {
  try {
    if (v === undefined) return sessionStorage.getItem("newsletter-admin-range");
    sessionStorage.setItem("newsletter-admin-range", String(v));
  } catch {
    return null;
  }
  return null;
}
