// Going back to an earlier revision of the settings is an ordinary
// publication of the old values (README of the round, decision 6). This
// works out, from the overlay as it was at that commit (GET
// api/revisions/{id}, redacted), which pending changes bring each field back:
// a value the old overlay set and the current one differs from is set again;
// a key the overlay sets now and did not then is reverted (removed). Pure.
//
// A secret the old overlay held as something other than a reference cannot
// be brought back by the panel (it never sees the value): that key is
// skipped and listed, so the person can re-enter it.

import { fromDraft, toDraft } from "./controls.js";
import { sameValue } from "./settings-changes.js";

const isObject = (v) => typeof v === "object" && v !== null && !Array.isArray(v);

// lookup: the value at `segments` in a document, {has, value}.
export function lookup(doc, segments) {
  let at = doc;
  for (const k of segments) {
    if (!isObject(at) || !Object.hasOwn(at, k)) return { has: false, value: undefined };
    at = at[k];
  }
  return { has: true, value: at };
}

// restoreChanges(fields, old) → {set: [{entry, value}], revert: [entry], skipped: [entry]}.
// `fields` are the Options tab's field entries (form-model.js fieldsOf);
// `old` the overlay at the revision (an object, redacted).
export function restoreChanges(fields, old) {
  const out = { set: [], revert: [], skipped: [] };
  const doc = isObject(old) ? old : {};
  for (const entry of fields) {
    const then = lookup(doc, entry.segments);
    if (!then.has) {
      if (entry.source === "overlay") out.revert.push(entry);
      continue;
    }
    const r = fromDraft(toDraft(entry.node, then.value));
    if (r.errors.length) {
      out.skipped.push(entry);
      continue;
    }
    const now = entry.source === "overlay" ? fromDraft(toDraft(entry.node, entry.value)).value : undefined;
    if (!sameValue(r.value, now)) out.set.push({ entry, value: r.value });
  }
  return out;
}

// leaves: the dotted paths and values an overlay sets, for showing a
// revision — a mapping is descended into, anything else is a leaf.
export function leaves(doc, at = []) {
  if (!isObject(doc)) return [];
  const out = [];
  for (const [k, v] of Object.entries(doc)) {
    const p = [...at, k];
    if (isObject(v) && Object.keys(v).length && !("set" in v && "reference" in v)) out.push(...leaves(v, p));
    else out.push({ path: p.join("."), value: v });
  }
  return out;
}
