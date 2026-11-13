// What the dashboard says, computed from the API's answers: the warnings and
// every chart with its title, summary sentence and table. No DOM here.

import { figure } from "../charts/figure.js";
import { lineChart } from "../charts/line.js";
import { columns, hbars } from "../charts/bars.js";
import { legend } from "../charts/svg.js";
import { bucket, peak, sum } from "../charts/scale.js";
import { href } from "../core/router.js";

export const STATUS_KEYS = ["confirmed", "pending", "unsubscribed"];
export const REASON_KEYS = ["hard", "soft", "complaint", "manual"];
export const RESULT_KEYS = ["sent", "failed", "skipped", "uncertain"];

// warnings lists what needs the admin, most serious first. Each is
// {level: "critical" | "warning" | "info", key, vars, href?}.
export function warnings({ settings, engine, campaigns = [] }) {
  const out = [];
  const add = (level, key, vars = {}, link = null) => out.push({ level, key, vars, href: link });
  if (settings && !settings.transport) add("critical", "warn.noTransport", {}, href("settings"));
  if (settings && !settings.siteUrlValid) add("critical", "warn.noSiteUrl", {}, href("settings"));
  if (settings && !settings.sender.email) add("warning", "warn.noSender", {}, href("settings"));
  if (settings && !settings.engine.queue) add("critical", "warn.noQueue", {}, href("settings"));
  for (const c of engine?.campaigns ?? []) {
    if (c.status === "paused" && c.error) add("critical", "warn.enginePaused", { subject: c.subject, error: c.error }, href("campaign", { id: c.id }));
  }
  if (engine && engine.deadLetters.count > 0) add("warning", "warn.deadLetters", { count: engine.deadLetters.count });
  for (const c of campaigns) {
    if (c.uncertain > 0) add("warning", "warn.uncertain", { count: c.uncertain, subject: c.subject }, href("campaign", { id: c.id }, { tab: "uncertain" }));
  }
  if (settings && ["resend", "mailchannels"].includes(settings.transport) && !settings.webhooks[settings.transport]) {
    add("info", "warn.noWebhook", { transport: settings.transport }, href("settings"));
  }
  const rank = { critical: 0, warning: 1, info: 2 };
  return out.sort((a, b) => rank[a.level] - rank[b.level]);
}

// growthFigure: sign-ups vs unsubscribes per day.
export function growthFigure({ stats, t, fmt }) {
  const days = stats.series.map((d) => d.day);
  const signups = sum(stats.series, "signups");
  const unsubs = sum(stats.series, "unsubscribes");
  const top = peak(stats.series, "signups");
  const summary =
    signups + unsubs === 0
      ? t("growth.quiet", { days: stats.days })
      : t("growth.summary", { days: stats.days, signups: fmt.number(signups), unsubs: fmt.number(unsubs), net: fmt.number(signups - unsubs) }) +
        (top ? " " + t("growth.peak", { day: fmt.dayLong(top.day), count: top.signups }) : "");
  const series = [
    { key: "signups", label: t("series.signups"), values: stats.series.map((d) => d.signups) },
    { key: "unsubscribes", label: t("series.unsubscribes"), values: stats.series.map((d) => d.unsubscribes) },
  ];
  const read = (i) => t("growth.read", { day: fmt.dayLong(days[i]), signups: fmt.number(series[0].values[i]), unsubs: fmt.number(series[1].values[i]) });
  const chart = lineChart({ days, series, fmt, read });
  return figure({
    t,
    title: t("growth.title", { days: stats.days }),
    summary,
    legend: legend(series, "line"),
    plot: chart.plot,
    table: {
      columns: [
        { key: "day", label: t("col.day"), format: (d) => fmt.dayLong(d) },
        { key: "signups", label: t("series.signups"), numeric: true, format: fmt.number },
        { key: "unsubscribes", label: t("series.unsubscribes"), numeric: true, format: fmt.number },
      ],
      rows: [...stats.series].reverse(),
    },
  });
}

// statusFigure: how many people are in each status.
export function statusFigure({ stats, t, fmt }) {
  const rows = STATUS_KEYS.map((k) => ({ label: t(`status.${k}`), key: k, parts: [{ key: k, value: stats.byStatus[k] ?? 0 }] }));
  const total = stats.total;
  const share = (n) => fmt.percent(total ? n / total : 0);
  return figure({
    t,
    title: t("statusChart.title"),
    summary: t("statusChart.summary", {
      count: total,
      confirmed: fmt.number(stats.byStatus.confirmed ?? 0),
      share: share(stats.byStatus.confirmed ?? 0),
      pending: fmt.number(stats.byStatus.pending ?? 0),
      unsubscribed: fmt.number(stats.byStatus.unsubscribed ?? 0),
    }),
    empty: total ? null : t("statusChart.empty"),
    plot: hbars({ rows, keys: STATUS_KEYS, valueText: (r) => `${fmt.number(r.total)} · ${share(r.total)}` }),
    table: {
      columns: [
        { key: "label", label: t("col.status") },
        { key: "total", label: t("col.people"), numeric: true, format: fmt.number },
      ],
      rows: rows.map((r) => ({ label: r.label, total: r.parts[0].value })),
    },
  });
}

// languageName: "pl" → "Polish" in the panel's language; "unknown" → words.
export function languageName(code, lang, t) {
  if (!code || code === "unknown") return t("lang.unknown");
  try {
    return new Intl.DisplayNames([lang], { type: "language" }).of(code);
  } catch {
    return code;
  }
}

// languageFigure: per language, stacked by status, largest first; beyond
// eight languages the rest is folded into "other".
export function languageFigure({ stats, t, fmt, lang }) {
  const entries = Object.entries(stats.byLanguage)
    .map(([code, by]) => ({ code, by, total: STATUS_KEYS.reduce((n, k) => n + (by[k] ?? 0), 0) }))
    .sort((a, b) => b.total - a.total || a.code.localeCompare(b.code));
  const shown = entries.slice(0, 8);
  const rest = entries.slice(8);
  if (rest.length) {
    const by = Object.fromEntries(STATUS_KEYS.map((k) => [k, rest.reduce((n, e) => n + (e.by[k] ?? 0), 0)]));
    shown.push({ code: "other", by, total: rest.reduce((n, e) => n + e.total, 0), label: t("lang.other", { count: rest.length }) });
  }
  const rows = shown.map((e) => ({ label: e.label ?? languageName(e.code, lang, t), parts: STATUS_KEYS.map((k) => ({ key: k, value: e.by[k] ?? 0 })), by: e.by }));
  const series = STATUS_KEYS.map((k) => ({ key: k, label: t(`status.${k}`) }));
  return figure({
    t,
    title: t("langChart.title"),
    summary: rows.length ? t("langChart.summary", { top: rows[0].label, count: entries.length, people: fmt.number(shown[0].total) }) : t("langChart.none"),
    empty: rows.length ? null : t("statusChart.empty"),
    legend: legend(series),
    plot: hbars({ rows, keys: STATUS_KEYS, valueText: (r) => fmt.number(r.total) }),
    table: {
      columns: [{ key: "label", label: t("col.language") }, ...STATUS_KEYS.map((k) => ({ key: k, label: t(`status.${k}`), numeric: true, format: fmt.number }))],
      rows: rows.map((r) => ({ label: r.label, ...Object.fromEntries(STATUS_KEYS.map((k) => [k, r.by[k] ?? 0])) })),
    },
  });
}

// campaignsFigure: the latest started campaigns, each a stacked bar of its
// recipients (sent / failed / skipped / uncertain), unsubscribes in the label.
export function campaignsFigure({ list, t, fmt }) {
  const items = list.items;
  const rows = items.map((c) => ({ label: c.subject || t("campaign.untitled"), parts: RESULT_KEYS.map((k) => ({ key: k, value: c[k] })), c }));
  const sent = sum(items, "sent");
  const largest = Math.max(0, ...items.map((c) => c.total));
  return figure({
    t,
    title: t("campChart.title"),
    summary: items.length ? t("campChart.summary", { count: items.length, sent: fmt.number(sent), largest: fmt.number(largest) }) : t("campChart.none"),
    empty: items.length ? null : t("campChart.empty"),
    legend: legend(RESULT_KEYS.map((k) => ({ key: k, label: t(`result.${k}`) }))),
    plot: hbars({ rows, keys: RESULT_KEYS, valueText: (r) => t("campChart.value", { sent: fmt.number(r.c.sent), unsubs: fmt.number(r.c.unsubscribes) }) }),
    table: {
      columns: [
        { key: "subject", label: t("col.campaign") },
        { key: "startedAt", label: t("col.started"), format: fmt.date },
        ...RESULT_KEYS.map((k) => ({ key: k, label: t(`result.${k}`), numeric: true, format: fmt.number })),
        { key: "unsubscribes", label: t("result.unsubscribes"), numeric: true, format: fmt.number },
      ],
      rows: items,
    },
  });
}

// suppressionFigure: suppressions created per day / week / month, by reason.
export function suppressionFigure({ stats, t, fmt, headingLevel = "h3" }) {
  const b = bucket(stats.series, REASON_KEYS);
  const added = REASON_KEYS.reduce((n, k) => n + sum(stats.series, k), 0);
  const label = b.size === "month" ? fmt.month : fmt.day;
  const every = b.size === "day" ? 5 : b.size === "week" ? 2 : 1;
  return figure({
    t,
    headingLevel,
    title: t("supChart.title", { days: stats.days }),
    summary: t("supChart.summary", {
      count: stats.total,
      hard: fmt.number(stats.byReason.hard),
      soft: fmt.number(stats.byReason.soft),
      complaint: fmt.number(stats.byReason.complaint),
      manual: fmt.number(stats.byReason.manual),
      added: fmt.number(added),
      days: stats.days,
    }),
    legend: legend(REASON_KEYS.map((k) => ({ key: k, label: t(`reason.${k}`) }))),
    plot: columns({ series: b.series, keys: REASON_KEYS, fmt, label, every }),
    table: {
      columns: [
        { key: "day", label: t(`col.${b.size}`), format: (d) => (b.size === "month" ? fmt.month(d) : fmt.dayLong(d)) },
        ...REASON_KEYS.map((k) => ({ key: k, label: t(`reason.${k}`), numeric: true, format: fmt.number })),
      ],
      rows: [...b.series].reverse(),
    },
  });
}
