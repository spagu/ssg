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
  const limit = Math.min(100, Math.max(1, Number.parseInt(q.get("limit") ?? "50", 10) || 50));

  const { results } = await env.SHOP_DB.prepare(
    `SELECT id, kind, target, attempts, next_attempt, last_error, done_at, created_at
       FROM outbox
      WHERE done_at IS NULL ${stuckOnly ? "AND attempts >= 3" : ""}
      ORDER BY next_attempt
      LIMIT ?`,
  )
    .bind(limit)
    .all<Omit<OutboxRow, "payload_json">>();

  const counts = await env.SHOP_DB.prepare(
    `SELECT
       SUM(CASE WHEN done_at IS NULL THEN 1 ELSE 0 END) AS pending,
       SUM(CASE WHEN done_at IS NULL AND attempts >= 3 THEN 1 ELSE 0 END) AS stuck,
       SUM(CASE WHEN done_at IS NOT NULL THEN 1 ELSE 0 END) AS done
     FROM outbox`,
  ).first<{ pending: number; stuck: number; done: number }>();

  // The payload can hold a download link and a buyer's address, so the list
  // shows what a message is and how it is going, never what it says (S-28).
  return json({ messages: results ?? [], counts: counts ?? { pending: 0, stuck: 0, done: 0 } });
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
