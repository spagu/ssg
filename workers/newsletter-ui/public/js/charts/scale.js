// Scales and ticks. Every chart here counts things, so the value axis always
// starts at zero (a bar or a line of counts that does not start at zero lies
// about proportions) and its ticks are "nice" integers.

// niceStep is 1, 2 or 5 × 10^k, the smallest that fits `count` intervals.
export function niceStep(span, count) {
  const raw = span / Math.max(1, count);
  const mag = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 5, 10].map((m) => m * mag).find((s) => s >= raw);
  return Math.max(1, step);
}

// countTicks(max) → {max, ticks}: zero-based integer ticks covering max, about
// `count` of them. An all-zero series gets 0…4, so a flat line sits on a
// readable axis instead of a collapsed one.
export function countTicks(max, count = 4) {
  const top = Math.max(0, Number(max) || 0);
  if (top === 0) return { max: 4, ticks: [0, 1, 2, 3, 4] };
  const step = niceStep(top, count);
  const niceMax = Math.ceil(top / step) * step;
  const ticks = [];
  for (let v = 0; v <= niceMax; v += step) ticks.push(v);
  return { max: niceMax, ticks };
}

// linear maps [d0, d1] onto [r0, r1].
export const linear = (d0, d1, r0, r1) => (v) => (d1 === d0 ? r0 : r0 + ((v - d0) / (d1 - d0)) * (r1 - r0));

// dayTicks picks which of `days` (ISO "YYYY-MM-DD", consecutive) get a label:
// up to 92 days, evenly spaced (ending on the last day, the one people look
// for); longer ranges label the first day of each month.
export function dayTicks(days, maxTicks = 6) {
  if (days.length === 0) return [];
  if (days.length > 92) {
    const firsts = days.map((d, i) => (d.endsWith("-01") ? i : -1)).filter((i) => i >= 0);
    const every = Math.ceil(firsts.length / Math.max(1, maxTicks * 2));
    return firsts.filter((_, k) => k % every === 0);
  }
  const step = Math.max(1, Math.ceil((days.length - 1) / Math.max(1, maxTicks - 1)));
  const out = [];
  for (let i = days.length - 1; i >= 0; i -= step) out.unshift(i);
  return out;
}

// bucket groups a per-day series into weeks (90 days) or months (365 days),
// summing every numeric key; 30 days stays per day. Each bucket is labelled
// by its first day. Used where daily columns would be too thin to see.
export function bucket(series, keys) {
  const size = series.length > 92 ? "month" : series.length > 31 ? "week" : "day";
  if (size === "day") return { size, series };
  const out = [];
  series.forEach((row, i) => {
    const key = size === "month" ? row.day.slice(0, 7) : Math.floor(i / 7);
    const last = out[out.length - 1];
    if (!last || last.key !== key) out.push({ key, day: row.day, ...Object.fromEntries(keys.map((k) => [k, 0])) });
    const cur = out[out.length - 1];
    for (const k of keys) cur[k] += Number(row[k]) || 0;
  });
  return { size, series: out.map(({ key: _k, ...rest }) => rest) };
}

// sum of one key over a series.
export const sum = (series, key) => series.reduce((n, r) => n + (Number(r[key]) || 0), 0);

// peak: the row with the highest value of key (the latest on a tie), or null when all are 0.
export function peak(series, key) {
  let best = null;
  for (const r of series) if ((Number(r[key]) || 0) > 0 && (!best || r[key] >= best[key])) best = r;
  return best;
}
