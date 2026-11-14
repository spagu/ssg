// Strict checks of what a person sends to change module settings (GO-149):
// one module's fields, a new order, a whole settings document to import.
// Unknown fields, unknown modules, wrong types and oversized texts are
// refused with a sentence that names the place — nothing is guessed or
// trimmed into shape. A leading underscore keeps this file out of the Pages
// route table.

import { problem } from "./_lib";

export const LIMITS = { label: 60, note: 500, modules: 500, position: 9999, bodyBytes: 16_384, importBytes: 65_536 };
export const EXPORT_KIND = "ssg-core-settings";
export const EXPORT_FORMAT = 1;

const LANG = /^[a-z]{2,3}$/;
const ID = /^[a-z][a-z0-9-]*$/;
// Labels are one line; a note may have line breaks. No other control
// characters in either.
const LABEL_BAD = /[\u0000-\u001f\u007f]/;
const NOTE_BAD = /[\u0000-\u0009\u000b-\u001f\u007f]/;

export type Checked<T> = { ok: true; value: T } | { ok: false; error: string; detail: string };
const bad = (detail: string, error = "invalid_settings"): { ok: false; error: string; detail: string } => ({ ok: false, error, detail });
const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

function onlyKeys(v: Record<string, unknown>, allowed: string[], where: string): string | null {
  const extra = Object.keys(v).filter((k) => !allowed.includes(k));
  return extra.length ? `${where}: unknown field ${extra.map((k) => JSON.stringify(k)).join(", ")}` : null;
}

// checkLabels: {lang: label}; an empty label removes that language's label.
export function checkLabels(raw: unknown, where: string): Checked<Record<string, string>> {
  if (!isObject(raw)) return bad(`${where} must be an object of {language: label}`);
  const out: Record<string, string> = {};
  for (const [lang, label] of Object.entries(raw)) {
    if (!LANG.test(lang)) return bad(`${where}: ${JSON.stringify(lang)} is not a language code (two or three lowercase letters)`);
    if (typeof label !== "string") return bad(`${where}.${lang} must be text`);
    const text = label.trim();
    if (text.length > LIMITS.label) return bad(`${where}.${lang} is longer than ${LIMITS.label} characters`);
    if (LABEL_BAD.test(text)) return bad(`${where}.${lang} contains a line break or a control character`);
    if (text) out[lang] = text;
  }
  return { ok: true, value: out };
}

export function checkNote(raw: unknown, where: string): Checked<string> {
  if (typeof raw !== "string") return bad(`${where} must be text`);
  const text = raw.trim();
  if (text.length > LIMITS.note) return bad(`${where} is longer than ${LIMITS.note} characters`);
  if (NOTE_BAD.test(text)) return bad(`${where} contains a control character`);
  return { ok: true, value: text };
}

export interface Patch {
  hidden?: boolean;
  labels?: Record<string, string>;
  note?: string;
}

// checkPatch: PATCH /api/core/manage/modules/{id} — at least one of hidden,
// labels (the whole map), note.
export function checkPatch(raw: unknown): Checked<Patch> {
  if (!isObject(raw)) return bad("the body must be a JSON object");
  const extra = onlyKeys(raw, ["hidden", "labels", "note"], "body");
  if (extra) return bad(extra);
  if (!("hidden" in raw) && !("labels" in raw) && !("note" in raw)) return bad("send at least one of hidden, labels, note");
  const out: Patch = {};
  if ("hidden" in raw) {
    if (typeof raw.hidden !== "boolean") return bad("hidden must be true or false");
    out.hidden = raw.hidden;
  }
  if ("labels" in raw) {
    const l = checkLabels(raw.labels, "labels");
    if (!l.ok) return l;
    out.labels = l.value;
  }
  if ("note" in raw) {
    const n = checkNote(raw.note, "note");
    if (!n.ok) return n;
    out.note = n.value;
  }
  return { ok: true, value: out };
}

// checkOrder: PUT /api/core/manage/order — {order: [...]} naming every module
// of the registry exactly once.
export function checkOrder(raw: unknown, known: string[]): Checked<string[]> {
  if (!isObject(raw) || !Array.isArray(raw.order)) return bad("the body must be {\"order\": [module ids]}");
  const extra = onlyKeys(raw, ["order"], "body");
  if (extra) return bad(extra);
  const order = raw.order as unknown[];
  if (order.some((id) => typeof id !== "string")) return bad("order must list module ids");
  const ids = order as string[];
  const unknown = ids.filter((id) => !known.includes(id));
  if (unknown.length) return bad(`not modules of this site: ${unknown.join(", ")}`, "unknown_modules");
  if (new Set(ids).size !== ids.length) return bad("a module is listed twice");
  const missing = known.filter((id) => !ids.includes(id));
  if (missing.length) return bad(`order must list every module; missing: ${missing.join(", ")}`);
  return { ok: true, value: ids };
}

export interface Imported {
  id: string;
  hidden: boolean;
  position: number | null;
  labels: Record<string, string>;
  note: string;
}

function checkEntry(raw: unknown, i: number): Checked<Imported> {
  const where = `modules[${i}]`;
  if (!isObject(raw)) return bad(`${where} must be an object`);
  const extra = onlyKeys(raw, ["id", "hidden", "position", "labels", "note"], where);
  if (extra) return bad(extra);
  if (typeof raw.id !== "string" || !ID.test(raw.id)) return bad(`${where}.id must be a module id`);
  const hidden = raw.hidden ?? false;
  if (typeof hidden !== "boolean") return bad(`${where}.hidden must be true or false`);
  const position = raw.position ?? null;
  if (position !== null && (!Number.isInteger(position) || (position as number) < 0 || (position as number) > LIMITS.position)) {
    return bad(`${where}.position must be a whole number from 0 to ${LIMITS.position}, or null`);
  }
  const labels = checkLabels(raw.labels ?? {}, `${where}.labels`);
  if (!labels.ok) return labels;
  const note = checkNote(raw.note ?? "", `${where}.note`);
  if (!note.ok) return note;
  return { ok: true, value: { id: raw.id, hidden, position: position as number | null, labels: labels.value, note: note.value } };
}

// checkImport: a document GET /api/core/manage/export wrote, for this site's
// modules. Every module it names must exist here, each once.
export function checkImport(raw: unknown, known: string[]): Checked<Imported[]> {
  if (!isObject(raw)) return bad("the file is not a JSON object");
  const extra = onlyKeys(raw, ["kind", "format", "exportedAt", "modules"], "the file");
  if (extra) return bad(extra);
  if (raw.kind !== EXPORT_KIND) return bad(`the file is not a core settings export (kind must be "${EXPORT_KIND}")`);
  if (raw.format !== EXPORT_FORMAT) return bad(`the file has format ${JSON.stringify(raw.format)}; this core reads format ${EXPORT_FORMAT}`);
  if ("exportedAt" in raw && typeof raw.exportedAt !== "string") return bad("exportedAt must be text");
  if (!Array.isArray(raw.modules)) return bad("modules must be a list");
  if (raw.modules.length > LIMITS.modules) return bad(`modules lists more than ${LIMITS.modules} entries`);
  const out: Imported[] = [];
  for (const [i, m] of raw.modules.entries()) {
    const e = checkEntry(m, i);
    if (!e.ok) return e;
    out.push(e.value);
  }
  const ids = out.map((m) => m.id);
  const unknown = ids.filter((id) => !known.includes(id));
  if (unknown.length) return bad(`not modules of this site: ${[...new Set(unknown)].join(", ")}`, "unknown_modules");
  if (new Set(ids).size !== ids.length) return bad("a module is listed twice");
  return { ok: true, value: out };
}

// readBody reads a JSON body of at most `max` bytes.
export async function readBody(request: Request, max: number): Promise<Checked<unknown>> {
  const declared = Number(request.headers.get("content-length") ?? "0");
  if (declared > max) return bad(`the body is larger than ${max} bytes`, "too_large");
  const text = await request.text();
  if (new TextEncoder().encode(text).length > max) return bad(`the body is larger than ${max} bytes`, "too_large");
  try {
    return { ok: true, value: JSON.parse(text) as unknown };
  } catch {
    return bad("the body is not JSON", "invalid_json");
  }
}

// refused: the answer for a failed check — 413 too large, 400 not JSON, 422
// for everything that is JSON but not acceptable.
const STATUS: Record<string, number> = { too_large: 413, invalid_json: 400 };
export const refused = (c: { error: string; detail: string }): Response => problem(c.error, STATUS[c.error] ?? 422, c.detail);
