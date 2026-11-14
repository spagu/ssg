// What the home page decides, as plain functions over the API's answers —
// the views only lay the results out. No DOM here.

// ILLUSTRATED: modules with their own picture in img/modules/<id>.svg —
// drawn for this page, never fetched from a module. Anything else (a module
// you wrote) gets img/modules/default.svg.
export const ILLUSTRATED = [
  "auth", "comments", "contact-form", "conversions-proxy", "cookie-consent", "dynamic-price",
  "newsletter", "rate-limit", "republish-trigger", "stripe-checkout", "web-push",
];
const ALIASES = { "newsletter-ui": "newsletter" };

export function illustration(id) {
  const name = ALIASES[id] ?? id;
  return `img/modules/${ILLUSTRATED.includes(name) ? name : "default"}.svg`;
}

// statusIndex: /api/core/status items by id; an unusable answer is empty.
export function statusIndex(body) {
  const out = new Map();
  for (const s of Array.isArray(body?.items) ? body.items : []) if (s && typeof s.id === "string") out.set(s.id, s);
  return out;
}

const KNOWN_REASONS = ["timeout", "deadline", "network", "http_status", "redirect", "refused_target", "invalid_manifest"];
export const reasonKey = (reason) => `reason.${KNOWN_REASONS.includes(reason) ? reason : "other"}`;

// cardState: one word for a module, most serious first —
//   unavailable  it (or its summary) did not answer, or its manifest is bad
//   denied       it refused this account
//   needsConfig  it answers and reports configuration problems
//   working      it answered
//   unchecked    nothing to ask it (no endpoint) and no status to go by
// with the reason and what is known about it.
export function cardState(item, status) {
  const s = item.summary;
  if (status?.status === "unreachable") return { state: "unavailable", reason: status.reason, httpStatus: status.httpStatus ?? null };
  if (status?.status === "invalid_manifest" || item.status === "invalid_manifest") {
    return { state: "unavailable", reason: "invalid_manifest", detail: status?.detail ?? status?.reason ?? null };
  }
  if (item.status === "unreachable") return { state: "unavailable", reason: "other" };
  if (s?.state === "unavailable") return { state: "unavailable", reason: s.reason, httpStatus: s.httpStatus ?? null };
  if (status?.status === "denied" || s?.state === "denied") return { state: "denied" };
  if (s?.state === "ok" && s.problems.length) return { state: "needsConfig", problems: s.problems.length };
  if (status?.status === "reachable" || s?.state === "ok" || s?.state === "invalid") return { state: "working" };
  return { state: "unchecked" };
}

// metricsOf: up to four numbers worth a tile (the format allows four).
export const metricsOf = (item) => (item.summary?.state === "ok" ? item.summary.metrics.slice(0, 4) : []);

// seriesOf: a series worth a chart — at least two days.
export const seriesOf = (item) => {
  const series = item.summary?.state === "ok" ? item.summary.series : null;
  return series && series.points.length >= 2 ? series : null;
};

// authHint: one calm sentence about the `auth` worker, only with a shared
// password and more than one panel — the password stays a supported mode.
export const authHint = (mode, items) => mode === "password" && items.filter((i) => i.panel).length > 1;

// unanswered: modules that declare a summary but gave none usable this time.
export const unanswered = (items) => items.filter((i) => i.summary && i.summary.state !== "ok").length;

export const SEVERITIES = ["action", "warning", "info"];

// severityCounts: how many attention items of each severity are shown.
export function severityCounts(items) {
  const out = { action: 0, warning: 0, info: 0 };
  for (const a of items) if (a.severity in out) out[a.severity] += 1;
  return out;
}

// buildLines: what the Site section says about the build, as [key, vars]
// pairs for the strings. `now` keeps "3 hours ago" testable.
export function buildLines(info, fmt, now = Date.now()) {
  const b = info.build;
  if (info.buildState !== "ok" || !b) return [[`site.build.${["absent", "invalid"].includes(info.buildState) ? info.buildState : "unreadable"}`, { option: info.buildOption ?? "build_report" }]];
  const when = b.buildTime ?? b.finished;
  const lines = [["site.build.ok", { version: b.version ?? "—", time: fmt.dateTime(when), relative: fmt.relative(when, now) }]];
  if (b.pages !== null && b.files !== null) {
    lines.push([b.durationSeconds === null ? "site.build.size" : "site.build.sizeTime", { pages: fmt.number(b.pages), files: fmt.number(b.files), duration: fmt.seconds(b.durationSeconds) }]);
  }
  return lines;
}

// modeKey: the sign-in mode in words; an unknown one is said as unknown.
export const modeKey = (mode) => `mode.${["password", "auth", "access"].includes(mode) ? mode : "unknown"}`;

// statusKey: a module's registry status in words.
export const statusKey = (status) => `modstatus.${["registered", "live", "unreachable", "invalid_manifest"].includes(status) ? status : "other"}`;

// errorKey: the sentence for a failed request.
export const errorKey = (err) =>
  `error.${["network", "missing", "signin", "auth", "forbidden", "rate", "config", "conflict", "invalid"].includes(err?.kind) ? err.kind : "server"}`;

// loginHref: where the "Sign in" link of an `auth`-mode refusal goes — the
// configured login page with this page as `next`, never another host.
export function loginHref(config, here) {
  const login = typeof config.loginUrl === "string" && /^\/(?![/\\])[^\s\\]*$/.test(config.loginUrl) ? config.loginUrl : "/auth/login";
  const url = new URL(login, "https://h.invalid");
  url.searchParams.set("next", here);
  return url.pathname + url.search;
}
