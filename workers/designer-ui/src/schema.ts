// The configuration schema ssg exports (`ssg config schema`, GO-177): every
// key, its type, description, group and class. config-schema.json is its
// generated twin — never edited by hand; regenerate it in the ssg repository
// with UPDATE_CONFIG_SCHEMA=1 go test ./internal/config -run TestSchemaTwinUpToDate.
// The worker keeps no list of settings of its own (README of the round,
// decision 8): what is in the schema is what the panel shows and accepts.
//
// Paths: "feeds[].merchant.brand" — "[]" any list element, "*" any map key.
// Element nodes (items, values) carry no class: they take their owner's,
// which is what clsOf() resolves.

import schemaJson from "./config-schema.json" with { type: "json" };

export type SettingClass = "plain" | "executes" | "secret";
export type SecretFormat = "whole" | "login" | "inline";

export interface SchemaNode {
  path: string;
  key?: string;
  type: "bool" | "int" | "number" | "string" | "enum" | "list" | "map" | "object" | "any";
  enum?: string[];
  default?: unknown;
  description?: string;
  group?: string;
  class?: SettingClass;
  secretFormat?: SecretFormat;
  mcpWritable?: boolean;
  open?: boolean; // an object that accepts keys it does not name
  items?: SchemaNode;
  values?: SchemaNode;
  fields?: SchemaNode[];
  alternate?: SchemaNode; // a second accepted form ("story: true")
}

export interface SchemaGroup { id: string; title: string; description: string }
export interface Schema { schemaVersion: number; overlayFile: string; groups: SchemaGroup[]; keys: SchemaNode[] }

export const SCHEMA = schemaJson as Schema;
// ROOT: the configuration as one object node, so a path walk has one start.
export const ROOT: SchemaNode = { path: "", type: "object", class: "plain", fields: SCHEMA.keys };

// Resolved per node: the effective class and the secret format of the secret
// key owning it (Go's checkObject passes the format down the same way).
const classes = new WeakMap<SchemaNode, SettingClass>();
const secrets = new WeakMap<SchemaNode, SecretFormat>();

function index(n: SchemaNode, cls: SettingClass, secret: SecretFormat | undefined): void {
  const own = n.class ?? cls;
  const fmt = n.class === "secret" ? n.secretFormat : secret;
  classes.set(n, own);
  if (fmt) secrets.set(n, fmt);
  for (const f of n.fields ?? []) index(f, own, fmt);
  for (const c of [n.items, n.values, n.alternate]) if (c) index(c, own, fmt);
}
index(ROOT, "plain", undefined);

// clsOf: the node's class, its owner's when it has none of its own.
export const clsOf = (n: SchemaNode): SettingClass => classes.get(n)!;
// secretFormatOf: the format when the node is (inside) a secret key, else undefined.
export const secretFormatOf = (n: SchemaNode): SecretFormat | undefined => secrets.get(n);

// open keys of an open object are not described: they take the object's class.
const openChildren = new Map<string, SchemaNode>();
function openChild(n: SchemaNode, key: string): SchemaNode {
  const path = `${n.path}.${key}`;
  let c = openChildren.get(path);
  if (!c) {
    c = { path, key, type: "any" };
    index(c, clsOf(n), secretFormatOf(n));
    openChildren.set(path, c);
  }
  return c;
}

// child: the node describing `key` inside an object or map node, or null
// (an unknown key of a closed object, or a node holding no keys).
export function child(n: SchemaNode, key: string): SchemaNode | null {
  if (n.type === "map") return n.values ?? null;
  if (n.type !== "object") return null;
  return n.fields?.find((f) => f.key === key) ?? (n.open ? openChild(n, key) : null);
}

// namedNodes: every key the overlay can address by name — the root's keys
// and object fields below them, never list elements or map entries (those
// are part of their owner's value). The settings tab lists exactly these.
export function namedNodes(keys: SchemaNode[] = SCHEMA.keys): { node: SchemaNode; segments: string[] }[] {
  const out: { node: SchemaNode; segments: string[] }[] = [];
  const walk = (fields: SchemaNode[] | undefined, at: string[]) => {
    for (const f of fields ?? []) {
      const segments = [...at, f.key!];
      out.push({ node: f, segments });
      if (f.type === "object") walk(f.fields, segments);
    }
  };
  walk(keys, []);
  return out;
}

// fits: does the value have the node's kind (a list, a mapping, a scalar)?
function fits(n: SchemaNode, v: unknown): boolean {
  if (n.type === "any") return true;
  if (Array.isArray(v)) return n.type === "list";
  if (typeof v === "object" && v !== null) return n.type === "object" || n.type === "map";
  return n.type !== "list" && n.type !== "object" && n.type !== "map";
}

// shapeOf: the first of `nodes` whose kind the value has (the primary form or
// its alternate: `story: true`, `languages: [{code: en}]`), else the last one.
export function shapeOf(v: unknown, nodes: (SchemaNode | undefined)[]): SchemaNode {
  const known = nodes.filter((x): x is SchemaNode => x !== undefined);
  return known.find((x) => fits(x, v)) ?? known[known.length - 1];
}

// elementOf: the node describing an element `e` of the list `n` describes.
export const elementOf = (n: SchemaNode, e: unknown): SchemaNode => shapeOf(e, [n.items, n.alternate?.items, n]);

const RANK: Record<SettingClass, number> = { plain: 0, executes: 1, secret: 2 };
// strongest: the class asking for the most trust among `list`.
export const strongest = (list: Iterable<SettingClass>): SettingClass =>
  [...list].reduce<SettingClass>((a, b) => (RANK[b] > RANK[a] ? b : a), "plain");

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

// classesIn: the classes of every node a value reaches — its own, its
// alternate's, and those of the elements and keys it holds. A list of
// workers holding an auth.token reaches "secret" though `workers` is "executes".
export function classesIn(v: unknown, n: SchemaNode, out = new Set<SettingClass>()): Set<SettingClass> {
  out.add(clsOf(n));
  if (n.alternate) classesIn(v, n.alternate, out);
  if (Array.isArray(v) && n.items) for (const e of v) classesIn(e, n.items, out);
  if (isObject(v)) {
    for (const [k, e] of Object.entries(v)) {
      const c = child(n, k);
      if (c) classesIn(e, c, out);
    }
  }
  return out;
}
