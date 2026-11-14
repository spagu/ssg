// Cloudflare Pages Function: the audit log (GO-138).
//   GET /api/auth/admin/events?kind=&user=&from=&to=&cursor=   → 200 {items: [event], nextCursor, retentionDays}
//
// Requires `auth:admin`. Newest first, 100 a page. `kind` one event kind;
// `user` an account id — the account the event is about OR the
// administrator who acted; `from` (inclusive) / `to` (exclusive) ISO 8601
// dates or times. An event is {id, at, kind, userId, email, by, byEmail,
// method, detail}; `email` / `byEmail` are the accounts' addresses now (null
// once deleted — account_deleted keeps the address in `detail`). Events
// older than AUTH_AUDIT_RETENTION_DAYS (365) are purged as new ones are
// written. 400 invalid_request {detail}.

import { authorize, invalid } from "../../_admin";
import { EVENTS_PAGE, listEvents, readEventFilter } from "../../_admin-events";
import { auditRetentionDays } from "../../_audit";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const f = readEventFilter(new URL(request.url).searchParams);
  if (typeof f === "string") return invalid(f);
  return json({ ...(await listEvents(env, f, EVENTS_PAGE)), retentionDays: auditRetentionDays(env) });
};
