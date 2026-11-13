// The import's writes: new subscriber rows and, for addresses the old list
// marked as bounced or complained (new ones, and existing subscribers whose
// row stays untouched), suppressions — all in ONE db.batch, so a failed
// import leaves nothing half written.
//
// Each chunk of rows travels as ONE JSON parameter (json_each), the same shape
// as the plan's lookups: 10 000 rows cost about ten statements, not 10 000.

import type { Env } from "./lib";
import { classOf, fileReason } from "./import-plan";
import type { Candidate } from "./import-plan";
import { SUPPRESSION_UPSERT, suppressionKey } from "./suppress";

export const INSERT_CHUNK = 1000; // rows per INSERT statement (~170 KB JSON parameter)

// consent_text for rows imported as unsubscribed without a consentSource: they
// are never mailed, the column only needs to say how the row got there.
export const UNSUBSCRIBED_CONSENT = "Imported as unsubscribed";

// newToken: 32 random bytes, base64url, 43 characters — the `newsletter`
// template's _lib.ts newToken, which its isToken() check expects.
export function newToken(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

// record is one subscriber row in json_each order (see subscriberInserts). A
// sign-up date from the file is when the person consented and joined; without
// one, the import is.
function record(r: Candidate, status: string, consentText: string, now: string): unknown[] {
  const joined = r.signedUpAt ?? now;
  return [
    crypto.randomUUID(), r.email, status, r.language, r.sourcePage ?? null, r.tags, consentText, joined, newToken(), joined,
    status === "confirmed" ? joined : null,
    status === "unsubscribed" ? (r.unsubscribedAt ?? now) : null,
  ];
}

// subscriberInserts: ON CONFLICT DO NOTHING covers a sign-up that lands between
// the plan and the write: that row is skipped (not counted), never overwritten.
function subscriberInserts(env: Env, records: unknown[][], now: string): D1PreparedStatement[] {
  const out: D1PreparedStatement[] = [];
  for (let i = 0; i < records.length; i += INSERT_CHUNK) {
    out.push(
      env.NEWSLETTER_DB.prepare(
        `INSERT INTO subscribers
           (id, email, status, language, source_page, tags, consent_text, consent_at, token, created_at, updated_at,
            confirmed_at, unsubscribed_at)
         SELECT json_extract(value, '$[0]'), json_extract(value, '$[1]'), json_extract(value, '$[2]'),
                json_extract(value, '$[3]'), json_extract(value, '$[4]'), json_extract(value, '$[5]'),
                json_extract(value, '$[6]'), json_extract(value, '$[7]'), json_extract(value, '$[8]'),
                json_extract(value, '$[9]'), ?, json_extract(value, '$[10]'), json_extract(value, '$[11]')
           FROM json_each(?) WHERE true
         ON CONFLICT DO NOTHING`,
      ).bind(now, JSON.stringify(records.slice(i, i + INSERT_CHUNK))),
    );
  }
  return out;
}

// suppressionInserts writes `hard` (bounced, cleaned) or `complaint` rows with
// source `import` and the address kept, merged with any row already there
// (a weaker one the plan saw, or one that appeared since) by the shared
// SUPPRESSION_UPSERT rule: the stronger reason is kept.
async function suppressionInserts(env: Env, rows: Candidate[], now: string): Promise<D1PreparedStatement[]> {
  const out: D1PreparedStatement[] = [];
  for (let i = 0; i < rows.length; i += INSERT_CHUNK) {
    const chunk = await Promise.all(
      rows.slice(i, i + INSERT_CHUNK).map(async (r) => [await suppressionKey(r.email), r.email, fileReason(r)]),
    );
    out.push(
      env.NEWSLETTER_DB.prepare(
        `INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at)
         SELECT json_extract(value, '$[0]'), json_extract(value, '$[1]'), json_extract(value, '$[2]'), 1, 'import', ?, ?
           FROM json_each(?) WHERE true
         ${SUPPRESSION_UPSERT}`,
      ).bind(now, now, JSON.stringify(chunk)),
    );
  }
  return out;
}

async function run(env: Env, subscriberStatements: D1PreparedStatement[], others: D1PreparedStatement[]): Promise<number> {
  const all = [...subscriberStatements, ...others];
  if (!all.length) return 0;
  const results = await env.NEWSLETTER_DB.batch(all);
  return results.slice(0, subscriberStatements.length).reduce((n, res) => n + res.meta.changes, 0);
}

// insertRows writes rows with one status and one consent text (the plain
// import of active rows). Returns how many rows were inserted.
export async function insertRows(env: Env, rows: Candidate[], status: string, consentText: string): Promise<number> {
  const now = new Date().toISOString();
  return run(env, subscriberInserts(env, rows.map((r) => record(r, status, consentText, now)), now), []);
}

// writePlan writes every new row of a plan by its class: confirmed / pending /
// unsubscribed subscriber rows, suppressions for bounced and complained ones,
// plus the suppressions for existing subscribers and the raised ones
// (suppressExisting and strengthen: no subscriber row is written for them).
// suppressionsAdded counts every suppression written.
export async function writePlan(
  env: Env,
  fresh: Candidate[],
  opts: { confirmed: boolean; consentSource: string },
  suppressExisting: Candidate[] = [],
): Promise<{ imported: number; suppressionsAdded: number }> {
  const now = new Date().toISOString();
  const records: unknown[][] = [];
  const suppress: Candidate[] = [...suppressExisting];
  for (const r of fresh) {
    const k = classOf(r, opts.confirmed);
    if (k === "toSuppress") suppress.push(r);
    else if (k === "toUnsubscribed") records.push(record(r, "unsubscribed", opts.consentSource || UNSUBSCRIBED_CONSENT, now));
    else records.push(record(r, k === "toConfirm" ? "confirmed" : "pending", opts.consentSource, now));
  }
  const imported = await run(env, subscriberInserts(env, records, now), await suppressionInserts(env, suppress, now));
  return { imported, suppressionsAdded: suppress.length };
}
