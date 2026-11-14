// Cloudflare Pages Function: comments moderation.
//   GET  /api/comments/admin?status=pending → queue for review
//   POST /api/comments/admin {id, action}   → approve | spam | delete
//   POST /api/comments/admin {"action":"purge"} → run the retention purge now
//
// Behind requireAdmin (Cloudflare Access, an `auth` session, or HTTP Basic with
// COMMENTS_ADMIN_PASSWORD). In auth mode each action needs its permission
// (OPERATIONS in _admin-auth.ts) and is recorded with its actor (_events.ts).
// The static panel that drives it is public/comments-admin.html.

import { Env, json, purgeExpired, requireAdmin } from "./_lib";
import { permit } from "./_admin-auth";
import { eventFor, recordBulk } from "./_events";
import { ensureSchema } from "./_schema";

interface AdminRow {
  id: string;
  url: string;
  author: string;
  body: string;
  status: string;
  created_at: string;
}

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await requireAdmin(request, env, "list");
  if (admin instanceof Response) return admin;
  if (!env.COMMENTS_DB) return json({ error: "comments not configured" }, 503);
  await ensureSchema(env);

  const status = new URL(request.url).searchParams.get("status") || "pending";
  if (!["pending", "spam", "approved"].includes(status)) {
    return json({ error: "status must be pending, spam or approved" }, 400);
  }
  const { results } = await env.COMMENTS_DB.prepare(
    `SELECT id, url, author, body, status, created_at
       FROM comments WHERE status = ? ORDER BY created_at DESC LIMIT 500`,
  ).bind(status).all<AdminRow>();

  return json({ status, count: results.length, comments: results });
};

interface Action {
  id?: string;
  action?: string; // approve | spam | delete | purge
}

// The SQL of each per-comment action. A delete records its event first: the
// event statement copies the id from the row, which must still exist.
const CHANGE = {
  approve: "UPDATE comments SET status = 'approved' WHERE id = ?",
  spam: "UPDATE comments SET status = 'spam' WHERE id = ?",
  delete: "DELETE FROM comments WHERE id = ?",
} as const;

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await requireAdmin(request, env);
  if (admin instanceof Response) return admin;
  if (!env.COMMENTS_DB) return json({ error: "comments not configured" }, 503);
  await ensureSchema(env);

  let payload: Action;
  try {
    payload = (await request.json()) as Action;
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }
  if (payload.action === "purge") {
    const refused = permit(admin, "purge");
    if (refused) return refused;
    const purged = await purgeExpired(env, true);
    await recordBulk(env, admin, "purge", purged);
    return json({ ok: true, purged });
  }
  const id = payload.id;
  if (!id) return json({ error: "id is required" }, 422);

  const action = payload.action;
  if (action !== "approve" && action !== "spam" && action !== "delete") {
    return json({ error: "action must be approve, spam or delete" }, 400);
  }
  const refused = permit(admin, action);
  if (refused) return refused;
  const change = env.COMMENTS_DB.prepare(CHANGE[action]).bind(id);
  const event = eventFor(env, admin, action, id);
  await env.COMMENTS_DB.batch(action === "delete" ? [event, change] : [change, event]);
  return json({ ok: true });
};
