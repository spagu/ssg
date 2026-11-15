// Helpers of the settings and theme suites: publishing through the whole
// pipeline, what a commit changed in the fake repository, a valid sample
// value for any schema node, and the scan that no answer leaks a secret.
import assert from "node:assert/strict";
import { ADVANCED, LITERAL, READ, call } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import type { SchemaNode } from "../src/schema.ts";
import { secretFormatOf } from "../src/schema.ts";

// SEEN: the text of every answer the suites received — scanned for secrets.
export const SEEN: string[] = [];
export const OVERLAY_LITERAL = "ov3rlay-literal-SECRET";

export async function get(gh: FakeGitHub, path: string, perms = READ) {
  const r = await call(gh, path, perms);
  SEEN.push(r.text);
  return r;
}

// publish posts settings changes with the overlay version read just before
// (or `base` when given).
export async function publish(gh: FakeGitHub, perms: string[], changes: unknown, base?: string | null, message: unknown = "Change settings") {
  const baseBlobId = base === undefined ? (await get(gh, "/api/settings")).body.overlayBlobId : base;
  const r = await call(gh, "/api/settings/publish", perms, { json: { message, baseBlobId, changes } });
  SEEN.push(r.text);
  return r;
}

// changedFiles: the paths the branch's last commit changed.
export function changedFiles(gh: FakeGitHub): string[] {
  const s = gh.store;
  const head = s.refs.get("main")!;
  const parent = s.commits.get(head)!.parents[0];
  const now = s.flatAt(head);
  const before = parent ? s.flatAt(parent) : new Map();
  const out = new Set<string>();
  for (const [p, e] of now) if (before.get(p)?.sha !== e.sha) out.add(p);
  for (const p of before.keys()) if (!now.has(p)) out.add(p);
  return [...out].sort();
}

// fileAt: a file's text at the tip of main (null when absent).
export function fileAt(gh: FakeGitHub, path: string): string | null {
  const sha = gh.store.blobIdAt("main", path);
  return sha ? gh.store.text(sha) : null;
}

// sample: a value the node accepts — a reference for a secret, the first
// real enum value, one element for a list or map, the first field for an object.
export function sample(n: SchemaNode): unknown {
  const format = secretFormatOf(n);
  if (n.type === "string" && format) return format === "login" ? "admin:$SAMPLE_SECRET" : format === "inline" ? "Bearer $SAMPLE_SECRET" : "$SAMPLE_SECRET";
  switch (n.type) {
    case "bool": return true;
    case "int": return 7;
    case "number": return 1.5;
    case "enum": return n.enum!.find((e) => e !== "") ?? "";
    case "list": return [sample(n.items!)];
    case "map": return { k: sample(n.values!) };
    case "object": return n.fields?.length ? { [n.fields[0].key!]: sample(n.fields[0]) } : {};
    default: return "sample";
  }
}

// addressOf: for a schema path ("ai.models.*.url", "workers[].auth.token"),
// the keys a change can name (up to the first list) and the value to write
// there so that the node is reached with `leaf`.
export function addressOf(path: string, leaf: unknown): { keys: string[]; value: unknown } {
  const steps: ({ key: string } | { list: true })[] = [];
  for (const token of path.split(".")) {
    const name = token.replace(/(\[\])+$/, "");
    steps.push({ key: name === "*" ? "k" : name });
    for (let i = 0; i < (token.length - name.length) / 2; i++) steps.push({ list: true });
  }
  const first = steps.findIndex((s) => "list" in s);
  const cut = first < 0 ? steps.length : first;
  let value = leaf;
  for (let i = steps.length - 1; i >= cut; i--) {
    const s = steps[i];
    value = "list" in s ? [value] : { [s.key]: value };
  }
  return { keys: steps.slice(0, cut).map((s) => (s as { key: string }).key), value };
}

// assertNoSecret: no answer seen so far holds a secret's literal value.
export function assertNoSecret(): void {
  assert.ok(SEEN.length > 0);
  for (const text of SEEN) {
    assert.ok(!text.includes(LITERAL), "the .ssg.yaml literal leaked");
    assert.ok(!text.includes(OVERLAY_LITERAL), "the overlay literal leaked");
  }
}

export { ADVANCED };
