// Who moderated what (GO-136). A leading underscore keeps this file out of the
// Pages route table — the admin handlers import it.
//
// One row per change in comment_events (schema.sql, created by ensureSchema):
// the action, the comment it touched (null for a purge or an import), the
// actor (the auth account id, the Access email, or "password" / "access"),
// the mode, and a detail (how many rows a purge or an import changed). The
// comments table is not altered: no existing column changes meaning.

import type { Admin } from "./_admin-auth";
import type { Env } from "./_lib";

export type EventAction = "approve" | "spam" | "delete" | "purge" | "import";

const COLUMNS = "INSERT INTO comment_events (id, comment_id, action, actor, mode, detail, created_at)";

// eventFor is the statement recording `action` on comment `id`, batched with
// the change itself. It inserts only when the comment exists — run it BEFORE a
// delete — so an action on a missing id leaves no trace, as it changes nothing.
export function eventFor(env: Env, admin: Admin, action: EventAction, id: string): D1PreparedStatement {
  return env.COMMENTS_DB.prepare(`${COLUMNS} SELECT ?, id, ?, ?, ?, NULL, ? FROM comments WHERE id = ?`)
    .bind(crypto.randomUUID(), action, admin.actor, admin.mode, new Date().toISOString(), id);
}

// recordBulk records a purge or an import that changed `count` rows; none, no row.
export async function recordBulk(env: Env, admin: Admin, action: EventAction, count: number): Promise<void> {
  if (count <= 0) return;
  await env.COMMENTS_DB.prepare(`${COLUMNS} VALUES (?, NULL, ?, ?, ?, ?, ?)`)
    .bind(crypto.randomUUID(), action, admin.actor, admin.mode, String(count), new Date().toISOString())
    .run();
}
