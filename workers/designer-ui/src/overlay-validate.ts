// Validation of a `.ssg.designer.json` document against the schema — the same
// rules ssg applies when it loads the overlay (internal/config/overlay_validate.go,
// GO-177), so a file this worker writes never stops the site's build: every
// key must exist, every value must have the key's type (or its alternate's),
// an enum value must be listed, null is not a list element, and a secret key
// holds only an environment reference. Problems name the path, never the
// value — a refused value may be a secret pasted in the wrong place.

import { ROOT, child, secretFormatOf } from "./schema";
import type { SchemaNode, SecretFormat } from "./schema";

// ENV_WHOLE: "$NAME" or "${NAME}" as the whole value; ENV_REF: one anywhere.
export const ENV_WHOLE = /^\$(?:\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)$/;
export const ENV_REF = /\$(?:\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)/;

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const join = (path: string, key: string) => (path ? `${path}.${key}` : key);

// secretRefOK: is `s` a reference in the format of its secret key? (Go's
// secretRefOK: inline = any text holding one; login = "login:$NAME".)
export function secretRefOK(format: SecretFormat, s: string): boolean {
  if (format === "inline") return ENV_REF.test(s);
  if (format === "login") {
    const i = s.indexOf(":");
    return i >= 0 && ENV_WHOLE.test(s.slice(i + 1));
  }
  return ENV_WHOLE.test(s);
}

const SECRET_FORM: Record<SecretFormat, string> = {
  whole: '"$NAME" or "${NAME}"',
  login: '"login:$NAME"',
  inline: 'text holding "$NAME", e.g. "Bearer $TOKEN"',
};

// kind names a JSON value's type for a problem (null never gets here: it
// removes a key, and in a list it has its own problem).
function kind(v: unknown): string {
  if (typeof v === "boolean") return "true/false";
  if (typeof v === "number") return "a number";
  if (typeof v === "string") return "text";
  if (Array.isArray(v)) return "a list";
  return "an object";
}

// primary checks v against the node's own type and appends problems.
function primary(n: SchemaNode, v: unknown, path: string, out: string[]): void {
  const bad = (want: string) => {
    const enumText = n.enum?.length ? `one of ${n.enum.map((e) => JSON.stringify(e)).join(", ")}` : want;
    out.push(`${path}: expected ${enumText}${n.alternate ? ` (or ${n.alternate.type})` : ""}, got ${kind(v)}`);
  };
  switch (n.type) {
    case "any":
      return;
    case "bool":
      if (typeof v !== "boolean") bad("true or false");
      return;
    case "int":
    case "number":
      if (typeof v !== "number" || !Number.isFinite(v)) bad("a number");
      else if (n.type === "int" && !Number.isSafeInteger(v)) bad("a whole number");
      return;
    case "string":
    case "enum":
      if (typeof v !== "string") bad("text");
      else stringRules(n, v, path, out);
      return;
    case "list":
      if (!Array.isArray(v)) return bad("a list");
      v.forEach((e, i) => {
        if (e === null) out.push(`${path}[${i}]: null is not a list element`);
        else check(n.items!, e, `${path}[${i}]`, out);
      });
      return;
    case "map":
      if (!isObject(v)) return bad("an object");
      for (const [k, e] of Object.entries(v)) if (e !== null) check(n.values!, e, join(path, k), out);
      return;
    default:
      if (!isObject(v)) return bad("an object");
      object(n, v, path, out);
  }
}

// stringRules: the enum, and the secret rule for a value inside a secret key.
function stringRules(n: SchemaNode, s: string, path: string, out: string[]): void {
  if (n.enum?.length && !n.enum.includes(s)) out.push(`${path}: not one of ${n.enum.map((e) => JSON.stringify(e)).join(", ")}`);
  const format = secretFormatOf(n);
  if (format && s !== "" && !secretRefOK(format, s)) {
    out.push(`${path}: a secret must be written as an environment reference (${SECRET_FORM[format]}), never as a literal value`);
  }
}

// check: the node's type, or its alternate's — only the primary's problems
// are reported when neither fits (as ssg does).
function check(n: SchemaNode, v: unknown, path: string, out: string[]): void {
  const mine: string[] = [];
  primary(n, v, path, mine);
  if (!mine.length) return;
  if (n.alternate) {
    const alt: string[] = [];
    primary(n.alternate, v, path, alt);
    if (!alt.length) return;
  }
  out.push(...mine);
}

// object: every key known (or the object open), null always allowed.
function object(n: SchemaNode, m: Record<string, unknown>, path: string, out: string[]): void {
  for (const [k, e] of Object.entries(m)) {
    const c = child(n, k);
    if (!c) out.push(`${join(path, k)}: unknown configuration key`);
    else if (e !== null) check(c, e, join(path, k), out);
  }
}

// themeName: `template` is one folder inside templates_dir (GO-183; Go's
// checkThemeName). ssg joins templates_dir and template, parses the folder's
// *.html and copies its css/, js/ and images/ into the public site, so a
// separator, "." or ".." would let a plain key point the build at any folder
// of the CI machine (templates_dir itself is "executes").
function themeName(v: unknown, out: string[]): void {
  if (typeof v === "string" && (/[\\/]/.test(v) || v === "." || v === "..")) {
    out.push("template: a theme is one folder name inside templates_dir, never a path");
  }
}

// validateOverlay returns every problem of the document, sorted (ssg sorts
// too, so one fix-and-retry round is enough); empty when it is valid.
export function validateOverlay(doc: unknown): string[] {
  if (!isObject(doc)) return ["the top level must be an object"];
  const out: string[] = [];
  object(ROOT, doc, "", out);
  themeName(doc.template, out);
  return out.sort((a, b) => (a < b ? -1 : 1)); // never equal: each names its own path
}
