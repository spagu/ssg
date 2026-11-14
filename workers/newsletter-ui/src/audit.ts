// The admin audit trail (GO-137): who scheduled, paused, erased, imported.
//
// One place writes it — app.ts, after a state-changing route's handler has
// answered, from the route's `audit` action in routes.ts — so no handler can
// forget it and none has to know about it. A row holds:
//   mode + actor   the `auth` account (id and address) in `auth` mode; in the
//                  other modes nobody is known: the mode alone ("password",
//                  "access") is the actor. Access's identity is not read by
//                  this worker, so it is not invented here either;
//   action         the route's audit action ("campaign.schedule");
//   target         ids from the path ("campaign:<id>", "subscriber:<id>",
//                  "suppression:<hash>"), or the id a create answered with.
//                  Never an address: an id is all an audit needs, and a GDPR
//                  erasure must not leave the erased address behind here;
//   status         what the request was answered (a refused schedule is
//                  worth knowing about too).
// Rows older than NEWSLETTER_AUDIT_RETENTION_DAYS (default 365; "0" keeps
// them forever) are purged on each write, in the same batch.
//
//   GET {base}/api/audit?actor=&action=&cursor=&limit=  newest first

import type { Admin } from "./auth";
import { json } from "./lib";
import type { Env } from "./lib";
import type { Route, RouteContext } from "./router";
import { decodeCursor, encodeCursor } from "./subscribers";

export const DEFAULT_RETENTION_DAYS = 365;
const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 200;
const MAX_TARGET = 200;

// retentionDays reads NEWSLETTER_AUDIT_RETENTION_DAYS: a whole number of days,
// 0 = keep forever; anything else falls back to the default.
export function retentionDays(env: Env): number {
  const raw = (env.NEWSLETTER_AUDIT_RETENTION_DAYS ?? "").trim();
  return /^\d+$/.test(raw) ? Number(raw) : DEFAULT_RETENTION_DAYS;
}

// targetOf names what the call acted on: each path parameter with the
// collection it belongs to ("/api/campaigns/:id/pause" → "campaign:<id>"), else
// the `id` a create answered with. A value that looks like an address is left
// out — the routes take ids, and an address must never end up here.
export async function targetOf(route: Route, params: Record<string, string>, res: Response): Promise<string | null> {
  const segments = route.path.split("/");
  const parts: string[] = [];
  segments.forEach((s, i) => {
    if (!s.startsWith(":")) return;
    const value = params[s.slice(1)];
    if (value && !value.includes("@")) parts.push(`${segments[i - 1].replace(/s$/, "")}:${value}`);
  });
  if (!parts.length && res.ok && (res.headers.get("content-type") ?? "").includes("json")) {
    const body = (await res.clone().json().catch(() => null)) as { id?: unknown } | null;
    if (body && typeof body.id === "string" && !body.id.includes("@")) parts.push(`${segments[2].replace(/s$/, "")}:${body.id}`);
  }
  return parts.length ? parts.join(" ").slice(0, MAX_TARGET) : null;
}

// recordAudit writes one row and purges the expired ones. A failure is logged,
// never turned into an error: the action itself has already happened, and
// answering 500 would tell the admin it had not.
export async function recordAudit(env: Env, admin: Admin, route: Route, params: Record<string, string>, res: Response): Promise<void> {
  try {
    const db = env.NEWSLETTER_DB;
    const now = new Date();
    const days = retentionDays(env);
    const writes = [
      db
        .prepare("INSERT INTO admin_audit (id, at, mode, actor_id, actor_email, action, target, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
        .bind(crypto.randomUUID(), now.toISOString(), admin.mode, admin.user?.id ?? null, admin.user?.email ?? null, route.audit as string, await targetOf(route, params, res), res.status),
    ];
    if (days > 0) writes.push(db.prepare("DELETE FROM admin_audit WHERE at < ?").bind(new Date(now.getTime() - days * 86_400_000).toISOString()));
    await db.batch(writes);
  } catch (e) {
    console.error("newsletter-ui: audit", e);
  }
}

const bad = (error: string, detail: string) => json({ error, detail }, 400);

// GET ?actor=&action=&cursor=&limit= — newest first. `actor` matches an
// account id or address, or a mode ("password", "access") for the rows that
// name nobody; `action` matches exactly.
export async function listAudit({ env, url }: RouteContext): Promise<Response> {
  const p = url.searchParams;
  const where: string[] = [];
  const args: unknown[] = [];
  const actor = (p.get("actor") ?? "").trim();
  if (actor) {
    where.push("(actor_id = ? OR actor_email = ? OR (actor_id IS NULL AND mode = ?))");
    args.push(actor, actor.toLowerCase(), actor);
  }
  const action = (p.get("action") ?? "").trim();
  if (action) {
    where.push("action = ?");
    args.push(action);
  }
  const rawLimit = p.get("limit");
  if (rawLimit !== null && (!/^\d+$/.test(rawLimit) || Number(rawLimit) < 1)) return bad("invalid_limit", `limit must be 1-${MAX_LIMIT}`);
  const limit = rawLimit === null ? DEFAULT_LIMIT : Math.min(Number(rawLimit), MAX_LIMIT);
  const rawCursor = p.get("cursor");
  if (rawCursor) {
    const c = decodeCursor(rawCursor);
    if (!c) return bad("invalid_cursor", "use nextCursor from the previous page");
    where.push("(at < ? OR (at = ? AND id < ?))");
    args.push(c[0], c[0], c[1]);
  }
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT id, at, mode, actor_id, actor_email, action, target, status FROM admin_audit
      ${where.length ? "WHERE " + where.join(" AND ") : ""} ORDER BY at DESC, id DESC LIMIT ?`,
  )
    .bind(...args, limit + 1)
    .all<Record<string, unknown>>();
  const items = results.slice(0, limit).map((r) => ({
    id: r.id as string,
    at: r.at as string,
    mode: r.mode as string,
    actor: r.actor_id ? { id: r.actor_id as string, email: r.actor_email as string } : null,
    action: r.action as string,
    target: r.target as string | null,
    status: r.status as number,
  }));
  const last = items[items.length - 1];
  return json({ items, nextCursor: results.length > limit ? encodeCursor(last.at, last.id) : null });
}
