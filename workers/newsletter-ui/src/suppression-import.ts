// POST {base}/api/suppressions/import/preview — what an import would do, writing nothing
// POST {base}/api/suppressions/import         — the import
//
// For SMTP users, whose server reports bounces by mail rather than webhook:
// paste the bounced addresses as CSV. Body and size rules are the subscriber
// import's (text/csv, or JSON {"csv": "…"}; NEWSLETTER_IMPORT_MAX_ROWS).
// Columns by header name: `email` (required), `reason` (optional: hard — the
// default, it is a bounce list —, complaint or manual). Rows are new,
// duplicate (again in the file), existing (already suppressed: left as they
// are) or invalid. A complaint row also unsubscribes the subscriber.

import { parseCSV } from "./csv";
import { BYTES_PER_ROW, DEFAULT_MAX_ROWS, INSERT_CHUNK, readImport } from "./import";
import { INVALID_LIST_MAX, LOOKUP_CHUNK, SAMPLE_SIZE } from "./import-plan";
import type { PlanError } from "./import-plan";
import { normaliseEmail } from "./input";
import { intVar, json } from "./lib";
import type { Env } from "./lib";
import type { RouteContext } from "./router";
import { SUPPRESSION_UPSERT, suppressedKeys, suppressionKey } from "./suppress";

const IMPORTABLE = ["hard", "complaint", "manual"];

interface Row {
  line: number;
  email: string;
  reason: string;
  key: string;
}
interface Invalid {
  line: number;
  reason: "invalid_email" | "invalid_reason";
  value: string;
}
interface Plan {
  total: number;
  fresh: Row[];
  duplicate: Row[];
  existing: Row[];
  invalid: Invalid[];
}

// planImport reads and classifies the file; precedence: invalid, repeat in
// the file (the first occurrence wins), already suppressed.
export async function planImport(env: Env, csv: string, maxRows: number): Promise<Plan | PlanError> {
  const records = parseCSV(csv);
  if (!records.length) return { error: "empty_file", detail: "the file has no header row", status: 400 };
  const header = records[0].fields.map((h) => h.trim().toLowerCase());
  const [ie, ir] = [header.indexOf("email"), header.indexOf("reason")];
  if (ie < 0) return { error: "missing_email_column", detail: "the header row must name an email column", status: 400 };
  const data = records.slice(1);
  if (data.length > maxRows) return { error: "too_many_rows", detail: `at most ${maxRows} rows per import (NEWSLETTER_IMPORT_MAX_ROWS)`, status: 413 };

  const out: Plan = { total: data.length, fresh: [], duplicate: [], existing: [], invalid: [] };
  const seen = new Set<string>();
  const unique: Row[] = [];
  for (const { line, fields } of data) {
    const get = (i: number) => (i < 0 ? "" : (fields[i] ?? "").trim());
    const email = normaliseEmail(get(ie));
    const reason = get(ir).toLowerCase() || "hard";
    if (!email) out.invalid.push({ line, reason: "invalid_email", value: get(ie).slice(0, 100) });
    else if (!IMPORTABLE.includes(reason)) out.invalid.push({ line, reason: "invalid_reason", value: get(ir).slice(0, 100) });
    else if (seen.has(email)) out.duplicate.push({ line, email, reason, key: "" });
    else (seen.add(email), unique.push({ line, email, reason, key: await suppressionKey(email) }));
  }
  const blocked = await suppressedKeys(env, unique.map((r) => r.key), LOOKUP_CHUNK);
  for (const r of unique) (blocked.has(r.key) ? out.existing : out.fresh).push(r);
  return out;
}

const entry = (r: Row) => ({ line: r.line, email: r.email, reason: r.reason });

export function summariseImport(p: Plan) {
  return {
    total: p.total,
    counts: { new: p.fresh.length, duplicate: p.duplicate.length, existing: p.existing.length, invalid: p.invalid.length },
    sample: { new: p.fresh.slice(0, SAMPLE_SIZE).map(entry), duplicate: p.duplicate.slice(0, SAMPLE_SIZE).map(entry), existing: p.existing.slice(0, SAMPLE_SIZE).map(entry) },
    invalid: p.invalid.slice(0, INVALID_LIST_MAX),
  };
}

// writeImport writes the new rows, a chunk per statement (one JSON parameter
// each), all in one batch: all rows or none.
export async function writeImport(env: Env, rows: Row[]): Promise<number> {
  if (!rows.length) return 0;
  const db = env.NEWSLETTER_DB;
  const at = new Date().toISOString();
  const statements: D1PreparedStatement[] = [];
  for (let i = 0; i < rows.length; i += INSERT_CHUNK) {
    const chunk = rows.slice(i, i + INSERT_CHUNK);
    statements.push(
      db
        .prepare(
          `INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at)
           SELECT json_extract(value, '$[0]'), json_extract(value, '$[1]'), json_extract(value, '$[2]'), 1, 'import', ?, ?
             FROM json_each(?) WHERE true
           ${SUPPRESSION_UPSERT}`,
        )
        .bind(at, at, JSON.stringify(chunk.map((r) => [r.key, r.email, r.reason]))),
      db.prepare("DELETE FROM soft_bounces WHERE email_hash IN (SELECT value FROM json_each(?))").bind(JSON.stringify(chunk.map((r) => r.key))),
    );
    const complaints = chunk.filter((r) => r.reason === "complaint").map((r) => r.email);
    if (complaints.length) {
      statements.push(
        db
          .prepare(
            `UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = ?, updated_at = ?
              WHERE status != 'unsubscribed' AND email IN (SELECT value FROM json_each(?))`,
          )
          .bind(at, at, JSON.stringify(complaints)),
      );
    }
  }
  await db.batch(statements);
  return rows.length;
}

const fail = (e: PlanError) => json({ error: e.error, detail: e.detail }, e.status);

async function prepare({ request, env, url }: RouteContext): Promise<Plan | PlanError> {
  const maxRows = intVar(env.NEWSLETTER_IMPORT_MAX_ROWS, DEFAULT_MAX_ROWS);
  const req = await readImport(request, url, maxRows * BYTES_PER_ROW);
  return "error" in req ? req : planImport(env, req.csv, maxRows);
}

export async function suppressionImportPreview(c: RouteContext): Promise<Response> {
  const p = await prepare(c);
  return "error" in p ? fail(p) : json(summariseImport(p));
}

export async function suppressionImportRun(c: RouteContext): Promise<Response> {
  const p = await prepare(c);
  if ("error" in p) return fail(p);
  return json({ ...summariseImport(p), imported: await writeImport(c.env, p.fresh) });
}
