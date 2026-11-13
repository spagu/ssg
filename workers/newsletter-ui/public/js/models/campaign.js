// Campaign rules the panel needs, mirrored from the worker's state machine
// (src/campaign-state.ts) and validation (src/campaign-model.ts, segment.ts),
// so buttons that cannot work are never offered. The worker still decides:
// a 409 from a race is shown like any other answer.

import { figure } from "../charts/figure.js";
import { hbars } from "../charts/bars.js";

export const STATUSES = ["draft", "scheduled", "sending", "paused", "sent", "cancelled"];

// actionsFor(status) → the actions offered, in the order shown.
const ACTIONS = {
  draft: ["save", "test", "schedule", "duplicate", "delete"],
  scheduled: ["unschedule", "cancel", "duplicate"],
  sending: ["pause", "cancel", "duplicate"],
  paused: ["resume", "cancel", "duplicate"],
  sent: ["duplicate"],
  cancelled: ["duplicate", "delete"],
};
export const actionsFor = (status) => ACTIONS[status] ?? [];

// Destructive or mail-starting actions need a confirmation naming the campaign.
export const CONFIRM = new Set(["schedule", "cancel", "delete"]);

// isLive: the engine is working on it; the panel polls its progress.
export const isLive = (status) => status === "sending" || status === "paused";
export const hasStats = (c) => !!c.startedAt;

export const TAG_RE = /^[a-z0-9_-]{1,64}$/;
export const LANG_RE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

// splitList("news, Offers;news") → ["news", "offers"] (lower-cased, unique).
export const splitList = (text, lower = true) => [
  ...new Set(
    String(text || "")
      .split(/[,;\s]+/)
      .map((s) => (lower ? s.trim().toLowerCase() : s.trim()))
      .filter(Boolean),
  ),
];

// segmentFrom(form fields) → {segment} or {error: key, value}.
export function segmentFrom({ languages, tags, tagMode }) {
  const langs = splitList(languages, false);
  const badLang = langs.find((l) => !LANG_RE.test(l));
  if (badLang) return { error: "segment.badLanguage", value: badLang };
  const list = splitList(tags);
  const badTag = list.find((t) => !TAG_RE.test(t));
  if (badTag) return { error: "segment.badTag", value: badTag };
  return { segment: { languages: langs, tags: list, tagMode: tagMode === "all" ? "all" : "any" } };
}

// payloadFrom(form fields) → the PATCH/POST body, or {error}.
export function payloadFrom(f) {
  const seg = segmentFrom(f);
  if (seg.error) return seg;
  return {
    body: {
      subject: f.subject.replace(/[\r\n]+/g, " "),
      body: f.body,
      bodyFormat: f.bodyFormat === "html" ? "html" : "markdown",
      textOverride: f.textOverride.trim() ? f.textOverride : null,
      fromEmail: f.fromEmail.trim() || null,
      fromName: f.fromName.trim() || null,
      segment: seg.segment,
    },
  };
}

// fieldsOf(campaign) → the editor's field values.
export const fieldsOf = (c) => ({
  subject: c.subject ?? "",
  body: c.body ?? "",
  bodyFormat: c.bodyFormat ?? "markdown",
  textOverride: c.textOverride ?? "",
  fromEmail: c.fromEmail ?? "",
  fromName: c.fromName ?? "",
  languages: (c.segment?.languages ?? []).join(", "),
  tags: (c.segment?.tags ?? []).join(", "),
  tagMode: c.segment?.tagMode ?? "any",
});

export const sameFields = (a, b) => Object.keys(a).every((k) => a[k] === b[k]);

const pad = (n) => String(n).padStart(2, "0");

// toLocalInput(iso) → "YYYY-MM-DDTHH:MM" in the browser's zone, for <input type=datetime-local>.
export function toLocalInput(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// fromLocalInput("YYYY-MM-DDTHH:MM") (local time) → ISO, or null.
export function fromLocalInput(value) {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value || "")) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

// RESULTS: the rows of the campaign result chart, from GET /campaigns/:id/stats.
export const RESULTS = ["sent", "failed", "skipped", "pending", "uncertain"];

// resultsFigure: the recipients' outcomes, plus unsubscribes credited by time window.
export function resultsFigure({ stats, t, fmt }) {
  const r = stats.recipients;
  const rows = [
    ...RESULTS.map((k) => ({ label: t(`result.${k}`), parts: [{ key: "sent", value: r[k] }], key: k, value: r[k] })),
    { label: t("result.unsubscribes"), parts: [{ key: "unsubscribes", value: stats.unsubscribes.count }], key: "unsubscribes", value: stats.unsubscribes.count },
  ];
  return figure({
    t,
    title: t("results.title"),
    summary: t("results.summary", {
      count: r.total,
      sent: fmt.number(r.sent),
      share: fmt.percent(r.total ? r.sent / r.total : 0),
      failed: fmt.number(r.failed),
      unsubs: fmt.number(stats.unsubscribes.count),
    }),
    empty: r.total ? null : t("results.empty"),
    plot: hbars({ rows, keys: ["sent", "unsubscribes"], max: Math.max(1, r.total), valueText: (row) => fmt.number(row.total) }),
    table: {
      columns: [
        { key: "label", label: t("col.outcome") },
        { key: "value", label: t("col.people"), numeric: true, format: fmt.number },
      ],
      rows,
    },
  });
}

// clicksFigure: clicks per link; null when tracking is off and nothing was counted.
export function clicksFigure({ stats, t, fmt }) {
  const c = stats.clicks;
  if (!c.tracking && c.links.length === 0) return null;
  const rows = c.links.map((l) => ({ label: shortUrl(l.url), parts: [{ key: "clicks", value: l.clicks }], url: l.url, clicks: l.clicks }));
  return figure({
    t,
    title: t("clicks.title"),
    summary: c.links.length ? t("clicks.summary", { count: c.total, links: fmt.number(c.links.length), top: shortUrl(c.links[0].url) }) : t("clicks.none"),
    empty: c.links.length ? null : t("clicks.empty"),
    plot: hbars({ rows, keys: ["clicks"], valueText: (row) => fmt.number(row.total) }),
    table: {
      columns: [
        { key: "url", label: t("col.link") },
        { key: "clicks", label: t("col.clicks"), numeric: true, format: fmt.number },
      ],
      rows,
    },
  });
}

// shortUrl: "https://example.com/blog/post/?a=1" → "example.com/blog/post/?a=1", at most 60 characters.
export function shortUrl(url) {
  const s = String(url).replace(/^https?:\/\//, "");
  return s.length > 60 ? s.slice(0, 59) + "…" : s;
}
