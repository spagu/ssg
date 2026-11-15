// The frontmatter form (README of the round, decision 9): the simple keys a
// writer changes most — title, description, date, status, slug, tags,
// categories — as form fields; everything else stays in the frontmatter's
// own text, which the worker keeps byte for byte. The panel parses no YAML:
// the worker sends `fields` (the simple keys it read) and `fixed` (keys it
// will not change), and takes back only the keys that changed. Pure.

// FORM: the keys on the form, in order, and how each is edited.
export const FORM = [
  { key: "title", kind: "text" },
  { key: "description", kind: "textarea" },
  { key: "date", kind: "text" },
  // ssg publishes a file with frontmatter only with `status: publish`
  // (docs/CONTENT.md); anything else, or none, is a draft.
  { key: "status", kind: "status" },
  { key: "slug", kind: "text" },
  { key: "tags", kind: "list" },
  { key: "categories", kind: "list" },
];
const KIND = Object.fromEntries(FORM.map((f) => [f.key, f.kind]));

// inputOf: a frontmatter value as the form shows it (a list as "a, b";
// status as "published or not").
export function inputOf(kind, value) {
  if (kind === "status") return value === "publish";
  if (Array.isArray(value)) return value.join(", ");
  return value === null || value === undefined ? "" : String(value);
}

// valueOf: what the form's input is written back as. A list is split at
// commas; a number the input still spells is kept a number; status is
// "publish", or back to what it was (else "draft") when unticked.
export function valueOf(kind, input, original) {
  if (kind === "status") return input === true ? "publish" : typeof original === "string" && original !== "publish" ? original : "draft";
  if (kind === "list") return String(input).split(",").map((s) => s.trim()).filter(Boolean);
  return typeof original === "number" && String(original) === input ? original : String(input);
}

// formOf: the form's starting inputs for an article's `fields`.
export function formOf(fields) {
  return Object.fromEntries(FORM.map((f) => [f.key, inputOf(f.kind, fields[f.key])]));
}

// fieldChanges: {key: new value} for every key whose input changed. An
// absent key reads as empty (or unticked), so left so it is not added.
export function fieldChanges(original, inputs) {
  const out = {};
  for (const { key, kind } of FORM) {
    const before = inputOf(kind, original[key]);
    const now = inputs[key] ?? before;
    if (now === before) continue;
    out[key] = valueOf(kind, now, original[key]);
  }
  return out;
}

// shown: a value in the publish dialog's words ("a, b"; yes/no from `t`).
export const shown = (key, value, t) => {
  if (value === undefined) return t("form.unset");
  if (KIND[key] === "status") return String(value);
  const s = inputOf(KIND[key] ?? "text", value);
  return s === "" ? t("form.empty") : s;
};

// fieldLines: the publish dialog's lines for the form's changes.
export function fieldLines(original, inputs, t) {
  return Object.entries(fieldChanges(original, inputs)).map(([key, value]) => ({
    label: t(`form.${key}`),
    from: shown(key, original[key], t),
    to: shown(key, value, t),
  }));
}

// DATE: what ssg reads as a date (a day, optionally with a time).
export const DATE = /^\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)?$/;
// problems: the inputs the form cannot publish as they are ({key: string id}).
// requireTitle: a new article, or one that has a title, must keep one.
export function problems(inputs, requireTitle = true) {
  const out = {};
  if (requireTitle && String(inputs.title ?? "").trim() === "") out.title = "form.titleMissing";
  if (inputs.date && !DATE.test(String(inputs.date).trim())) out.date = "form.dateInvalid";
  if (inputs.slug && !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(inputs.slug)) out.slug = "form.slugInvalid";
  return out;
}
