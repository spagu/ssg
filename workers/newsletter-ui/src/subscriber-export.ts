// GET {base}/api/subscribers/:id/export  — GDPR access request: one person, JSON
// GET {base}/api/subscribers.csv         — the mailing list as CSV
//
// The CSV has the columns, order and safety rules of the `newsletter`
// template's `GET /api/newsletter/admin?format=csv` (BOM, CRLF, formula guard,
// unsubscribe_url per row), so a tool that reads one reads the other. The one
// difference: suppressed addresses are left out — this file is what gets
// mailed, and a suppressed address must not be.

import type { RouteContext } from "./router";
import { json, secure } from "./lib";
import { isStatus } from "./input";
import { toCSV } from "./csv";
import { detailOf, findSubscriber, notFound } from "./subscribers";
import { dropHashOnly, NOT_SUPPRESSED_SQL } from "./suppress";

export const CSV_COLUMNS = [
  "email", "status", "language", "tags", "source_page", "consent_text", "consent_at",
  "created_at", "confirmed_at", "unsubscribed_at", "unsubscribe_url",
];

const day = () => new Date().toISOString().slice(0, 10);

// exportSubscriber is everything stored about the person, minus the IP and
// User-Agent hashes and the unsubscribe token (a credential, not information
// about them), as a file download.
export async function exportSubscriber({ env, params }: RouteContext): Promise<Response> {
  const row = await findSubscriber(env, params.id);
  if (!row) return notFound();
  const data = { exportedAt: new Date().toISOString(), subscriber: await detailOf(env, row) };
  return json(data, 200, { "content-disposition": `attachment; filename="subscriber-${params.id.replace(/[^A-Za-z0-9-]/g, "")}-${day()}.json"` });
}

// exportSubscribersCSV: ?status=confirmed (default) | pending | unsubscribed | all.
// The unsubscribe links point at the `newsletter` template on this origin —
// both workers serve one site, the admin under its base path.
export async function exportSubscribersCSV({ env, url }: RouteContext): Promise<Response> {
  const status = url.searchParams.get("status") || "confirmed";
  if (status !== "all" && !isStatus(status)) {
    return json({ error: "invalid_status", detail: "status must be pending, confirmed, unsubscribed or all" }, 400);
  }
  const where = [NOT_SUPPRESSED_SQL("s")];
  if (status !== "all") where.push("s.status = ?");
  const stmt = env.NEWSLETTER_DB.prepare(
    `SELECT s.email, s.status, s.language, s.tags, s.source_page, s.consent_text, s.consent_at,
            s.created_at, s.confirmed_at, s.unsubscribed_at, s.token
       FROM subscribers s WHERE ${where.join(" AND ")} ORDER BY s.created_at`,
  );
  const { results } = await (status === "all" ? stmt : stmt.bind(status)).all<{ email: string; token: string; consent_at: string }>();
  // freshConsentWins: the same rule as the send engine, so the file lists
  // exactly who a campaign would mail (someone who signed up again after an
  // erasure is on it).
  const kept = await dropHashOnly(env, results, true);

  const base = `${url.origin}/api/newsletter/unsubscribe?token=`;
  const rows = kept.map(({ token, ...row }) => ({ ...row, unsubscribe_url: base + token }));
  return secure(
    new Response(toCSV(CSV_COLUMNS, rows), {
      headers: {
        "content-type": "text/csv; charset=utf-8",
        "content-disposition": `attachment; filename="newsletter-${status}-${day()}.csv"`,
      },
    }),
  );
}
