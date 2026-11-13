// The import's plan: a CSV turned into rows classified as new / duplicate in
// the file / already in the table / suppressed / invalid, and each new row by
// what it becomes (classOf), before anything is written. The preview shows
// it; the import writes its `fresh` rows and the `suppressExisting`
// suppressions and nothing else, so both can never disagree.
//
// D1 limits shape the queries: a statement takes at most 100 bound parameters
// and an invocation runs a bounded number of statements. So a lookup passes one
// chunk of addresses as ONE JSON parameter (json_each), instead of one
// parameter or one statement per row: 10 000 rows cost a handful of statements.

import type { Env } from "./lib";
import { readRows } from "./import-read";
import type { DateWarning, Read, ReadOptions, SourceWarning } from "./import-read";
import type { FileStatus } from "./import-values";
import { reasonRank, suppressionKey } from "./suppress";

export { readRows };

export const LOOKUP_CHUNK = 2000; // addresses per lookup statement (~100 KB of JSON)
export const SAMPLE_SIZE = 10;
export const INVALID_LIST_MAX = 100;
export const SOURCE_WARNING_MAX = 20;

// A row to write. The optional fields are present only when the file says so:
// no `status` = an active subscriber (confirmed or pending by the admin's
// choice), no date = now.
export interface Candidate {
  line: number;
  email: string;
  language: string | null;
  tags: string;
  status?: Exclude<FileStatus, "active">;
  signedUpAt?: string;
  unsubscribedAt?: string;
  sourcePage?: string;
}
export interface Entry {
  line: number;
  email: string;
}
export interface Invalid {
  line: number;
  reason: "invalid_email" | "invalid_language" | "invalid_tags" | "invalid_status";
  value: string;
}
export interface Plan {
  total: number;
  fresh: Candidate[];
  duplicate: Entry[];
  existing: Entry[];
  suppressed: Entry[];
  // Addresses already in the table (so also in `existing` or `suppressed`)
  // that the file marks bounced or complained and that are not yet suppressed
  // that strongly: the import suppresses them; their subscriber row is never
  // touched — the suppression alone stops mail.
  suppressExisting: Candidate[];
  // Addresses not in the table but already suppressed with a weaker reason
  // (soft, manual) that the file marks bounced or complained: the import
  // raises the suppression to the file's reason. They stay in `suppressed`.
  strengthen: Candidate[];
  invalid: Invalid[];
  detected: Read["detected"];
  dateWarnings: DateWarning[];
  sourceWarnings: SourceWarning[];
}
// extra: more fields for the error response (what was detected, to fix by).
export type PlanError = { error: string; detail: string; status: number; extra?: Record<string, unknown> };

// existingEmails returns which addresses are already in subscribers, any status.
export async function existingEmails(env: Env, emails: string[]): Promise<Set<string>> {
  const found = new Set<string>();
  for (let i = 0; i < emails.length; i += LOOKUP_CHUNK) {
    const { results } = await env.NEWSLETTER_DB.prepare(
      "SELECT email FROM subscribers WHERE email IN (SELECT value FROM json_each(?))",
    )
      .bind(JSON.stringify(emails.slice(i, i + LOOKUP_CHUNK)))
      .all<{ email: string }>();
    for (const r of results) found.add(r.email);
  }
  return found;
}

// suppressionReasons returns the reason of every suppressed key among `keys`,
// one JSON parameter per chunk like existingEmails.
export async function suppressionReasons(env: Env, keys: string[]): Promise<Map<string, string>> {
  const found = new Map<string, string>();
  for (let i = 0; i < keys.length; i += LOOKUP_CHUNK) {
    const { results } = await env.NEWSLETTER_DB.prepare(
      "SELECT email_hash, reason FROM suppressions WHERE email_hash IN (SELECT value FROM json_each(?))",
    )
      .bind(JSON.stringify(keys.slice(i, i + LOOKUP_CHUNK)))
      .all<{ email_hash: string; reason: string }>();
    for (const r of results) found.set(r.email_hash, r.reason);
  }
  return found;
}

// fileReason is the suppression a bounced or complained row asks for.
export const fileReason = (c: Candidate): "hard" | "complaint" | null =>
  c.status === "complaint" ? "complaint" : c.status === "bounced" ? "hard" : null;

// plan classifies every row. Precedence: invalid, then a repeat within the
// file (the first occurrence wins), then suppressed, then already present.
// An address already in the table keeps its row whatever the file says about
// it (unsubscribed, bounced): an import never touches an existing row. A
// bounce or complaint for it still becomes a suppression (suppressExisting)
// unless one at least as strong is already there.
export async function plan(env: Env, csv: string, maxRows: number, opts: ReadOptions = {}): Promise<Plan | PlanError> {
  const read = readRows(csv, maxRows, opts);
  if ("error" in read) return read;
  const out: Plan = {
    total: read.total,
    fresh: [],
    duplicate: [],
    existing: [],
    suppressed: [],
    suppressExisting: [],
    strengthen: [],
    invalid: read.invalid,
    detected: read.detected,
    dateWarnings: read.dateWarnings,
    sourceWarnings: read.sourceWarnings,
  };

  const seen = new Set<string>();
  const unique: Candidate[] = [];
  for (const r of read.rows) {
    if (seen.has(r.email)) out.duplicate.push({ line: r.line, email: r.email });
    else (seen.add(r.email), unique.push(r));
  }
  const keys = await Promise.all(unique.map((r) => suppressionKey(r.email)));
  const held = await suppressionReasons(env, keys);
  const present = await existingEmails(env, unique.map((r) => r.email));
  unique.forEach((r, i) => {
    const entry = { line: r.line, email: r.email };
    const reason = held.get(keys[i]);
    const wanted = fileReason(r);
    const stronger = wanted !== null && (reason === undefined || reasonRank(reason) < reasonRank(wanted));
    if (stronger && present.has(r.email)) out.suppressExisting.push(r);
    else if (stronger && reason !== undefined) out.strengthen.push(r);
    if (reason !== undefined) out.suppressed.push(entry);
    else if (present.has(r.email)) out.existing.push(entry);
    else out.fresh.push(r);
  });
  return out;
}

const entry = (c: Candidate | Entry): Entry => ({ line: c.line, email: c.email });

// What the import does with a new row.
export const CLASSES = ["toConfirm", "toPending", "toUnsubscribed", "toSuppress"] as const;
export type RowClass = (typeof CLASSES)[number];

export function classOf(c: Candidate, confirmed: boolean): RowClass {
  if (c.status === "bounced" || c.status === "complaint") return "toSuppress";
  if (c.status === "unsubscribed") return "toUnsubscribed";
  return c.status === "pending" || !confirmed ? "toPending" : "toConfirm";
}

// summarise is the plan as the API answers it: counts, a few examples of each
// kind, the first invalid rows with the reason, to fix the file by, what was
// detected in the file and what each new row will become (`confirmed`: the
// request's choice for active rows). toSuppressExisting counts the existing
// subscribers the import suppresses (see Plan.suppressExisting); they are
// not in counts.new, and stay in counts.existing / counts.suppressed.
// toStrengthen counts the suppressed addresses whose suppression the file
// raises (Plan.strengthen); they stay in counts.suppressed.
export function summarise(p: Plan, confirmed = false) {
  const byClass = Object.fromEntries(CLASSES.map((k) => [k, [] as Candidate[]])) as Record<RowClass, Candidate[]>;
  for (const c of p.fresh) byClass[classOf(c, confirmed)].push(c);
  const classCounts = {
    ...(Object.fromEntries(CLASSES.map((k) => [k, byClass[k].length])) as Record<RowClass, number>),
    toSuppressExisting: p.suppressExisting.length,
    toStrengthen: p.strengthen.length,
  };
  const classSample = {
    ...(Object.fromEntries(CLASSES.map((k) => [k, byClass[k].slice(0, SAMPLE_SIZE).map(entry)])) as Record<RowClass, Entry[]>),
    toSuppressExisting: p.suppressExisting.slice(0, SAMPLE_SIZE).map(entry),
    toStrengthen: p.strengthen.slice(0, SAMPLE_SIZE).map(entry),
  };
  return {
    total: p.total,
    counts: { new: p.fresh.length, duplicate: p.duplicate.length, existing: p.existing.length, suppressed: p.suppressed.length, invalid: p.invalid.length },
    sample: {
      new: p.fresh.slice(0, SAMPLE_SIZE).map(entry),
      duplicate: p.duplicate.slice(0, SAMPLE_SIZE),
      existing: p.existing.slice(0, SAMPLE_SIZE),
      suppressed: p.suppressed.slice(0, SAMPLE_SIZE),
    },
    invalid: p.invalid.slice(0, INVALID_LIST_MAX),
    detected: p.detected,
    classCounts,
    classSample,
    dateWarnings: p.dateWarnings.slice(0, INVALID_LIST_MAX),
    dateWarningCount: p.dateWarnings.length,
    sourceWarnings: p.sourceWarnings.length,
    sourceWarningSample: p.sourceWarnings.slice(0, SOURCE_WARNING_MAX),
  };
}
