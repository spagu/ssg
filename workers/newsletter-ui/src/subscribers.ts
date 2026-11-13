// GET {base}/api/subscribers       — the list, filtered, cursor-paginated
// GET {base}/api/subscribers/:id   — one person: record, consent, sends, suppression
//
// What leaves the database about a person is decided here, once: SUMMARY_SQL
// and DETAIL_SQL name every column read. `token` (the unsubscribe secret) and
// the IP / User-Agent hashes (abuse handling) are never selected, so no route
// built on these can leak them by accident.

import type { RouteContext } from "./router";
import { json } from "./lib";
import type { Env } from "./lib";
import { escapeLike, isStatus, normaliseLang, splitTags, TAG_RE, text } from "./input";
import { findSuppression } from "./suppress";

export const DEFAULT_LIMIT = 50;
export const MAX_LIMIT = 200;

const SUMMARY_COLUMNS = "id, email, status, language, tags, source_page, created_at, updated_at, confirmed_at, unsubscribed_at";
const DETAIL_COLUMNS = SUMMARY_COLUMNS + ", consent_text, consent_at, confirm_sent_at";

type Row = Record<string, unknown>;

export function summary(r: Row) {
  return {
    id: r.id as string,
    email: r.email as string,
    status: r.status as string,
    language: r.language as string | null,
    tags: splitTags(r.tags),
    sourcePage: r.source_page as string | null,
    createdAt: r.created_at as string,
    updatedAt: r.updated_at as string,
    confirmedAt: r.confirmed_at as string | null,
    unsubscribedAt: r.unsubscribed_at as string | null,
  };
}

// The cursor is the (created_at, id) of the last row served, opaque to the
// client. Ordering by both makes the order total: an import writes thousands
// of rows with one created_at, and id breaks the tie.
export const encodeCursor = (createdAt: string, id: string): string =>
  btoa(JSON.stringify([createdAt, id])).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");

export function decodeCursor(raw: string): [string, string] | null {
  try {
    const v = JSON.parse(atob(raw.replaceAll("-", "+").replaceAll("_", "/")));
    return Array.isArray(v) && v.length === 2 && v.every((x) => typeof x === "string") ? (v as [string, string]) : null;
  } catch {
    return null;
  }
}

const bad = (error: string, detail: string) => json({ error, detail }, 400);

export async function listSubscribers({ env, url }: RouteContext): Promise<Response> {
  const p = url.searchParams;
  const where: string[] = [];
  const args: unknown[] = [];

  const status = p.get("status");
  if (status !== null) {
    if (!isStatus(status)) return bad("invalid_status", "status must be pending, confirmed or unsubscribed");
    where.push("status = ?");
    args.push(status);
  }
  const language = p.get("language");
  if (language !== null) {
    const lang = normaliseLang(language);
    if (!lang) return bad("invalid_language", "language must be a tag like en or pt-BR");
    where.push("language = ?");
    args.push(lang);
  }
  const tag = p.get("tag");
  if (tag !== null) {
    const t = tag.trim().toLowerCase();
    if (!TAG_RE.test(t)) return bad("invalid_tag", "a tag is 1-32 characters of a-z 0-9 _ -");
    // instr, not LIKE: "_" is a valid tag character and a LIKE wildcard.
    where.push("instr(',' || tags || ',', ?) > 0");
    args.push(`,${t},`);
  }
  const q = text(p.get("q"), 254).toLowerCase();
  if (q) {
    where.push("email LIKE ? ESCAPE '\\'");
    args.push(`%${escapeLike(q)}%`);
  }

  let limit = DEFAULT_LIMIT;
  const rawLimit = p.get("limit");
  if (rawLimit !== null) {
    if (!/^\d+$/.test(rawLimit) || Number(rawLimit) < 1) return bad("invalid_limit", `limit must be 1-${MAX_LIMIT}`);
    limit = Math.min(Number(rawLimit), MAX_LIMIT);
  }
  const rawCursor = p.get("cursor");
  if (rawCursor) {
    const c = decodeCursor(rawCursor);
    if (!c) return bad("invalid_cursor", "use nextCursor from the previous page");
    where.push("(created_at < ? OR (created_at = ? AND id < ?))");
    args.push(c[0], c[0], c[1]);
  }

  // Newest first; one row more than asked tells whether a next page exists.
  const sql = `SELECT ${SUMMARY_COLUMNS} FROM subscribers ${where.length ? "WHERE " + where.join(" AND ") : ""}
                ORDER BY created_at DESC, id DESC LIMIT ?`;
  const { results } = await env.NEWSLETTER_DB.prepare(sql).bind(...args, limit + 1).all<Row>();
  const page = results.slice(0, limit);
  const last = page[page.length - 1];
  const nextCursor = results.length > limit ? encodeCursor(last.created_at as string, last.id as string) : null;
  return json({ items: page.map(summary), nextCursor });
}

export async function findSubscriber(env: Env, id: string): Promise<Row | null> {
  return env.NEWSLETTER_DB.prepare(`SELECT ${DETAIL_COLUMNS} FROM subscribers WHERE id = ?`).bind(id).first<Row>();
}

// sendsOf lists a person's campaign_sends rows with the campaign's subject
// (NULL when the campaign was deleted), newest first.
export async function sendsOf(env: Env, id: string) {
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT s.campaign_id, c.subject, s.status, s.attempts, s.error, s.claimed_at, s.sent_at
       FROM campaign_sends s LEFT JOIN campaigns c ON c.id = s.campaign_id
      WHERE s.subscriber_id = ? ORDER BY s.claimed_at DESC`,
  )
    .bind(id)
    .all<Row>();
  return results.map((r) => ({
    campaignId: r.campaign_id as string,
    subject: r.subject as string | null,
    status: r.status as string,
    attempts: r.attempts as number,
    error: r.error as string | null,
    claimedAt: r.claimed_at as string,
    sentAt: r.sent_at as string | null,
  }));
}

// detailOf is the full picture of one person, shared by GET /:id and the GDPR
// export. The suppression is reported without its key: a hash of the address
// is still personal data and the admin has no use for it.
export async function detailOf(env: Env, r: Row) {
  const sup = await findSuppression(env, r.email as string);
  return {
    ...summary(r),
    consentText: r.consent_text as string,
    consentAt: r.consent_at as string,
    confirmSentAt: r.confirm_sent_at as string | null,
    sends: await sendsOf(env, r.id as string),
    suppression: sup ? { reason: sup.reason, source: sup.source, count: sup.count, createdAt: sup.createdAt, updatedAt: sup.updatedAt } : null,
  };
}

export const notFound = () => json({ error: "not_found" }, 404);

export async function getSubscriber({ env, params }: RouteContext): Promise<Response> {
  const row = await findSubscriber(env, params.id);
  return row ? json(await detailOf(env, row)) : notFound();
}
