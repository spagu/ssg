// POST {base}/api/import/preview — what an import would do, writing nothing
// POST {base}/api/import         — the import
//
// The body is either the CSV itself (content-type text/csv, options in the
// query string: ?consentSource=…&confirmed=true&dateOrder=dmy&mapping={…}) or
// JSON {"csv": "…", "consentSource": "…", "confirmed": true, "dateOrder": "dmy",
// "mapping": {"email": "<header>", …}}. A raw body may be UTF-8 or UTF-16.
// The file may come from any common mailing tool: import-read.ts finds the
// delimiter and the columns, import-values.ts reads statuses and dates.
//
// Rules, all enforced here, none optional:
//   - consentSource is required for an import that adds people who may be
//     mailed (active or pending in the file), and stored as their
//     consent_text: a row that says "agreed" without saying to what proves
//     nothing. Active rows are `confirmed` only with consentSource AND
//     confirmed:true, otherwise `pending`. (No confirmation mail is sent for
//     pending imports; an admin sends it separately.)
//   - unsubscribed in the file → inserted as `unsubscribed`, so a later import
//     of an older copy of the list cannot add the person as active; bounced or
//     complained in the file → a suppression, never a subscriber row.
//   - existing rows (any status) and suppressed addresses are skipped, never
//     updated: an import can never undo an unsubscribe or an erasure. An
//     existing subscriber the file marks bounced or complained is still
//     suppressed (the row stays as it is; the suppression stops mail), and an
//     address suppressed for a weaker reason (soft, manual) that the file
//     marks bounced or complained has its suppression raised to that reason.
//   - every new row gets a fresh random token of the same shape the
//     `newsletter` template generates, so its unsubscribe links work there.
//   - all writes go in one db.batch: one transaction, all rows or none.

import type { RouteContext } from "./router";
import { intVar, json } from "./lib";
import type { Env } from "./lib";
import { decodeCSV } from "./csv";
import { isTrue, readJSONObject, text } from "./input";
import { parseMapping } from "./import-columns";
import type { Mapping } from "./import-columns";
import { classOf, plan, summarise } from "./import-plan";
import type { Plan, PlanError } from "./import-plan";
import { isDateOrder } from "./import-values";
import type { DateOrder } from "./import-values";
import { writePlan } from "./import-write";

export { INSERT_CHUNK, insertRows, newToken } from "./import-write";

export const DEFAULT_MAX_ROWS = 20_000;
export const BYTES_PER_ROW = 1024; // the byte cap scales with the row cap

interface ImportRequest {
  csv: string;
  consentSource: string;
  confirmed: boolean;
  mapping: Mapping;
  dateOrder?: DateOrder;
}

const fail = (e: PlanError) => json({ error: e.error, detail: e.detail, ...e.extra }, e.status);

// options validates the column override and the date order of either shape.
function options(rawMapping: unknown, rawOrder: unknown): Pick<ImportRequest, "mapping" | "dateOrder"> | PlanError {
  const mapping = parseMapping(rawMapping);
  if ("error" in mapping) return mapping as PlanError;
  if (rawOrder === undefined || rawOrder === null || rawOrder === "") return { mapping };
  if (!isDateOrder(rawOrder)) return { error: "invalid_date_order", detail: 'dateOrder is "dmy" or "mdy"', status: 400 };
  return { mapping, dateOrder: rawOrder };
}

// queryMapping reads ?mapping= as JSON; anything unreadable is passed on as
// is, for parseMapping to refuse.
function queryMapping(raw: string | null): unknown {
  if (raw === null) return undefined;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

// readImport reads either body shape, refusing oversized bodies before parsing.
export async function readImport(request: Request, url: URL, maxBytes: number): Promise<ImportRequest | PlanError> {
  const tooBig: PlanError = { error: "too_large", detail: `the import is limited to ${maxBytes} bytes`, status: 413 };
  if (Number(request.headers.get("content-length") ?? 0) > maxBytes) return tooBig;
  const type = (request.headers.get("content-type") ?? "").toLowerCase();
  let out: ImportRequest;
  if (type.includes("application/json")) {
    const body = await readJSONObject(request);
    if (!body || typeof body.csv !== "string") return { error: "invalid_body", detail: 'send {"csv": "…", "consentSource": "…"}', status: 400 };
    const opts = options(body.mapping, body.dateOrder);
    if ("error" in opts) return opts;
    out = { csv: body.csv, consentSource: text(body.consentSource, 1000), confirmed: body.confirmed === true, ...opts };
  } else if (type.includes("text/csv") || type.includes("text/plain")) {
    const p = url.searchParams;
    const opts = options(queryMapping(p.get("mapping")), p.get("dateOrder"));
    if ("error" in opts) return opts;
    const csv = decodeCSV(new Uint8Array(await request.arrayBuffer()));
    out = { csv, consentSource: text(p.get("consentSource"), 1000), confirmed: isTrue(p.get("confirmed")), ...opts };
  } else {
    return { error: "unsupported_media_type", detail: "send text/csv or application/json", status: 415 };
  }
  return out.csv.length > maxBytes ? tooBig : out;
}

const limits = (env: Env) => {
  const maxRows = intVar(env.NEWSLETTER_IMPORT_MAX_ROWS, DEFAULT_MAX_ROWS);
  return { maxRows, maxBytes: maxRows * BYTES_PER_ROW };
};

async function prepare({ request, env, url }: RouteContext): Promise<{ req: ImportRequest; plan: Plan } | PlanError> {
  const { maxRows, maxBytes } = limits(env);
  const req = await readImport(request, url, maxBytes);
  if ("error" in req) return req;
  const p = await plan(env, req.csv, maxRows, { mapping: req.mapping, dateOrder: req.dateOrder });
  return "error" in p ? p : { req, plan: p };
}

export async function importPreview(c: RouteContext): Promise<Response> {
  const r = await prepare(c);
  return "error" in r ? fail(r) : json(summarise(r.plan, r.req.confirmed));
}

// needsConsent: a consentSource is asked for unless every row to write is an
// unsubscribe or a suppression — people who will never be mailed. A file with
// nothing to write still asks, as it always did.
const needsConsent = (p: Plan): boolean =>
  (!p.fresh.length && !p.suppressExisting.length && !p.strengthen.length) || p.fresh.some((c) => !["toUnsubscribed", "toSuppress"].includes(classOf(c, false)));

export async function importRun(c: RouteContext): Promise<Response> {
  const r = await prepare(c);
  if ("error" in r) return fail(r);
  if (!r.req.consentSource && needsConsent(r.plan)) {
    return json({ error: "consent_source_required", detail: "say where and how these people consented (consentSource)" }, 400);
  }
  const written = await writePlan(c.env, r.plan.fresh, r.req, [...r.plan.suppressExisting, ...r.plan.strengthen]);
  const status = r.req.confirmed ? "confirmed" : "pending";
  return json({ ...summarise(r.plan, r.req.confirmed), ...written, status });
}
