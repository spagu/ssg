// The settings edited in the panel and not yet published, and how a change
// reads in the publish dialog ("Site name: Example → My kitchen"). Pure —
// the store is a Map the views share; the words come in through `t`.
//
// A change is {path: ["mddb", "lang"], value}: what POST api/settings/publish
// takes. value null removes the key from .ssg.designer.json ("revert"), so the
// value in .ssg.yaml, or ssg's default, applies again.

import { isRedacted } from "./secret-ref.js";

export const keyOf = (segments) => JSON.stringify(segments);
const canonical = (v) => JSON.stringify(sorted(v)) ?? "undefined";
function sorted(v) {
  if (Array.isArray(v)) return v.map(sorted);
  if (typeof v !== "object" || v === null) return v;
  return Object.fromEntries(Object.keys(v).sort((a, b) => (a < b ? -1 : 1)).map((k) => [k, sorted(v[k])]));
}
export const sameValue = (a, b) => canonical(a) === canonical(b);

// createChanges: {set, revert, discard, get, has, size, list, clear, paths}.
//   set(entry, value, baseline) — baseline is the value the form started from;
//   setting a value equal to it drops the change instead of keeping a no-op.
export function createChanges() {
  const items = new Map();
  return {
    set(entry, value, baseline) {
      const k = keyOf(entry.segments);
      if (sameValue(value, baseline)) items.delete(k);
      else items.set(k, { entry, value });
    },
    // revert: remove the key from the overlay. Only a key the overlay holds
    // has anything to remove; for any other the pending edit is dropped.
    revert(entry) {
      const k = keyOf(entry.segments);
      if (entry.source === "overlay") items.set(k, { entry, value: null });
      else items.delete(k);
    },
    discard: (entry) => items.delete(keyOf(entry.segments)),
    get: (entry) => items.get(keyOf(entry.segments)),
    has: (entry) => items.has(keyOf(entry.segments)),
    get size() {
      return items.size;
    },
    // paths: the dotted paths with a change (for the "only changed" filter).
    paths: () => new Set([...items.values()].map((c) => c.entry.segments.join("."))),
    list: () => [...items.values()].map((c) => ({ path: c.entry.segments, value: c.value })),
    items: () => [...items.values()],
    clear: () => items.clear(),
  };
}

const MAX = 48;
const clip = (s) => (s.length > MAX ? s.slice(0, MAX - 1) + "\u2026" : s);

// formatValue: a value in a few words, for a list of changes. A secret is
// only ever a reference ($NAME) or "set" / "not set".
export function formatValue(v, t) {
  if (v === undefined) return t("value.unset");
  if (v === null) return t("value.reverted");
  if (isRedacted(v)) return v.reference ?? t(v.set ? "value.secretSet" : "value.secretUnset");
  if (typeof v === "boolean") return t(v ? "value.on" : "value.off");
  if (typeof v === "number") return String(v);
  if (typeof v === "string") return v === "" ? t("value.empty") : `\u201c${clip(v)}\u201d`;
  if (Array.isArray(v)) return t("value.items", { count: v.length });
  return t("value.keys", { count: Object.keys(v).length });
}

// describe: one change as {label, from, to, reverted} for the dialog.
// `labelOf(entry)` names the key in the panel's language.
export function describe(change, t, labelOf) {
  const { entry, value } = change;
  return {
    path: entry.segments.join("."),
    label: labelOf(entry),
    from: formatValue(entry.value, t),
    to: value === null ? t("value.reverted") : formatValue(value, t),
    reverted: value === null,
    advanced: entry.class !== "plain",
  };
}

// commitMessage: the message the dialog proposes — the keys' labels, up to
// three, then "and N more".
export function commitMessage(labels, t) {
  if (!labels.length) return "";
  const head = labels.slice(0, 3).join(", ");
  return labels.length > 3 ? t("publish.messageMore", { keys: head, count: labels.length - 3 }) : t("publish.messageFew", { keys: head });
}
