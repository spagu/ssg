// Numbers, dates and ages through Intl, in the panel's language. The API
// speaks ISO 8601 in UTC; times are shown in the browser's time zone.

const DAY_MS = 86_400_000;

export function createFormat(locale, timeZone) {
  const tz = timeZone ? { timeZone } : {};
  const num = new Intl.NumberFormat(locale);
  const date = new Intl.DateTimeFormat(locale, { dateStyle: "medium", ...tz });
  const dateTime = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", ...tz });
  const rel = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const list = new Intl.ListFormat(locale, { style: "long", type: "conjunction" });
  const valid = (iso) => typeof iso === "string" && !Number.isNaN(Date.parse(iso));
  return {
    locale,
    number: (n) => num.format(Number(n) || 0),
    // A missing or unreadable time shows as an em dash, never "Invalid Date".
    date: (iso) => (valid(iso) ? date.format(new Date(iso)) : "—"),
    dateTime: (iso) => (valid(iso) ? dateTime.format(new Date(iso)) : "—"),
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
  };
}

// isoDay: the UTC day of a time, "2026-11-14" — what a date input holds.
export const isoDay = (ms) => new Date(ms).toISOString().slice(0, 10);
