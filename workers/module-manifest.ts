// The module manifest reader (GO-143): checks a parsed module.json against the
// JSON Schema in module.schema.json, then the rules a schema cannot express;
// readSummary does the same for a module's summary against
// module-summary.schema.json (GO-148). No dependencies, so a worker (the planned `core`) can carry a copy of it.
//
// The schema is the single definition of the format; this file interprets the
// small subset of JSON Schema it uses and throws on any other keyword, so a
// schema edit that needs a richer validator fails loudly instead of being
// silently ignored.

// MANIFEST_CONTRACT is the newest manifest contract this reader understands.
// A manifest with a higher `contract` is refused before anything else is
// checked: its fields may mean something this reader does not know.
export const MANIFEST_CONTRACT = 1;

// LANGUAGES every text in a manifest carries — the panels' five languages.
export const LANGUAGES = ["en", "pl", "hi", "de", "fr"] as const;

export type Text = Record<(typeof LANGUAGES)[number], string> & Record<string, string>;
export interface NavItem {
  label: Text;
  path: string;
  permission: string;
}
export interface Permission {
  name: string;
  description: Text;
}
export interface ModuleManifest {
  id: string;
  contract: number;
  version: string;
  kind: "pages-function" | "worker";
  title: Text;
  description: Text;
  panel: string | null;
  nav: NavItem[];
  permissions: Permission[];
  endpoints: Record<string, string>;
  disable: Text;
}

export type Schema = Record<string, unknown>;
export type Result = { ok: true; manifest: ModuleManifest } | { ok: false; errors: string[] };

// Keywords that only document the schema and never reject a value.
const ANNOTATIONS = new Set(["$schema", "$id", "$comment", "$defs", "title", "description"]);

const typeOf = (v: unknown): string => {
  if (v === null) return "null";
  if (Array.isArray(v)) return "array";
  if (typeof v === "number") return Number.isInteger(v) ? "integer" : "number";
  return typeof v;
};
const typeMatches = (want: string, got: string): boolean => want === got || (want === "number" && got === "integer");
const isObject = (v: unknown): v is Record<string, unknown> => typeOf(v) === "object";

// validateSchema returns every place where value breaks schema, as
// "<path>: <problem>" lines; an empty list means it conforms. root resolves
// local "#/$defs/…" references.
export function validateSchema(value: unknown, schema: Schema, root: Schema = schema, at = "manifest"): string[] {
  const errors: string[] = [];
  for (const [key, rule] of Object.entries(schema)) {
    if (ANNOTATIONS.has(key)) continue;
    const check = KEYWORDS[key];
    if (!check) throw new Error(`module.schema.json: unsupported keyword "${key}" at ${at}`);
    errors.push(...check(value, rule, schema, root, at));
  }
  return errors;
}

type Check = (value: unknown, rule: unknown, schema: Schema, root: Schema, at: string) => string[];

const KEYWORDS: Record<string, Check> = {
  $ref(value, rule, _s, root, at) {
    const ref = String(rule);
    if (!ref.startsWith("#/$defs/")) throw new Error(`module.schema.json: only local #/$defs references are supported (${ref})`);
    const target = (root.$defs as Record<string, Schema> | undefined)?.[ref.slice(8)];
    if (!target) throw new Error(`module.schema.json: ${ref} is not defined`);
    return validateSchema(value, target, root, at);
  },
  type(value, rule, _s, _r, at) {
    const wanted = Array.isArray(rule) ? (rule as string[]) : [String(rule)];
    const got = typeOf(value);
    return wanted.some((w) => typeMatches(w, got)) ? [] : [`${at}: must be ${wanted.join(" or ")}, not ${got}`];
  },
  enum(value, rule, _s, _r, at) {
    return (rule as unknown[]).includes(value) ? [] : [`${at}: must be one of ${(rule as unknown[]).map((v) => JSON.stringify(v)).join(", ")}`];
  },
  required(value, rule, _s, _r, at) {
    if (!isObject(value)) return [];
    return (rule as string[]).filter((k) => !(k in value)).map((k) => `${at}.${k}: is required`);
  },
  properties(value, rule, _s, root, at) {
    if (!isObject(value)) return [];
    const props = rule as Record<string, Schema>;
    return Object.keys(props).filter((k) => k in value).flatMap((k) => validateSchema(value[k], props[k], root, `${at}.${k}`));
  },
  additionalProperties(value, rule, schema, root, at) {
    if (!isObject(value)) return [];
    const known = (schema.properties ?? {}) as Record<string, unknown>;
    const extra = Object.keys(value).filter((k) => !(k in known));
    if (rule === false) return extra.map((k) => `${at}.${k}: is not allowed`);
    if (!isObject(rule)) return [];
    return extra.flatMap((k) => validateSchema(value[k], rule, root, `${at}.${k}`));
  },
  items(value, rule, _s, root, at) {
    if (!Array.isArray(value)) return [];
    return value.flatMap((v, i) => validateSchema(v, rule as Schema, root, `${at}[${i}]`));
  },
  maxItems: (value, rule, _s, _r, at) => (Array.isArray(value) && value.length > Number(rule) ? [`${at}: at most ${rule} entries`] : []),
  minLength: (value, rule, _s, _r, at) => (typeof value === "string" && [...value].length < Number(rule) ? [`${at}: must not be empty`] : []),
  maxLength: (value, rule, _s, _r, at) => (typeof value === "string" && [...value].length > Number(rule) ? [`${at}: longer than ${rule} characters`] : []),
  pattern: (value, rule, _s, _r, at) => (typeof value === "string" && !new RegExp(String(rule)).test(value) ? [`${at}: ${JSON.stringify(value)} does not match ${rule}`] : []),
  minimum: (value, rule, _s, _r, at) => (typeof value === "number" && value < Number(rule) ? [`${at}: must be at least ${rule}`] : []),
};

// readManifest checks a parsed module.json: the contract first (a newer one
// is refused with a message that says what to do), then the schema, then the
// rules that tie fields together. Unknown fields are ignored, never refused.
export function readManifest(value: unknown, schema: Schema): Result {
  const contract = isObject(value) ? value.contract : undefined;
  if (Number.isInteger(contract) && (contract as number) > MANIFEST_CONTRACT) {
    const id = isObject(value) && typeof value.id === "string" ? `module "${value.id}"` : "this module";
    return {
      ok: false,
      errors: [`${id} uses manifest contract ${contract}, but this reader understands contract ${MANIFEST_CONTRACT} at most — upgrade ssg (and the core worker) to read it`],
    };
  }
  const errors = validateSchema(value, schema);
  if (errors.length) return { ok: false, errors };
  const m = value as ModuleManifest;
  errors.push(...crossChecks(m));
  return errors.length ? { ok: false, errors } : { ok: true, manifest: m };
}

// crossChecks: what a JSON Schema cannot say about a valid-shaped manifest.
function crossChecks(m: ModuleManifest): string[] {
  const errors: string[] = [];
  const declared = m.permissions.map((p) => p.name);
  for (const name of new Set(declared.filter((n, i) => declared.indexOf(n) !== i))) {
    errors.push(`manifest.permissions: "${name}" is declared twice`);
  }
  const paths = m.nav.map((n) => n.path);
  for (const path of new Set(paths.filter((p, i) => paths.indexOf(p) !== i))) {
    errors.push(`manifest.nav: path "${path}" appears twice`);
  }
  m.nav.forEach((n, i) => {
    if (!declared.includes(n.permission)) errors.push(`manifest.nav[${i}].permission: "${n.permission}" is not declared in permissions`);
  });
  if (m.nav.length && m.panel === null) errors.push("manifest.nav: a module without a panel has no navigation entries");
  return errors;
}

// The module summary (GO-148): what GET …/api/module/summary answers, defined
// by module-summary.schema.json. The same reader, a second schema.
export const SUMMARY_CONTRACT = 1;
// SUMMARY_MAX_BYTES caps a whole answer (UTF-8 JSON): the schema caps every
// list and text, this caps what unknown fields could add.
export const SUMMARY_MAX_BYTES = 32768;

export interface ModuleSummary {
  contract: number;
  generatedAt: string;
  metrics: { key: string; label: Text; value: number; unit?: "percent" | "days" }[];
  attention: { severity: "action" | "warning" | "info"; text: Text; href: string }[];
  problems: { code: string; text: Text }[];
  series?: { label: Text; points: { day: string; value: number }[] };
}
export type SummaryResult = { ok: true; summary: ModuleSummary } | { ok: false; errors: string[] };

// readSummary checks a parsed summary like readManifest checks a manifest:
// contract first, then size, schema, and what the schema cannot say.
export function readSummary(value: unknown, schema: Schema): SummaryResult {
  const contract = isObject(value) ? value.contract : undefined;
  if (Number.isInteger(contract) && (contract as number) > SUMMARY_CONTRACT) {
    return { ok: false, errors: [`this summary uses contract ${contract}, but this reader understands contract ${SUMMARY_CONTRACT} at most — upgrade ssg (and the core worker) to read it`] };
  }
  const bytes = new TextEncoder().encode(JSON.stringify(value) ?? "").length;
  if (bytes > SUMMARY_MAX_BYTES) return { ok: false, errors: [`summary: ${bytes} bytes, more than ${SUMMARY_MAX_BYTES}`] };
  const errors = validateSchema(value, schema, schema, "summary");
  if (errors.length) return { ok: false, errors };
  const s = value as ModuleSummary;
  const keys = s.metrics.map((m) => m.key);
  for (const key of new Set(keys.filter((k, i) => keys.indexOf(k) !== i))) errors.push(`summary.metrics: key "${key}" appears twice`);
  if (Number.isNaN(Date.parse(s.generatedAt))) errors.push(`summary.generatedAt: "${s.generatedAt}" is not a date`);
  const days = s.series?.points.map((p) => p.day) ?? [];
  if (days.some((d, i) => i > 0 && d <= days[i - 1])) errors.push("summary.series.points: days must be in ascending order, each once");
  return errors.length ? { ok: false, errors } : { ok: true, summary: s };
}
