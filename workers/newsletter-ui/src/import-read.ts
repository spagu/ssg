// Reading an uploaded list into candidate rows: delimiter, header or none,
// which column is which (import-columns.ts), and each cell checked by the
// sign-up form's rules (input.ts) or read by import-values.ts. Nothing here
// touches the database; import-plan.ts classifies what this returns.

import { detectDelimiter, parseCSV } from "./csv";
import type { CSVRecord } from "./csv";
import { detectColumns } from "./import-columns";
import type { Detected, Mapping, Role } from "./import-columns";
import type { Candidate, Invalid, PlanError } from "./import-plan";
import { inferDateOrder, isSet, normaliseStatus, parseDate, sourcePath } from "./import-values";
import type { DateOrder, FileStatus } from "./import-values";
import { normaliseEmail, normaliseLang, parseTags } from "./input";

export interface ReadOptions {
  mapping?: Mapping;
  dateOrder?: DateOrder;
}
export interface DateWarning {
  line: number;
  column: string; // the header the value came from
  value: string;
  reason: "unparseable" | "future";
}
// A sign-up source cell that is not a page path or URL (free text such as
// "landing form"): not stored, and reported so the drop is not silent.
export interface SourceWarning {
  line: number;
  value: string;
}
export interface Read {
  rows: Candidate[];
  invalid: Invalid[];
  total: number;
  detected: Detected & { dateOrder: Partial<Record<Role, DateOrder>> };
  dateWarnings: DateWarning[];
  sourceWarnings: SourceWarning[];
}

// The two dates that are stored. bouncedAt only implies a status.
const DATE_ROLES = ["signedUpAt", "unsubscribedAt"] as const;

// readRows parses and validates the file. maxRows counts data rows only.
export function readRows(csv: string, maxRows: number, opts: ReadOptions = {}): Read | PlanError {
  const delimiter = detectDelimiter(csv);
  const records = parseCSV(csv, delimiter);
  if (!records.length) return { error: "empty_file", detail: "the file has no header row", status: 400 };
  // Line 1 holding an address means the list has no header row.
  const headerless = normaliseEmail(records[0].fields[0]) !== null;
  const headers = records[0].fields.map((h, i) => (headerless ? `column ${i + 1}` : h.trim()));
  const cols = detectColumns(headers, opts.mapping ?? {}, delimiter, headerless);
  if ("error" in cols) return cols;
  const data = headerless ? records : records.slice(1);
  if (data.length > maxRows) {
    return { error: "too_many_rows", detail: `at most ${maxRows} rows per import (NEWSLETTER_IMPORT_MAX_ROWS)`, status: 413 };
  }
  const cell = (fields: string[], role: Role) => (cols.index[role] < 0 ? "" : (fields[cols.index[role]] ?? "").trim());

  // Slash dates are read in one order per column, decided by the whole column.
  const dateOrder: Partial<Record<Role, DateOrder>> = {};
  const undecided: string[] = [];
  for (const role of DATE_ROLES) {
    if (cols.index[role] < 0) continue;
    const order = opts.dateOrder ?? inferDateOrder(data.map((r) => cell(r.fields, role)));
    if (order) dateOrder[role] = order;
    else undecided.push(cols.detected.columns[role] as string);
  }
  const detected = { ...cols.detected, dateOrder };
  if (undecided.length) {
    return {
      error: "ambiguous_date_format",
      detail: `the dates in ${undecided.map((h) => `"${h}"`).join(", ")} read as day/month and as month/day; send dateOrder: "dmy" or "mdy"`,
      status: 400,
      extra: { columns: undecided, detected },
    };
  }

  const out: Read = { rows: [], invalid: [], total: data.length, detected, dateWarnings: [], sourceWarnings: [] };
  const now = Date.now();
  const date = (rec: CSVRecord, role: (typeof DATE_ROLES)[number]): string | undefined => {
    const raw = cell(rec.fields, role);
    if (!isSet(raw)) return undefined;
    const d = parseDate(raw, dateOrder[role] as DateOrder, now);
    if ("iso" in d) return d.iso;
    out.dateWarnings.push({ line: rec.line, column: cols.detected.columns[role] as string, value: raw.slice(0, 100), reason: d.warning });
    return undefined;
  };
  for (const rec of data) {
    const row = readRow(rec, (role) => cell(rec.fields, role), out);
    if (!row) continue;
    if (row.status !== "bounced" && row.status !== "complaint") {
      const signedUpAt = date(rec, "signedUpAt");
      if (signedUpAt) row.signedUpAt = signedUpAt;
    }
    if (row.status === "unsubscribed") {
      const unsubscribedAt = date(rec, "unsubscribedAt");
      if (unsubscribedAt) row.unsubscribedAt = unsubscribedAt;
    }
    out.rows.push(row);
  }
  return out;
}

// readRow checks one record; an invalid one is recorded and gives null. The
// optional fields are only set when the file has them, so a plain
// email,language,tags file gives exactly the rows it always gave.
function readRow(rec: CSVRecord, get: (role: Role) => string, out: Pick<Read, "invalid" | "sourceWarnings">): Candidate | null {
  const { line } = rec;
  const bad = (reason: Invalid["reason"], value: string) => (out.invalid.push({ line, reason, value: value.slice(0, 100) }), null);
  const email = normaliseEmail(get("email"));
  if (!email) return bad("invalid_email", get("email"));

  let status: FileStatus = "active";
  const rawStatus = get("status");
  if (rawStatus) {
    const s = normaliseStatus(rawStatus);
    if (!s) return bad("invalid_status", rawStatus);
    status = s;
  } else if (isSet(get("bouncedAt"))) status = "bounced";
  else if (isSet(get("unsubscribedAt"))) status = "unsubscribed";

  // "en_US" is how locales are often written; the form's rule wants "en-US".
  const rawLang = get("language");
  const language = rawLang ? normaliseLang(rawLang.replace("_", "-")) : null;
  if (rawLang && !language) return bad("invalid_language", rawLang);
  // Groups and lists are often separated by semicolons inside one cell.
  const tags = parseTags(get("tags").replaceAll(";", ","));
  if (!tags) return bad("invalid_tags", get("tags"));

  const row: Candidate = { line, email, language, tags: tags.join(",") };
  if (status !== "active") row.status = status;
  // A placeholder ("-", "none") is an empty cell, not a dropped value.
  const rawSource = get("sourcePage");
  const source = sourcePath(rawSource);
  if (source) row.sourcePage = source;
  else if (isSet(rawSource)) out.sourceWarnings.push({ line, value: rawSource.slice(0, 100) });
  return row;
}
