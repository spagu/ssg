// One setting's value as a form, and back. toDraft(node, value) turns a value
// the worker answered (secrets as {set, reference}) into a draft the controls
// edit; fromDraft(draft) turns the draft back into the value a publication
// sends for that key, with every problem found on the way. Pure: the views
// render drafts and call these, the tests run every kind both ways.
//
// A draft is {kind, node, …}; the kind follows the schema node's type:
//   switch bool · number int/number · text string · select enum ·
//   secret a string inside a secret key (secret-ref.js) · json any ·
//   lines a list of scalars, one per line · rows a list of objects ·
//   map a map (a key and a value draft per row) · fields an object.
// A node with a second accepted form (`story: true`, `prefix: "/blog/"`)
// takes the form the value is written in; an unset value takes the first.

import { buildReference, parseReference, secretDraft } from "./secret-ref.js";

const isObject = (v) => typeof v === "object" && v !== null && !Array.isArray(v);
const SCALAR = new Set(["bool", "int", "number", "string", "enum"]);

// fits: does the value have the node's kind (a list, a mapping, a scalar)?
function fits(node, v) {
  if (node.type === "any") return true;
  if (Array.isArray(v)) return node.type === "list";
  if (isObject(v)) return node.type === "object" || node.type === "map";
  return SCALAR.has(node.type);
}

// formNode: the form of `node` the value is written in.
export const formNode = (node, value) => (node.alternate && value !== undefined && !fits(node, value) && fits(node.alternate, value) ? node.alternate : node);

// secretOf: the secret format a node is (inside), given its owner's.
const secretOf = (node, inherited) => (node.class === "secret" ? node.secretFormat ?? "whole" : node.class ? undefined : inherited);

// kindOf: the control for a node (already in its form).
export function kindOf(node, secret) {
  if (secret && node.type === "string") return "secret";
  if (node.type === "bool") return "switch";
  if (node.type === "int" || node.type === "number") return "number";
  if (node.type === "string") return "text";
  if (node.type === "enum") return "select";
  if (node.type === "list") return SCALAR.has(node.items?.type) ? "lines" : node.items?.type === "object" ? "rows" : "json";
  if (node.type === "map") return "map";
  if (node.type === "object") return "fields";
  return "json";
}

// jsonText shows a free-form value: a string as itself unless it would read
// back as something else ("true", "42"), everything else as JSON.
function jsonText(v) {
  if (v === undefined) return "";
  if (typeof v !== "string") return JSON.stringify(v, null, 2);
  try {
    JSON.parse(v);
    return JSON.stringify(v);
  } catch {
    return v;
  }
}

// toDraft: the draft of `value` for `node`; `secret` is the owner's format.
// A field left out shows what leaving it out means: the schema's default.
export function toDraft(node, given, secret) {
  const value = given === undefined ? node.default : given;
  const form = formNode(node, value);
  const fmt = secretOf(form, secretOf(node, secret));
  const kind = kindOf(form, fmt);
  const d = { kind, node: form, fmt };
  switch (kind) {
    case "secret": return { ...secretDraft(fmt, value), node: form };
    // unset: no value and no schema default — ssg decides; never shown as "Off".
    case "switch": return { ...d, on: value === true, unset: value === undefined };
    case "number": return { ...d, text: typeof value === "number" ? String(value) : "" };
    case "text": case "select": return { ...d, text: typeof value === "string" ? value : value === undefined || value === null ? "" : String(value) };
    case "lines": {
      const itemFmt = secretOf(form.items, fmt);
      // A secret element set as something other than a reference shows as
      // "?", which is not one: the list cannot be published until it is replaced.
      const line = (e) => {
        if (!itemFmt) return String(e);
        const s = secretDraft(itemFmt, e);
        return s.keep ? "?" : s.name ? buildReference(itemFmt, s).value : "";
      };
      return { ...d, secret: itemFmt, text: (Array.isArray(value) ? value : []).map(line).join("\n") };
    }
    case "rows": return { ...d, rows: (Array.isArray(value) ? value : []).map((e) => toDraft(form.items, e, fmt)) };
    case "map": return { ...d, rows: Object.entries(isObject(value) ? value : {}).map(([key, v]) => ({ key, d: toDraft(form.values, v, fmt) })) };
    case "fields": {
      const v = isObject(value) ? value : {};
      const known = new Set((form.fields ?? []).map((f) => f.key));
      const extra = Object.fromEntries(Object.entries(v).filter(([k]) => !known.has(k)));
      return { ...d, fields: Object.fromEntries((form.fields ?? []).map((f) => [f.key, toDraft(f, v[f.key], fmt)])), extra };
    }
    default: return { ...d, text: jsonText(value) };
  }
}

// emptyDraft: a new row's draft.
export const emptyDraft = (node, secret) => toDraft(node, undefined, secret);

// Row editing (rows and map drafts), in place — the view re-renders after.
export function addRow(d) {
  if (d.kind === "rows") d.rows.push(emptyDraft(d.node.items, d.fmt));
  else d.rows.push({ key: "", d: emptyDraft(d.node.values, d.fmt) });
  return d;
}
export function removeRow(d, i) {
  d.rows.splice(i, 1);
  return d;
}
// moveRow moves row i by delta (-1 up, +1 down); outside the list: no move.
export function moveRow(d, i, delta) {
  const j = i + delta;
  if (j < 0 || j >= d.rows.length) return d;
  [d.rows[i], d.rows[j]] = [d.rows[j], d.rows[i]];
  return d;
}

// zeroOf: the value a field left out of an object means anyway.
function zeroOf(node) {
  if (node.default !== undefined) return node.default;
  return { bool: false, string: "", enum: "" }[node.type];
}

const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

// fromDraft: {value, errors}. `field` — the draft is a field of an object,
// where a value equal to what leaving it out means is left out (undefined).
// errors: [{at: [key or index…], code}] — at is relative to the draft.
export function fromDraft(d, { field = false } = {}) {
  const errors = [];
  const fail = (code, at = []) => (errors.push({ at, code }), undefined);
  const value = read(d, field, fail);
  const omit = field && (value === undefined || same(value, zeroOf(d.node)) || (Array.isArray(value) && !value.length) || (isObject(value) && !Object.keys(value).length));
  return { value: omit ? undefined : value, errors };
}

function read(d, field, fail) {
  switch (d.kind) {
    case "switch": return d.on;
    case "number": {
      const s = d.text.trim();
      if (!s) return field ? undefined : fail("required");
      const n = Number(s);
      if (!Number.isFinite(n) || !/^-?\d+(\.\d+)?(e[+-]?\d+)?$/i.test(s)) return fail("number");
      return d.node.type === "int" && !Number.isInteger(n) ? fail("integer") : n;
    }
    case "text": case "select": return d.text;
    case "secret": {
      if (field && !d.name && !d.keep) return undefined;
      if (d.keep && !d.name) return fail("secretReEnter");
      const r = buildReference(d.format, d);
      return r.error ? fail(r.error) : r.value;
    }
    case "lines": return lines(d, fail);
    case "rows": return d.rows.map((r, i) => collect(fromDraft(r), fail, [i]));
    case "map": {
      const out = {};
      d.rows.forEach(({ key, d: v }, i) => {
        const k = key.trim();
        if (!k || k === "__proto__") return fail(k ? "badKey" : "emptyKey", [i]);
        if (Object.hasOwn(out, k)) return fail("duplicateKey", [i]);
        out[k] = collect(fromDraft(v), fail, [i]);
      });
      return out;
    }
    case "fields": {
      const out = { ...d.extra };
      for (const [k, f] of Object.entries(d.fields)) {
        const v = collect(fromDraft(f, { field: true }), fail, [k]);
        if (v !== undefined) out[k] = v;
      }
      return out;
    }
    default: {
      const s = d.text.trim();
      if (!s) return field ? undefined : fail("required");
      try {
        return JSON.parse(s);
      } catch {
        return d.text;
      }
    }
  }
}

// collect: a nested result's value, its errors re-rooted under `at`.
function collect(r, fail, at) {
  for (const e of r.errors) fail(e.code, [...at, ...e.at]);
  return r.value;
}

// lines: one element per non-empty line, typed by the list's items.
function lines(d, fail) {
  const items = d.node.items;
  const out = [];
  d.text.split("\n").map((l) => l.trim()).filter(Boolean).forEach((l, i) => {
    if (d.secret) return parseReference(d.secret, l) ? out.push(l) : fail("secretLine", [i]);
    if (items.type === "int" || items.type === "number") {
      const n = Number(l);
      if (!Number.isFinite(n) || (items.type === "int" && !Number.isInteger(n))) return fail(items.type === "int" ? "integer" : "number", [i]);
      return out.push(n);
    }
    if (items.type === "enum" && !items.enum.includes(l)) return fail("enum", [i]);
    if (items.type === "bool") return l === "true" || l === "false" ? out.push(l === "true") : fail("bool", [i]);
    return out.push(l);
  });
  return out;
}
