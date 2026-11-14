// The audit log as the admin panel reads it (GO-138): auth_events, filtered
// and paged newest first, and the same rows as CSV. A leading underscore keeps
// this file out of the Pages route table. Writing is _audit.ts's emitEvent
// (which also purges what is older than AUTH_AUDIT_RETENTION_DAYS).

import type { Env } from "./_lib";

export const EVENTS_PAGE = 100;
export const EXPORT_MAX = 10_000;

export interface EventFilter {
  kind: string; // one kind, or "" for all
  user: string; // the account changed OR the administrator who acted ("" = anyone)
  from: string; // ISO 8601, inclusive ("" = no bound)
  to: string; // ISO 8601, exclusive
  cursor: string; // "<at>|<id>" of the previous page's last row
}

export interface AuditRow {
  id: string;
  at: string;
  kind: string;
  userId: string | null;
  email: string | null; // the account's address now (null once it is deleted)
  by: string | null; // the administrator, for administrative events
  byEmail: string | null;
  method: string | null;
  detail: Record<string, unknown>;
}

const KIND = /^[a-z][a-z_]{0,40}$/;
const isTime = (v: string): boolean => /^\d{4}-\d{2}-\d{2}(T[\d:.]+Z)?$/.test(v) && !Number.isNaN(Date.parse(v));

// readEventFilter: the filter from query parameters, or the parameter at fault.
export function readEventFilter(q: URLSearchParams): EventFilter | string {
  const f = { kind: q.get("kind") ?? "", user: q.get("user") ?? "", from: q.get("from") ?? "", to: q.get("to") ?? "", cursor: q.get("cursor") ?? "" };
  if (f.kind && !KIND.test(f.kind)) return "kind: an event kind";
  for (const k of ["from", "to"] as const) if (f[k] && !isTime(f[k])) return `${k}: an ISO 8601 date or time`;
  // A bare day reads as that day's start (UTC).
  for (const k of ["from", "to"] as const) if (f[k]) f[k] = new Date(f[k]).toISOString();
  return f;
}

function parseDetail(raw: string): Record<string, unknown> {
  try {
    const v = JSON.parse(raw);
    return v && typeof v === "object" && !Array.isArray(v) ? v : {};
  } catch {
    return {};
  }
}

interface Row {
  id: string;
  at: string;
  kind: string;
  user_id: string | null;
  email: string | null;
  by_id: string | null;
  by_email: string | null;
  method: string | null;
  detail: string;
}

// BY: the administrator an event names (detail.by); a detail that is not
// JSON (never written by emitEvent, but a hand-edited row must not break the
// log) names nobody.
const BY = "(CASE WHEN json_valid(e.detail) THEN json_extract(e.detail, '$.by') END)";

// listEvents: up to `limit` events matching the filter, newest first.
export async function listEvents(env: Env, f: EventFilter, limit: number): Promise<{ items: AuditRow[]; nextCursor: string | null }> {
  const cut = f.cursor.indexOf("|");
  const [at, id] = cut < 0 ? ["\uffff", "\uffff"] : [f.cursor.slice(0, cut), f.cursor.slice(cut + 1)];
  const { results } = await env.AUTH_DB.prepare(
    `SELECT e.id, e.at, e.kind, e.user_id, u.email, ${BY} AS by_id, b.email AS by_email, e.method, e.detail
       FROM auth_events e
       LEFT JOIN users u ON u.id = e.user_id
       LEFT JOIN users b ON b.id = ${BY}
      WHERE (? = '' OR e.kind = ?)
        AND (? = '' OR e.user_id = ? OR ${BY} = ?)
        AND (? = '' OR e.at >= ?)
        AND (? = '' OR e.at < ?)
        AND (e.at < ? OR (e.at = ? AND e.id < ?))
      ORDER BY e.at DESC, e.id DESC LIMIT ?`,
  )
    .bind(f.kind, f.kind, f.user, f.user, f.user, f.from, f.from, f.to, f.to, at, at, id, limit + 1)
    .all<Row>();
  const items = results.slice(0, limit).map((r) => ({
    id: r.id,
    at: r.at,
    kind: r.kind,
    userId: r.user_id,
    email: r.email,
    by: typeof r.by_id === "string" ? r.by_id : null,
    byEmail: r.by_email,
    method: r.method,
    detail: parseDetail(r.detail),
  }));
  const last = items[items.length - 1];
  return { items, nextCursor: results.length > limit ? `${last.at}|${last.id}` : null };
}

// csvCell: one field (RFC 4180). A value a spreadsheet would run as a formula
// (= + - @, tab, CR first) gets a leading apostrophe — the newsletter
// export's rule, so an address like "=HYPERLINK(…)@example.com" stays text.
export function csvCell(value: unknown): string {
  let s = value == null ? "" : String(value);
  if (/^[=+\-@\t\r]/.test(s)) s = "'" + s;
  return /[",\r\n]/.test(s) || s !== s.trim() ? `"${s.replaceAll('"', '""')}"` : s;
}

export const CSV_HEADER = ["at", "kind", "account_id", "account_email", "by_id", "by_email", "method", "detail"];

// eventsCsv: a byte-order mark (spreadsheets then read UTF-8), the header,
// one line per event, CRLF line ends.
export function eventsCsv(rows: readonly AuditRow[]): string {
  const lines = [CSV_HEADER.join(",")];
  for (const r of rows) {
    lines.push([r.at, r.kind, r.userId, r.email, r.by, r.byEmail, r.method, JSON.stringify(r.detail)].map(csvCell).join(","));
  }
  return "\ufeff" + lines.join("\r\n") + "\r\n";
}
