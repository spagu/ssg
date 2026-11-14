// The filters of the People and Audit log views as they live in the hash's
// query ("#/people?status=blocked&q=ann"), so a filtered view can be linked,
// reloaded and returned to after step-up. Pure functions.

import { isoDay } from "../core/format.js";

export const USER_FILTER = ["q", "status", "roleId", "groupId"];
export const AUDIT_FILTER = ["kind", "user", "from", "to"];
export const STATUSES = ["active", "blocked"];

// pick keeps the known, non-empty keys of a query object.
export function pick(query, keys) {
  const out = {};
  for (const k of keys) if (typeof query[k] === "string" && query[k].trim() !== "") out[k] = query[k].trim();
  return out;
}

// userFilter: the People filter from a route's query; an unknown status is dropped.
export function userFilter(query) {
  const f = pick(query, USER_FILTER);
  if (f.status && !STATUSES.includes(f.status)) delete f.status;
  return f;
}

// auditFilter: the Audit log filter; dates must be YYYY-MM-DD.
export function auditFilter(query) {
  const f = pick(query, AUDIT_FILTER);
  for (const k of ["from", "to"]) if (f[k] && !/^\d{4}-\d{2}-\d{2}$/.test(f[k])) delete f[k];
  return f;
}

// toApi: the audit filter as the API reads it — "to" is a whole day, so the
// bound is the next day's start (the API's `to` is exclusive).
export function toApi(f) {
  const out = { ...f };
  if (out.to) out.to = isoDay(Date.parse(out.to + "T00:00:00Z") + 86_400_000);
  return out;
}

// lastDays: a range ending today (UTC) — the Audit log's quick choices.
export const lastDays = (days, now = Date.now()) => ({ from: isoDay(now - (days - 1) * 86_400_000), to: isoDay(now) });

// formData reads named fields out of a FormData-like object into a filter.
export function formData(data, keys) {
  const q = {};
  for (const k of keys) q[k] = String(data.get(k) ?? "");
  return pick(q, keys);
}
