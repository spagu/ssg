// What a settings publication changes, and how much trust each change needs
// (README of the round, decision 8; GO-177's notes for designer-ui):
//
//   - the overlay before and after are compared key by key; a key whose value
//     differs is a changed key ("leaf"). Mappings are descended into only
//     where ssg merges them key by key — the file's value there is a mapping
//     or absent. Where the file holds another form (`story: true`), or could
//     not be read, the object replaces it whole, so the object is the leaf
//     and its own class counts (mddb is "executes", mddb.lang "plain").
//   - an entry added to or removed from a map (a new external source, a new
//     model endpoint) is a leaf of its own: the entry's class counts.
//   - a leaf needs the strongest class of every node its old and new values
//     reach (a `workers` entry holding auth.token reaches "secret").
//   - a new value holding an environment reference ($NAME) needs
//     designer-ui:advanced whatever the key: the reference reads the build's
//     environment, and a key like `variables` would publish it.

import { lookup } from "./config-read";
import type { Doc } from "../portable/config-files";
import { canonicalJSON } from "./overlay-write";
import { ENV_REF } from "./overlay-validate";
import { strictReference } from "./redact";
import { ROOT, child, classesIn, elementOf, secretFormatOf } from "./schema";
import type { SchemaNode, SettingClass } from "./schema";

export interface ChangedKey {
  path: string; // dotted, for people
  classes: SettingClass[]; // every class the old and new values reach
  reference: boolean; // the new value holds a $NAME reference
}

const isObject = (v: unknown): v is Doc => typeof v === "object" && v !== null && !Array.isArray(v);
const sameJSON = (a: unknown, b: unknown) => canonicalJSON(a) === canonicalJSON(b);

// overlayLeaves: the dotted paths an overlay sets, sorted (what ssg's
// OverlayKeys lists: a non-empty mapping is descended into).
export function overlayLeaves(doc: Doc, at = ""): string[] {
  const out: string[] = [];
  for (const [k, v] of Object.entries(doc)) {
    const p = at ? `${at}.${k}` : k;
    if (isObject(v) && Object.keys(v).length) out.push(...overlayLeaves(v, p));
    else out.push(p);
  }
  return out.sort((a, b) => (a < b ? -1 : 1));
}

// hasReference: a string inside `v` holds $NAME.
export function hasReference(v: unknown): boolean {
  if (typeof v === "string") return ENV_REF.test(v);
  if (Array.isArray(v)) return v.some(hasReference);
  return isObject(v) && Object.values(v).some(hasReference);
}

export interface FileView { values: Doc; unread: Record<string, string> }

// merges: does ssg merge an overlay mapping at `segments` key by key?
function merges(file: FileView, segments: string[]): boolean {
  if (segments.length && segments[0] in file.unread) return false;
  const at = lookup(file.values, segments);
  return !at.has || isObject(at.value);
}

// changedKeys compares two overlays (before = the file in the repository).
export function changedKeys(before: Doc, after: Doc, file: FileView): ChangedKey[] {
  const out: ChangedKey[] = [];
  const leaf = (old: unknown, now: unknown, n: SchemaNode | null, segs: string[]) => {
    if (sameJSON(old, now)) return;
    // An unknown key (only an old one can reach here: validation refuses new
    // ones) is treated as the most sensitive kind.
    const classes = n === null ? new Set<SettingClass>(["executes"]) : classesIn(now, n, classesIn(old, n));
    out.push({ path: segs.join("."), classes: [...classes].sort((a, b) => (a < b ? -1 : 1)) /* a set: never equal */, reference: hasReference(now) });
  };
  const walk = (old: unknown, now: unknown, n: SchemaNode | null, segs: string[]) => {
    // An absent side is an empty mapping: removing an overlay mapping removes
    // each of its keys, which is what their classes are about.
    const prev = old === undefined ? {} : isObject(old) ? old : null;
    const next = now === undefined ? {} : isObject(now) ? now : null;
    if (n === null || (n.type !== "object" && n.type !== "map") || !prev || !next || !merges(file, segs)) return leaf(old, now, n, segs);
    for (const k of new Set([...Object.keys(prev), ...Object.keys(next)])) {
      const at = [...segs, k];
      // A map entry that appears or disappears from what ssg loads is a whole
      // thing (a new external source, a new model endpoint): its own class
      // counts, not only its fields'. An entry of the file stays either way.
      const kept = lookup(file.values, at).has || (Object.hasOwn(prev, k) && Object.hasOwn(next, k));
      if (n.type === "map" && !kept) leaf(prev[k], next[k], child(n, k), at);
      else walk(prev[k], next[k], child(n, k), at);
    }
  };
  walk(before, after, ROOT, []);
  return out.sort((a, b) => (a.path < b.path ? -1 : 1));
}

// needsAdvanced: the changed keys designer-ui:settings alone may not publish.
export const needsAdvanced = (changes: ChangedKey[]): string[] =>
  changes.filter((c) => c.reference || c.classes.some((k) => k !== "plain")).map((c) => c.path);

// secretShapeProblems: values inside secret keys that are not a reference in
// the strict form this worker accepts (redact.ts STRICT) — "" (unset) passes.
export function secretShapeProblems(v: unknown, n: SchemaNode | null, path: string, out: string[] = []): string[] {
  if (n === null) return out;
  const format = secretFormatOf(n);
  if (format && typeof v === "string") {
    if (v !== "" && !strictReference(format, v)) out.push(`${path}: a secret is written only as an environment reference, e.g. "$NAME"`);
  } else if (Array.isArray(v)) {
    v.forEach((e, i) => secretShapeProblems(e, elementOf(n, e), `${path}[${i}]`, out));
  } else if (isObject(v)) {
    for (const [k, e] of Object.entries(v)) secretShapeProblems(e, child(n, k), path ? `${path}.${k}` : k, out);
  }
  return out;
}
