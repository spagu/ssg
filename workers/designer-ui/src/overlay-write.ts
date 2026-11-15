// Building the next .ssg.designer.json from the current one and a list of
// changes, and writing it the one way it is always written: keys sorted at
// every level, two-space indent, a newline at the end — so two publications
// of the same settings are the same bytes and a diff in the repository shows
// only what changed (GO-178).
//
// A change names a key by its path of keys (["mddb", "lang"]) — a key of a
// map may hold a dot (a model named "gpt-4.1"), so a dotted string could not
// say where one key ends. `value` sets it; null removes it from the overlay,
// so the file's value (or the default) applies again. Lists are replaced
// whole, as ssg merges them: a path never goes into a list.

import type { Doc } from "./config-read";
import { OpError } from "./ops";

export interface SettingChange { path: string[]; value: unknown }

const isObject = (v: unknown): v is Doc => typeof v === "object" && v !== null && !Array.isArray(v);
const byKey = (a: string, b: string) => (a < b ? -1 : 1); // keys of one object are never equal

// sorted: a copy with every object's keys in order.
function sorted(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sorted);
  if (!isObject(v)) return v;
  return Object.fromEntries(Object.keys(v).sort(byKey).map((k) => [k, sorted(v[k])]));
}

// canonicalJSON: one line, keys sorted — for comparing values.
export const canonicalJSON = (v: unknown): string => JSON.stringify(sorted(v)) ?? "undefined";

// serializeOverlay: the file's bytes.
export const serializeOverlay = (doc: Doc): string => JSON.stringify(sorted(doc), null, 2) + "\n";

export const MAX_CHANGES = 200;

// parseChanges checks the shape of the request's changes (not the schema:
// validateOverlay does that on the whole result).
export function parseChanges(raw: unknown): SettingChange[] {
  if (!Array.isArray(raw) || raw.length === 0) throw new OpError("invalid_change", 400, "changes must be a non-empty list");
  if (raw.length > MAX_CHANGES) throw new OpError("invalid_change", 400, `at most ${MAX_CHANGES} changes per publication`);
  return raw.map((c, i) => {
    if (!isObject(c) || !("value" in c)) throw new OpError("invalid_change", 400, `changes[${i}] must be {path, value}`);
    const path = c.path;
    if (!Array.isArray(path) || path.length === 0 || path.some((s) => typeof s !== "string" || s === "" || s === "__proto__")) {
      throw new OpError("invalid_change", 400, `changes[${i}].path must be a non-empty list of keys`);
    }
    return { path: path as string[], value: c.value };
  });
}

// set writes `value` at `path`, making objects on the way (a value that is
// not an object is replaced by one).
function set(doc: Doc, path: string[], value: unknown): void {
  let at = doc;
  for (const k of path.slice(0, -1)) {
    if (!isObject(at[k])) at[k] = {};
    at = at[k] as Doc;
  }
  at[path[path.length - 1]] = structuredClone(value);
}

// remove deletes the key at `path`, and every object the removal leaves empty.
function remove(doc: Doc, path: string[]): void {
  const [k, ...rest] = path;
  if (!rest.length) {
    delete doc[k];
    return;
  }
  const sub = doc[k];
  if (!isObject(sub)) return;
  remove(sub, rest);
  if (Object.keys(sub).length === 0) delete doc[k];
}

// applyChanges returns the next overlay, in order; `before` is not modified.
export function applyChanges(before: Doc, changes: SettingChange[]): Doc {
  const doc = structuredClone(before);
  for (const { path, value } of changes) {
    if (value === null) remove(doc, path);
    else set(doc, path, value);
  }
  return doc;
}
