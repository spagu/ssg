// GET    {base}/api/suppressions        — the list, filtered, cursor-paginated
// POST   {base}/api/suppressions        — add one by hand
// DELETE {base}/api/suppressions/:hash  — lift one (a complaint needs {confirm: true})
//
// A row is shown with its key (emailHash): an erased address has no other
// handle, and lifting goes by key. `q` searches the kept addresses only — a
// hash-only row has nothing to match, which is the point of dropping it.

import { applyBounces } from "./bounce-store";
import { escapeLike, normaliseEmail, readJSONObject, text } from "./input";
import { json } from "./lib";
import type { RouteContext } from "./router";
import { decodeCursor, encodeCursor } from "./subscribers";
import { findSuppression, suppress } from "./suppress";
import type { Suppression } from "./suppress";

export const REASONS = ["hard", "soft", "complaint", "manual"] as const;
const ADDABLE = ["manual", "hard", "complaint"];
const HASH = /^[0-9a-f]{64}$/;
const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 200;

const bad = (error: string, detail: string) => json({ error, detail }, 400);

const item = (r: Record<string, unknown>): Suppression => ({
  emailHash: r.email_hash as string,
  email: r.email as string | null,
  reason: r.reason as string,
  count: r.count as number,
  source: r.source as string | null,
  createdAt: r.created_at as string,
  updatedAt: r.updated_at as string,
});

// GET ?reason=&q=&cursor=&limit= — newest change first.
export async function listSuppressions({ env, url }: RouteContext): Promise<Response> {
  const p = url.searchParams;
  const where: string[] = [];
  const args: unknown[] = [];
  const reason = p.get("reason");
  if (reason !== null) {
    if (!(REASONS as readonly string[]).includes(reason)) return bad("invalid_reason", `reason must be ${REASONS.join(", ")}`);
    where.push("reason = ?");
    args.push(reason);
  }
  const q = text(p.get("q"), 254).toLowerCase();
  if (q) {
    where.push("email LIKE ? ESCAPE '\\'");
    args.push(`%${escapeLike(q)}%`);
  }
  const rawLimit = p.get("limit");
  if (rawLimit !== null && (!/^\d+$/.test(rawLimit) || Number(rawLimit) < 1)) return bad("invalid_limit", `limit must be 1-${MAX_LIMIT}`);
  const limit = rawLimit === null ? DEFAULT_LIMIT : Math.min(Number(rawLimit), MAX_LIMIT);
  const rawCursor = p.get("cursor");
  if (rawCursor) {
    const c = decodeCursor(rawCursor);
    if (!c) return bad("invalid_cursor", "use nextCursor from the previous page");
    where.push("(updated_at < ? OR (updated_at = ? AND email_hash < ?))");
    args.push(c[0], c[0], c[1]);
  }
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT email_hash, email, reason, count, source, created_at, updated_at FROM suppressions
      ${where.length ? "WHERE " + where.join(" AND ") : ""} ORDER BY updated_at DESC, email_hash DESC LIMIT ?`,
  )
    .bind(...args, limit + 1)
    .all<Record<string, unknown>>();
  const items = results.slice(0, limit).map(item);
  const last = items[items.length - 1];
  return json({ items, nextCursor: results.length > limit ? encodeCursor(last.updatedAt, last.emailHash) : null });
}

// POST {email, reason?: manual (default) | hard | complaint} → 201 {suppression}
// (200 when the address was already suppressed; the stronger reason is kept).
// A complaint unsubscribes the subscriber, as one from a provider does.
export async function addSuppression({ request, env }: RouteContext): Promise<Response> {
  const body = await readJSONObject(request);
  const email = normaliseEmail(body?.email);
  if (!email) return bad("invalid_email", "email must be an address");
  const reason = body?.reason ?? "manual";
  if (typeof reason !== "string" || !ADDABLE.includes(reason)) return bad("invalid_reason", `reason must be ${ADDABLE.join(", ")}`);
  const existed = (await findSuppression(env, email)) !== null;
  if (reason === "manual") await suppress(env, { email, reason, source: "admin", keepEmail: true });
  else await applyBounces(env, "admin", [{ id: null, kind: reason as "hard" | "complaint", emails: [email], source: "admin", detail: "" }]);
  return json({ suppression: await findSuppression(env, email) }, existed ? 200 : 201);
}

// DELETE /:hash, body {confirm: true} required for a complaint: the person
// said this mail is spam, so lifting it is a decision, not a click. The soft
// count goes too, so the address starts clean.
export async function deleteSuppression({ request, env, params }: RouteContext): Promise<Response> {
  const hash = params.hash.toLowerCase();
  if (!HASH.test(hash)) return json({ error: "not_found" }, 404);
  const confirmed = (await readJSONObject(request))?.confirm === true;
  const db = env.NEWSLETTER_DB;
  // The complaint rule is in the DELETE itself, so a row that became a
  // complaint after the admin looked at it is not lifted unconfirmed.
  const [gone] = await db.batch([
    db.prepare("DELETE FROM suppressions WHERE email_hash = ? AND (reason != 'complaint' OR ?)").bind(hash, confirmed ? 1 : 0),
    db.prepare("DELETE FROM soft_bounces WHERE email_hash = ? AND NOT EXISTS (SELECT 1 FROM suppressions WHERE email_hash = ?)").bind(hash, hash),
  ]);
  if (gone.meta.changes === 1) return new Response(null, { status: 204 });
  const left = await db.prepare("SELECT 1 FROM suppressions WHERE email_hash = ?").bind(hash).first();
  if (!left) return json({ error: "not_found" }, 404);
  return json({ error: "confirm_required", detail: 'the recipient reported spam; send {"confirm": true} to lift it' }, 409);
}
