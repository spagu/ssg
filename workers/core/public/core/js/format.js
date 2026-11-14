// Numbers, dates and durations through Intl, in the page's language. The API
// speaks ISO 8601 in UTC; times are shown in the browser's time zone, except
// per-day series ("2026-11-13"), which are UTC days and stay UTC days.

const DAY_MS = 86_400_000;

export function createFormat(locale, timeZone) {
  const tz = timeZone ? { timeZone } : {};
  const num = new Intl.NumberFormat(locale, { maximumFractionDigits: 1 });
  const pct = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 });
  const days = new Intl.NumberFormat(locale, { style: "unit", unit: "day", unitDisplay: "long", maximumFractionDigits: 1 });
  // A contrast ratio as the build measured it (already rounded down to two
  // decimals): never rounded up here, so 4.49 is never shown as 4.5.
  const ratio = new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 2, roundingMode: "trunc" });
  const secs = new Intl.NumberFormat(locale, { style: "unit", unit: "second", unitDisplay: "short", maximumFractionDigits: 1 });
  const dateTime = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", ...tz });
  const time = new Intl.DateTimeFormat(locale, { timeStyle: "short", ...tz });
  const day = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", timeZone: "UTC" });
  const dayLong = new Intl.DateTimeFormat(locale, { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
  const rel = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const valid = (iso) => typeof iso === "string" && !Number.isNaN(Date.parse(iso));

  return {
    locale,
    number: (n) => num.format(Number(n) || 0),
    // metric: a summary metric's value with its unit (percent is 0–100).
    metric(value, unit) {
      if (unit === "percent") return pct.format((Number(value) || 0) / 100);
      if (unit === "days") return days.format(Number(value) || 0);
      return num.format(Number(value) || 0);
    },
    ratio: (r) => ratio.format(Number(r) || 0),
    seconds: (s) => secs.format(Number(s) || 0),
    // A missing or unreadable time shows as an em dash, never "Invalid Date".
    dateTime: (iso) => (valid(iso) ? dateTime.format(new Date(iso)) : "—"),
    time: (iso) => (valid(iso) ? time.format(new Date(iso)) : "—"),
    day: (isoDay) => day.format(new Date(isoDay + "T00:00:00Z")),
    dayLong: (isoDay) => dayLong.format(new Date(isoDay + "T00:00:00Z")),
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
  };
}
