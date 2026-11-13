// Numbers, dates and durations through Intl, in the panel's language. The API
// speaks ISO 8601 in UTC; dates are shown in the browser's time zone, except
// per-day series ("2026-11-13"), which are UTC days and stay UTC days.

const DAY_MS = 86_400_000;

export function createFormat(locale, timeZone) {
  const tz = timeZone ? { timeZone } : {};
  const num = new Intl.NumberFormat(locale);
  const pct = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 });
  const date = new Intl.DateTimeFormat(locale, { dateStyle: "medium", ...tz });
  const dateTime = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", ...tz });
  const day = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", timeZone: "UTC" });
  const dayLong = new Intl.DateTimeFormat(locale, { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
  const month = new Intl.DateTimeFormat(locale, { month: "short", timeZone: "UTC" });
  const rel = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const list = new Intl.ListFormat(locale, { style: "long", type: "conjunction" });
  const unit = (u) => new Intl.NumberFormat(locale, { style: "unit", unit: u, unitDisplay: "short" });
  const valid = (iso) => typeof iso === "string" && !Number.isNaN(Date.parse(iso));

  return {
    locale,
    number: (n) => num.format(Number(n) || 0),
    percent: (ratio) => pct.format(Number.isFinite(ratio) ? ratio : 0),
    // A missing or unreadable time shows as an em dash, never "Invalid Date".
    date: (iso) => (valid(iso) ? date.format(new Date(iso)) : "—"),
    dateTime: (iso) => (valid(iso) ? dateTime.format(new Date(iso)) : "—"),
    day: (isoDay) => day.format(new Date(isoDay + "T00:00:00Z")),
    dayLong: (isoDay) => dayLong.format(new Date(isoDay + "T00:00:00Z")),
    month: (isoDay) => month.format(new Date(isoDay + "T00:00:00Z")),
    list: (items) => list.format(items.map(String)),
    // relative("…", now) → "3 hours ago" / "in 2 days".
    relative(iso, now = Date.now()) {
      if (!valid(iso)) return "—";
      const diff = Date.parse(iso) - now;
      const abs = Math.abs(diff);
      if (abs < 60_000) return rel.format(Math.round(diff / 1000), "second");
      if (abs < 3_600_000) return rel.format(Math.round(diff / 60_000), "minute");
      if (abs < DAY_MS) return rel.format(Math.round(diff / 3_600_000), "hour");
      return rel.format(Math.round(diff / DAY_MS), "day");
    },
    // duration(4980) → "1 hr 23 min"; under a minute in seconds.
    duration(seconds) {
      if (seconds === null || seconds === undefined || !Number.isFinite(Number(seconds))) return "—";
      const s = Math.max(0, Math.round(Number(seconds)));
      if (s < 60) return unit("second").format(s);
      const h = Math.floor(s / 3600);
      const m = Math.floor((s % 3600) / 60);
      return h ? `${unit("hour").format(h)} ${unit("minute").format(m)}` : unit("minute").format(m);
    },
  };
}
