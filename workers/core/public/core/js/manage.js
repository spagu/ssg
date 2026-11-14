// The Modules view's decisions (GO-149, /core/#modules), as plain functions
// over GET /api/core/manage — the views only lay the results out. No DOM.

import { LANGS } from "./i18n.js";

// The languages a label can be given in: the panels' five.
export const LABEL_LANGS = LANGS;

// moveOrder: the order after moving `id` one place (dir -1 up, +1 down), or
// null when it is already at that end.
export function moveOrder(ids, id, dir) {
  const i = ids.indexOf(id);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= ids.length) return null;
  const out = [...ids];
  [out[i], out[j]] = [out[j], out[i]];
  return out;
}

// readOnlyKey: why the view cannot change anything, or null when it can.
export function readOnlyKey(view) {
  if (view.writable) return null;
  return view.readOnly === "database_error" ? "manage.readonly.database_error" : "manage.readonly.no_database";
}

// shownTitle: what the bar calls a module in this language — its label, or
// the manifest's title.
export const shownTitle = (item, lang) => item.labels[lang] ?? item.title;

// statusLine: since when a module is up or down, from the observed history.
export function statusLine(item) {
  const s = item.status;
  if (!s) return null;
  return { key: s.status === "unreachable" ? "manage.status.down" : "manage.status.up", at: s.at };
}

// who: the person behind a change, in words — a shared password and an
// Access login without an address name nobody in particular.
export const whoKey = (actor) => (actor === "password" || actor === "access" || actor === "auth" ? `manage.who.${actor}` : null);

const ACTIONS = ["hidden", "shown", "labels", "note", "order", "import"];

// changeKey: the sentence for a change-log row.
export const changeKey = (change) => `manage.change.${ACTIONS.includes(change.action) ? change.action : "other"}`;

// draftOf: the editable copy of a module's labels and note.
export const draftOf = (item) => ({ labels: Object.fromEntries(LABEL_LANGS.map((l) => [l, item.labels[l] ?? ""])), note: item.note });

// draftPatch: what a Save must send — only what differs from the stored
// settings — or null when nothing does. Labels in other languages than the
// five (set by an import) are kept.
export function draftPatch(item, draft) {
  const labels = { ...item.labels };
  for (const l of LABEL_LANGS) {
    const v = (draft.labels[l] ?? "").trim();
    if (v) labels[l] = v;
    else delete labels[l];
  }
  const same = Object.keys(labels).length === Object.keys(item.labels).length && Object.entries(labels).every(([k, v]) => item.labels[k] === v);
  const out = {};
  if (!same) out.labels = labels;
  if (draft.note.trim() !== item.note) out.note = draft.note.trim();
  return Object.keys(out).length ? out : null;
}

// labelsText: a label map as "en: Shop, pl: Sklep", or null when empty.
export const labelsText = (labels) => {
  const parts = Object.entries(labels ?? {}).map(([l, v]) => `${l}: ${v}`);
  return parts.length ? parts.join(", ") : null;
};

// previewKey: the sentence for one field an import would change.
export function previewKey(change) {
  if (change.field === "hidden") return change.to ? "manage.import.hide" : "manage.import.show";
  if (change.field === "position") return change.to === null ? "manage.import.unplace" : "manage.import.place";
  if (change.field === "labels") return labelsText(change.to) ? "manage.import.labels" : "manage.import.nolabels";
  return change.to ? "manage.import.note" : "manage.import.nonote";
}

// tooLarge: a file over the import limit is refused before it is read.
export const tooLarge = (file, limit) => typeof file?.size === "number" && file.size > limit;

// panelText: where a hidden module's panel still answers.
export const panelKey = (item) => (item.panel ? "manage.cosmetic.panel" : "manage.cosmetic.nopanel");
