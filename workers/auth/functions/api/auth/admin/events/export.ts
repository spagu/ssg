// Cloudflare Pages Function: the audit log as CSV (GO-138).
//   GET /api/auth/admin/events/export?kind=&user=&from=&to=   → 200 text/csv (attachment)
//
// Requires `auth:admin`. The same filter as the list, newest first, at most
// 10 000 rows (narrow the dates for more). Columns: at, kind, account_id,
// account_email, by_id, by_email, method, detail (JSON). A cell a
// spreadsheet would run as a formula is prefixed with an apostrophe; UTF-8
// with a byte-order mark, CRLF. 400 invalid_request {detail}.

import { authorize, invalid } from "../../_admin";
import { EXPORT_MAX, eventsCsv, listEvents, readEventFilter } from "../../_admin-events";
import { iso, secure } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const f = readEventFilter(new URL(request.url).searchParams);
  if (typeof f === "string") return invalid(f);
  const { items } = await listEvents(env, { ...f, cursor: "" }, EXPORT_MAX);
  const day = iso(caller.ctx.now).slice(0, 10);
  return secure(new Response(eventsCsv(items), { headers: { "content-type": "text/csv; charset=utf-8" } }), {
    "content-disposition": `attachment; filename="auth-audit-${day}.csv"`,
  });
};
