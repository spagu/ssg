// One article as the editor holds it: what the worker sent (the version it
// was read at, the frontmatter split from the body) and what the writer made
// of it. The same object is the draft stored in the browser, and it says
// which change to publish (POST api/changes, GO-170):
//
//   form edits      {op: "put", path, version, fields: {changed keys}, body?}
//   frontmatter     {op: "put", path, version, frontmatterRaw, body?}
//   as text
//   no frontmatter  {op: "put", path, version, text: "---\n…---\n" + body}
//   yet, now typed
//   a new article   {op: "put", path, version: null, fields (status always), body}
//
// The form and the frontmatter text exclude each other (the worker refuses
// both at once): the view locks one while the other has changes. Pure.

import { FORM, fieldChanges, formOf, problems } from "./fm-form.js";

// docOf: a document for an article the worker answered (GET api/content/{path}).
export function docOf(article, key = article.path) {
  return {
    key, path: article.path, section: null, origin: article.origin ?? "git", isNew: false,
    version: article.version, commit: article.commit, format: article.format, bom: article.bom,
    original: article.fields, fixed: article.fixed, inputs: formOf(article.fields),
    originalRaw: article.frontmatterRaw, raw: article.frontmatterRaw, rawEdited: false,
    originalBody: article.body, body: article.body,
  };
}

// newDoc: a new article at `path` (from GET api/content/new-path), a draft.
export function newDoc({ key, path, section, origin = "git", title, date }) {
  return {
    ...docOf({ path, version: null, commit: null, format: "yaml", bom: false, fields: {}, fixed: [], frontmatterRaw: "", body: "", origin }, key),
    section, isNew: true, inputs: { ...formOf({}), title, date, status: false },
  };
}

export const formUsable = (doc) => doc.isNew || doc.format === "yaml";
export const rawUsable = (doc) => !doc.isNew && doc.format !== "unclosed";
export const formChanges = (doc) => (formUsable(doc) && !doc.rawEdited ? fieldChanges(doc.original, doc.inputs) : {});

// withRaw: the document with its frontmatter typed as text.
export const withRaw = (doc, raw) => ({ ...doc, raw, rawEdited: raw !== doc.originalRaw });

const withEol = (s) => (s === "" || s.endsWith("\n") ? s : s + "\n");

// changeOf: the change publishing this document sends, or null when there
// is nothing to publish.
export function changeOf(doc) {
  const base = { op: "put", path: doc.path, version: doc.isNew ? null : doc.version };
  const body = doc.body !== doc.originalBody ? { body: doc.body } : {};
  // A new article names its status either way: ssg warns about a file with
  // no status line, and a draft should be one on purpose.
  if (doc.isNew) return { ...base, fields: { ...fieldChanges({}, doc.inputs), status: doc.inputs.status === true ? "publish" : "draft" }, body: doc.body };
  if (doc.rawEdited && doc.format === "none") return { ...base, text: doc.raw.trim() === "" ? doc.body : `---\n${withEol(doc.raw)}---\n${doc.body}` };
  if (doc.rawEdited) return { ...base, frontmatterRaw: doc.raw, body: doc.body };
  const fields = formChanges(doc);
  const has = Object.keys(fields).length > 0;
  if (!has && !("body" in body)) return null;
  return { ...base, ...(has ? { fields } : {}), ...body };
}

export const isDirty = (doc) => changeOf(doc) !== null;

// titleOf: the title the panel shows — the form's, else the file's name.
export const titleOf = (doc) => String(doc.inputs.title).trim() || doc.path.split("/").pop().replace(/\.md$/, "");

// formProblems: what keeps the form's values from being published.
export const formProblems = (doc) => (formUsable(doc) && !doc.rawEdited ? problems(doc.inputs, doc.isNew || Object.hasOwn(doc.original, "title")) : {});

// sameAs: whether publishing the document would leave the article as it is.
export function sameAs(doc, article) {
  if (doc.body !== article.body) return false;
  if (doc.rawEdited) return doc.raw === article.frontmatterRaw;
  return Object.entries(formChanges(doc)).every(([k, v]) => JSON.stringify(v) === JSON.stringify(article.fields[k]));
}

// openChoice: what opening an article with a stored draft means —
//   "repository"  no draft, or the draft says what the article says now
//   "draft"       the draft was made on this very version: carry on
//   "choose"      the article changed since: mine / repository / compare
export function openChoice(draft, article) {
  if (!draft || sameAs(draft, article)) return "repository";
  return draft.version === article.version ? "draft" : "choose";
}

// rebase: the draft's edits on top of the article as it is now ("mine"):
// the body and a typed frontmatter as the draft has them, and the form keys
// the draft changed over the article's current values.
export function rebase(draft, article) {
  const doc = docOf(article, draft.key);
  const edited = fieldChanges(draft.original, draft.inputs);
  const inputs = { ...doc.inputs };
  for (const { key } of FORM) if (Object.hasOwn(edited, key)) inputs[key] = draft.inputs[key];
  const out = { ...doc, inputs, body: draft.body };
  return draft.rawEdited ? withRaw(out, draft.raw) : out;
}

// messageOf: the commit message the publish dialog proposes.
export const messageOf = (doc, t) => t(doc.isNew ? "publish.messageNew" : "publish.messageUpdate", { title: titleOf(doc) });
