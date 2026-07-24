// Cloudflare Pages Function: comments moderation.
//   GET  /api/comments/admin?status=pending → queue for review
//   POST /api/comments/admin {id, action}   → approve | spam | delete
//
// Behind HTTP Basic auth (COMMENTS_ADMIN_PASSWORD). The static panel that drives
// it is public/comments-admin.html.

import { Env, json, requireAdmin } from "./_lib";

interface AdminRow {
  id: string;
  url: string;
  author: string;
  body: string;
  status: string;
  created_at: string;
}

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const denied = await requireAdmin(request, env);
  if (denied) return denied;

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
  action?: string; // approve | spam | delete
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const denied = await requireAdmin(request, env);
  if (denied) return denied;

  let payload: Action;
  try {
    payload = (await request.json()) as Action;
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }
  const id = payload.id;
  if (!id) return json({ error: "id is required" }, 422);

  switch (payload.action) {
    case "approve":
      await env.COMMENTS_DB.prepare("UPDATE comments SET status = 'approved' WHERE id = ?").bind(id).run();
      break;
    case "spam":
      await env.COMMENTS_DB.prepare("UPDATE comments SET status = 'spam' WHERE id = ?").bind(id).run();
      break;
    case "delete":
      await env.COMMENTS_DB.prepare("DELETE FROM comments WHERE id = ?").bind(id).run();
      break;
    default:
      return json({ error: "action must be approve, spam or delete" }, 400);
  }
  return json({ ok: true });
};
