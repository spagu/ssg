// GET /api/shop/admin/audit — who did what, newest first.
//
// Read-only on purpose: there is no endpoint that edits or clears this table,
// because a log an administrator can rewrite answers no question (S-30).

import type { Env } from "../_env";
import { json, str } from "../_lib";
import type { AuditRow } from "../_audit";
import type { AdminData } from "./_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const q = new URL(request.url).searchParams;
  const limit = Math.min(200, Math.max(1, Number.parseInt(q.get("limit") ?? "50", 10) || 50));
  const before = str(q.get("cursor"), 40);
  const subject = str(q.get("subject"), 80);

  const where: string[] = [];
  const binds: unknown[] = [];
  if (before) {
    // A timestamp alone is not a position: one action writes several entries in
    // the same millisecond, and paging on the timestamp would either repeat
    // them or skip the rest of that millisecond entirely. The id breaks the tie.
    const [at, id] = splitCursor(before);
    where.push("(created_at < ? OR (created_at = ? AND id < ?))");
    binds.push(at, at, id);
  }
  if (subject) {
    where.push("subject = ?");
    binds.push(subject);
  }

  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM audit_log ${where.length ? `WHERE ${where.join(" AND ")}` : ""}
      ORDER BY created_at DESC, id DESC LIMIT ?`,
  )
    .bind(...binds, limit + 1)
    .all<AuditRow>();

  const rows = results ?? [];
  const page = rows.slice(0, limit);
  const last = page.at(-1);
  return json({
    entries: page.map((e) => ({
      ...e,
      detail: e.detail_json ? safeParse(e.detail_json) : null,
      detail_json: undefined,
    })),
    nextCursor: rows.length > limit && last ? `${last.created_at}|${last.id}` : null,
  });
};

/** "2026-09-12T10:00:00.000Z|uuid" into its two halves. A cursor from an older
 *  panel that carries only the timestamp still works, one millisecond coarser. */
function splitCursor(cursor: string): [string, string] {
  const at = cursor.indexOf("|");
  return at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
}

function safeParse(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}
