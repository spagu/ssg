// Webhook health in Settings (#460): how many verified webhook events arrived
// in the last 24 hours and 7 days, the latest one's time, and a warning when a
// transport that reports bounces only by webhook has been silent since the
// last campaign started. Totals from GET /api/settings, never an address.

import { h } from "../core/vnode.js";

// countsText: "delivered: 3 · hard bounces: 1 · …" — a label and a number, so
// no language needs plural forms here.
const countsText = (app, c) =>
  app.t("set.hooks.counts", {
    delivered: app.fmt.number(c.delivered),
    hard: app.fmt.number(c.hard),
    soft: app.fmt.number(c.soft),
    complaint: app.fmt.number(c.complaint),
    other: app.fmt.number(c.other),
  });

export function webhookCard(app, a) {
  const { t, fmt } = app;
  const rows = [
    [t("set.hooks.last"), a.lastReceivedAt ? fmt.dateTime(a.lastReceivedAt) : t("set.hooks.never")],
    [t("set.hooks.day"), countsText(app, a.last24h)],
    [t("set.hooks.week"), countsText(app, a.last7d)],
  ];
  return h(
    "section",
    { class: "card", "aria-labelledby": "set-hooks" },
    h("h2", { id: "set-hooks" }, t("set.hooks")),
    a.warning
      ? h(
          "div",
          { class: "callout warning", role: "status" },
          h("p", { class: "callout-title" }, t("set.hooks.warn.title")),
          h("p", null, t("set.hooks.warn", { at: fmt.dateTime(a.lastCampaignStartedAt) })),
        )
      : null,
    h("dl", { class: "facts" }, rows.map(([k, v]) => [h("dt", null, k), h("dd", null, v)])),
    h("p", { class: "note" }, t("set.hooks.note")),
  );
}
