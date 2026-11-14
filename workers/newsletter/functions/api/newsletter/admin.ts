// Cloudflare Pages Function: newsletter admin.
//   GET  /api/newsletter/admin                        counts by status and language (JSON)
//   GET  /api/newsletter/admin?format=csv[&status=…]  subscriber export
//   POST /api/newsletter/admin {"action":"purge"}     run the retention purge now
//
// Behind Cloudflare Access (NEWSLETTER_ACCESS_TEAM + NEWSLETTER_ACCESS_AUD),
// an `auth` session (NEWSLETTER_AUTH_*, GO-137) or HTTP Basic
// (NEWSLETTER_ADMIN_PASSWORD) — _lib.ts requireAdmin. With `auth` the counts
// need newsletter:read, the export newsletter:export, the purge
// newsletter:settings.

import { EXPORT, READ, SETTINGS, json, requireAdmin } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { STATUSES, purgeExpired, statusCounts } from "./_store";
import { toCSV } from "./_csv";

// The export's columns. unsubscribe_url is what a mail sender puts in each
// message's List-Unsubscribe header (see the README); the IP / User-Agent
// hashes stay in the database — they are for abuse handling, not for mailing.
const COLUMNS = [
  "email", "status", "language", "tags", "source_page", "consent_text", "consent_at",
  "created_at", "confirmed_at", "unsubscribed_at", "unsubscribe_url",
];

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const url = new URL(request.url);
  const csv = url.searchParams.get("format") === "csv";
  const denied = await requireAdmin(request, env, csv ? EXPORT : READ);
  if (denied) return denied;
  if (!env.NEWSLETTER_DB) return json({ error: "newsletter not configured" }, 503);
  await ensureSchema(env);

  if (csv) return exportCSV(env, url);

  return json(await statusCounts(env));
};

// exportCSV builds the file in memory, which is fine for lists of tens of thousands.
// The default is the mailing list (confirmed only); ?status=all exports every row.
async function exportCSV(env: Env, url: URL): Promise<Response> {
  const status = url.searchParams.get("status") || "confirmed";
  if (status !== "all" && !STATUSES.includes(status)) {
    return json({ error: "status must be pending, confirmed, unsubscribed or all" }, 400);
  }
  const where = status === "all" ? "" : "WHERE status = ?";
  const stmt = env.NEWSLETTER_DB.prepare(
    `SELECT email, status, language, tags, source_page, consent_text, consent_at,
            created_at, confirmed_at, unsubscribed_at, token
       FROM subscribers ${where} ORDER BY created_at`,
  );
  const { results } = await (status === "all" ? stmt : stmt.bind(status)).all<Record<string, unknown>>();

  const base = `${url.origin}/api/newsletter/unsubscribe?token=`;
  const rows = results.map(({ token, ...row }) => ({ ...row, unsubscribe_url: base + String(token) }));
  const day = new Date().toISOString().slice(0, 10);
  return new Response(toCSV(COLUMNS, rows), {
    headers: {
      "content-type": "text/csv; charset=utf-8",
      "content-disposition": `attachment; filename="newsletter-${status}-${day}.csv"`,
      "cache-control": "no-store",
    },
  });
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const denied = await requireAdmin(request, env, SETTINGS);
  if (denied) return denied;
  if (!env.NEWSLETTER_DB) return json({ error: "newsletter not configured" }, 503);
  await ensureSchema(env);

  let payload: { action?: string };
  try {
    payload = (await request.json()) as { action?: string };
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }
  if (payload.action !== "purge") return json({ error: "action must be purge" }, 400);
  return json({ ok: true, purged: await purgeExpired(env, true) });
};
