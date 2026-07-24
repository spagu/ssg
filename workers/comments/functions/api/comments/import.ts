// Cloudflare Pages Function: bulk comment import (admin only).
//   POST /api/comments/import  { items: [...], defaultStatus?: "approved" }
//
// The canonical target every migration converts into: Disqus, WordPress (WXR),
// Commento, a spreadsheet — export them to this normalised JSON shape and post
// it. No captcha and no spam check: this is a trusted admin operation behind the
// same Basic-auth gate as moderation.
//
// Idempotent: each row's id is a hash of (url, author, body, created_at) and the
// insert is INSERT OR IGNORE, so re-posting the same export inserts nothing new
// instead of duplicating — a failed import is safe to retry.

import { Env, json, sha256hex, normaliseURL, requireAdmin } from "./_lib";

// One imported comment. Only url/author/body are required; the rest default.
interface ImportItem {
  url?: string;
  author?: string;
  email?: string;
  body?: string;
  created_at?: string; // ISO 8601; defaults to now when absent/invalid
  status?: string; // pending | approved | spam; defaults to defaultStatus
}

interface ImportBody {
  items?: ImportItem[];
  // Status for items that don't carry their own. Imported comments are usually
  // already-vetted, so "approved" is the default; use "pending" to re-moderate.
  defaultStatus?: string;
}

const STATUSES = ["pending", "approved", "spam"];
const MAX_ITEMS = 1000; // per request; chunk larger exports client-side
const BATCH = 50; // D1 statements per batch

// A valid ISO date string, else now — a messy export must not poison a row.
function isoOrNow(raw: unknown): string {
  if (typeof raw === "string") {
    const t = Date.parse(raw);
    if (!Number.isNaN(t)) return new Date(t).toISOString();
  }
  return new Date().toISOString();
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const denied = requireAdmin(request, env);
  if (denied) return denied;

  let payload: ImportBody;
  try {
    payload = (await request.json()) as ImportBody;
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }

  const items = payload.items;
  if (!Array.isArray(items)) {
    return json({ error: "items must be an array" }, 422);
  }
  if (items.length > MAX_ITEMS) {
    return json({ error: `too many items (max ${MAX_ITEMS} per request — chunk the export)` }, 413);
  }

  const fallback = STATUSES.includes(payload.defaultStatus || "") ? payload.defaultStatus! : "approved";

  // Build one prepared statement per valid item; count the invalid ones instead
  // of failing the whole batch, so one bad row doesn't sink a 900-row import.
  const stmt = env.COMMENTS_DB.prepare(
    `INSERT OR IGNORE INTO comments
       (id, url, author, email, body, status, created_at, ip_hash, user_agent, avatar_hash)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  );
  const rows: D1PreparedStatement[] = [];
  let invalid = 0;

  for (const it of items) {
    const url = normaliseURL(it.url);
    const author = (it.author || "").trim().slice(0, 80);
    const email = (it.email || "").trim().slice(0, 254);
    const body = (it.body || "").trim().slice(0, 5000);
    if (!url || !author || !body) {
      invalid++;
      continue;
    }
    const status = STATUSES.includes(it.status || "") ? it.status! : fallback;
    const createdAt = isoOrNow(it.created_at);
    const id = (await sha256hex(`${url}\n${author}\n${body}\n${createdAt}`)).slice(0, 32);
    rows.push(
      stmt.bind(
        id,
        url,
        author,
        email || null,
        body,
        status,
        createdAt,
        null, // ip_hash: imported rows have no originating IP
        "import",
        email ? await sha256hex(email.toLowerCase()) : null,
      ),
    );
  }

  // INSERT OR IGNORE reports meta.changes = 1 for a fresh row, 0 for a duplicate
  // that was skipped — so summing changes separates imported from already-present.
  let imported = 0;
  for (let i = 0; i < rows.length; i += BATCH) {
    const results = await env.COMMENTS_DB.batch(rows.slice(i, i + BATCH));
    for (const r of results) imported += r.meta.changes || 0;
  }

  return json({
    ok: true,
    total: items.length,
    imported,
    duplicate: rows.length - imported,
    invalid,
  });
};
