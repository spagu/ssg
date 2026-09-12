// GET  /api/shop/admin/outbox — what is queued, and what is stuck
// POST /api/shop/admin/outbox — drain the queue now, or retry one message
//
// The queue drains by itself on ordinary traffic. This endpoint is for the
// quiet shop where nobody visits between the sale and the complaint.

import type { Env } from "../_env";
import { audit } from "../_audit";
import { fail, json, readJson, str } from "../_lib";
import { drainOutbox, retryOutbox, type OutboxRow } from "../_outbox";
import type { AdminData } from "./_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const q = new URL(request.url).searchParams;
  const stuckOnly = q.get("stuck") === "1";
  const limit = Math.min(100, Math.max(1, Number.parseInt(q.get("limit") ?? "25", 10) || 25));
  const cursor = str(q.get("cursor"), 120);

  // Ordered by when each message is next due, so the cursor is that timestamp
  // and the id breaks the tie between two queued in the same millisecond.
  const where = ["done_at IS NULL"];
  const binds: unknown[] = [];
  if (stuckOnly) where.push("attempts >= 3");
  if (cursor) {
    const at = cursor.indexOf("|");
    const [due, id] = at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
    where.push("(next_attempt > ? OR (next_attempt = ? AND id > ?))");
    binds.push(due, due, id);
  }

  const { results } = await env.SHOP_DB.prepare(
    `SELECT id, kind, target, attempts, next_attempt, last_error, done_at, created_at
       FROM outbox
      WHERE ${where.join(" AND ")}
      ORDER BY next_attempt, id
      LIMIT ?`,
  )
    .bind(...binds, limit + 1)
    .all<Omit<OutboxRow, "payload_json">>();

  const counts = await env.SHOP_DB.prepare(
    `SELECT
       SUM(CASE WHEN done_at IS NULL THEN 1 ELSE 0 END) AS pending,
       SUM(CASE WHEN done_at IS NULL AND attempts >= 3 THEN 1 ELSE 0 END) AS stuck,
       SUM(CASE WHEN done_at IS NOT NULL THEN 1 ELSE 0 END) AS done
     FROM outbox`,
  ).first<{ pending: number; stuck: number; done: number }>();

  const rows = results ?? [];
  const page = rows.slice(0, limit);
  const last = page.at(-1);

  // The payload can hold a download link and a buyer's address, so the list
  // shows what a message is and how it is going, never what it says (S-28).
  return json({
    messages: page,
    counts: counts ?? { pending: 0, stuck: 0, done: 0 },
    total: stuckOnly ? (counts?.stuck ?? 0) : (counts?.pending ?? 0),
    nextCursor: rows.length > limit && last ? `${last.next_attempt}|${last.id}` : null,
  });
};

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const body = (await readJson<{ id?: string; action?: string }>(request)) ?? {};
  const actor = `admin:${data.admin.sub}`;

  if (body.action === "drain" || !body.id) {
    const sent = await drainOutbox(env, 20);
    await audit(env, actor, "outbox.drain", null, { sent });
    return json({ ok: true, sent });
  }

  const id = str(body.id, 40);
  const ok = await retryOutbox(env, id);
  if (!ok) return fail(request, "not_found", "No queued message with that id.", 404);
  const sent = await drainOutbox(env, 5);
  await audit(env, actor, "outbox.retry", id, { sent });
  return json({ ok: true, sent });
};
