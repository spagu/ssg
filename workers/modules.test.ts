// Every worker template is a module (GO-143): it carries a module.json that
// passes module.schema.json, and the manifest matches the template's code —
// its id is the directory, its kind is its layout, every path it names is
// really served, every permission its navigation needs is declared. A new
// template without a manifest fails here. Run from the repository root:
//
//   node --test 'workers/*.test.ts'
//
// Pages Functions cannot read module.json at runtime, and the site build copies
// only functions/, so a Pages template that serves its manifest imports a
// generated copy (_module.ts next to its manifest route). This test fails while
// a copy differs from module.json; regenerate the copies with
//
//   UPDATE_MODULE_TWINS=1 node --test 'workers/*.test.ts'
//
// A template whose manifest declares endpoints.summary (GO-148) keeps
// test/summary-sample.ts: run with the template's test/resolve.mjs, it drives
// the summary handler through the template's own test fakes on a seeded
// database and prints the body. This test validates that body against
// module-summary.schema.json and checks that every attention link is a page
// of the module's panel — declaring a summary without serving it fails here.
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { LANGUAGES, readManifest, readSummary } from "./module-manifest.ts";
import type { ModuleManifest, ModuleSummary } from "./module-manifest.ts";

const WORKERS = new URL(".", import.meta.url).pathname;
const schema = JSON.parse(readFileSync(join(WORKERS, "module.schema.json"), "utf8"));
const summarySchema = JSON.parse(readFileSync(join(WORKERS, "module-summary.schema.json"), "utf8"));

const isDir = (p: string) => existsSync(p) && statSync(p).isDirectory();
const isFile = (p: string) => existsSync(p) && statSync(p).isFile();

// A template is a directory with a functions/ tree or its own wrangler.toml —
// the same rule the Go scaffold test uses, so an untracked local leftover
// (a directory holding only node_modules or coverage) is not mistaken for one.
const TEMPLATES = readdirSync(WORKERS)
  .filter((d) => isDir(join(WORKERS, d)) && (isDir(join(WORKERS, d, "functions")) || isFile(join(WORKERS, d, "wrangler.toml"))))
  .sort();

// twinSource is the exact text of a Pages template's _module.ts.
export function twinSource(manifest: unknown): string {
  return (
    "// The module manifest (GO-143) as code. Pages Functions cannot read files at\n" +
    "// runtime and the site build copies only functions/, so the manifest route\n" +
    "// imports this copy of module.json (the template root's). Generated: edit\n" +
    "// module.json, then run UPDATE_MODULE_TWINS=1 node --test 'workers/*.test.ts'\n" +
    "// in the ssg repository, or copy the JSON here by hand — the two must match.\n" +
    "// A leading underscore keeps this file out of the Pages route table.\n" +
    `export const MODULE = ${JSON.stringify(manifest, null, 2)};\n`
  );
}

// pagesRoute finds the Pages Functions file that serves path, following
// Pages' file-based routing: a segment is a file or directory of that name,
// or a [param] one; the last segment may also be a directory's index.ts.
function pagesRoute(dir: string, path: string): string | null {
  let at = join(dir, "functions");
  const segs = path.split("/").filter(Boolean);
  for (const [i, seg] of segs.entries()) {
    const names = isDir(at) ? readdirSync(at) : [];
    const dynamic = names.find((n) => /^\[[^\].]+\]/.test(n));
    if (i === segs.length - 1) {
      for (const f of [`${seg}.ts`, join(seg, "index.ts"), dynamic && !isDir(join(at, dynamic)) ? dynamic : ""]) {
        if (f && isFile(join(at, f))) return join(at, f);
      }
      return null;
    }
    if (names.includes(seg) && isDir(join(at, seg))) at = join(at, seg);
    else if (dynamic && isDir(join(at, dynamic))) at = join(at, dynamic);
    else return null;
  }
  return null;
}

// staticFile: what the static assets (public/) serve for path, if anything.
function staticFile(dir: string, path: string): string | null {
  const file = join(dir, "public", path.endsWith("/") ? path + "index.html" : path);
  return isFile(file) ? file : null;
}

// openapiGets lists the GET paths a standalone Worker documents in its
// openapi.yaml (paths at two spaces, methods at four — the layout its own
// openapi test keeps equal to the route registry).
function openapiGets(dir: string): string[] {
  const file = join(dir, "openapi.yaml");
  if (!isFile(file)) return [];
  const out: string[] = [];
  let path = "";
  for (const line of readFileSync(file, "utf8").split("\n")) {
    const p = /^ {2}(\/\S*):\s*$/.exec(line);
    if (p) path = p[1];
    if (/^ {4}get:\s*$/.test(line)) out.push(path);
  }
  return out;
}

// served tells whether a GET for path (relative to the module's base, any
// #fragment being client-side) is answered by the template's own code.
function served(dir: string, m: ModuleManifest, raw: string): boolean {
  const path = raw.split("#")[0];
  if (staticFile(dir, path)) return true;
  if (m.kind === "worker") return openapiGets(dir).includes(path);
  const route = pagesRoute(dir, path);
  return route !== null && /export const onRequest(Get)?\b/.test(readFileSync(route, "utf8"));
}

// sampleSummary runs a template's summary sample and reads what it printed.
function sampleSummary(id: string, extraEnv: Record<string, string> = {}): ModuleSummary {
  const dir = join(WORKERS, id);
  const sample = join(dir, "test", "summary-sample.ts");
  assert.ok(isFile(sample), `workers/${id} declares endpoints.summary but has no test/summary-sample.ts driving its handler`);
  const hooks = join(dir, "test", "resolve.mjs");
  const env: Record<string, string | undefined> = { ...process.env, ...extraEnv };
  delete env.NODE_TEST_CONTEXT; // a plain script, not a test file reporting to this runner
  const run = spawnSync(process.execPath, [...(isFile(hooks) ? ["--import", hooks] : []), sample], { encoding: "utf8", env });
  assert.equal(run.status, 0, `${sample} failed:\n${run.stderr}`);
  const r = readSummary(JSON.parse(run.stdout), summarySchema);
  assert.ok(r.ok, `workers/${id} summary:\n  ${r.ok ? "" : r.errors.join("\n  ")}`);
  return r.summary;
}

function load(id: string): ModuleManifest {
  const file = join(WORKERS, id, "module.json");
  assert.ok(isFile(file), `workers/${id} has no module.json — every template is a module (docs/WORKER-MODULES.md)`);
  const r = readManifest(JSON.parse(readFileSync(file, "utf8")), schema);
  assert.ok(r.ok, `workers/${id}/module.json:\n  ${r.ok ? "" : r.errors.join("\n  ")}`);
  return r.manifest;
}

test("modules: every template under workers/ is found", () => {
  assert.ok(TEMPLATES.length >= 11, `only ${TEMPLATES.length} templates: ${TEMPLATES.join(", ")}`);
  assert.ok(TEMPLATES.includes("newsletter-ui") && TEMPLATES.includes("comments"));
});

for (const id of TEMPLATES) {
  const dir = join(WORKERS, id);

  test(`modules: ${id} — a valid manifest that matches the template`, () => {
    const m = load(id);
    assert.equal(m.id, id, "id is the template's directory name");
    assert.equal(m.kind, isFile(join(dir, "wrangler.toml")) ? "worker" : "pages-function", "kind follows the layout (own wrangler.toml = worker)");
    for (const p of m.permissions) {
      const namespace = p.name.split(":")[0];
      assert.ok(TEMPLATES.includes(namespace), `${p.name}: the namespace is a module id (its own, or the module whose data it acts on)`);
    }
    if (m.panel !== null) assert.ok(served(dir, m, m.panel), `panel ${m.panel} is not served`);
    for (const n of m.nav) assert.ok(served(dir, m, n.path), `nav path ${n.path} is not served`);
    for (const [name, path] of Object.entries(m.endpoints)) assert.ok(served(dir, m, path), `endpoints.${name} ${path} is not served`);
  });

  test(`modules: ${id} — no text is missing a language or left blank`, () => {
    const m = load(id);
    const texts = [m.title, m.description, m.disable, ...m.nav.map((n) => n.label), ...m.permissions.map((p) => p.description)];
    for (const t of texts) for (const l of LANGUAGES) assert.ok(t[l].trim(), `${l} is blank`);
  });

  test(`modules: ${id} — a declared summary answers the summary contract, linking into its own panel`, (t) => {
    const m = load(id);
    if (!m.endpoints.summary) return t.skip("no summary endpoint");
    const s = sampleSummary(id);
    assert.ok(s.metrics.length + s.attention.length + s.problems.length > 0, "the sample shows something to check");
    for (const a of s.attention) {
      assert.ok(m.panel !== null, "a module without a panel has nothing to link an attention item to");
      assert.ok(served(dir, m, a.href), `attention href ${a.href} is not served by the panel`);
    }
    const texts = [...s.metrics.map((x) => x.label), ...s.attention.map((x) => x.text), ...s.problems.map((x) => x.text), ...(s.series ? [s.series.label] : [])];
    for (const t of texts) for (const l of LANGUAGES) assert.ok(t[l].trim(), `${l} is blank`);
  });

  test(`modules: ${id} — a Pages manifest route serves an exact copy of module.json`, (t) => {
    const m = load(id);
    if (m.kind !== "pages-function" || !m.endpoints.manifest) return t.skip("no Pages manifest route");
    const twin = join(dir, "functions", m.endpoints.manifest.split("/").slice(0, -1).join("/"), "_module.ts");
    const want = twinSource(JSON.parse(readFileSync(join(dir, "module.json"), "utf8")));
    if (process.env.UPDATE_MODULE_TWINS === "1") writeFileSync(twin, want);
    assert.equal(isFile(twin) ? readFileSync(twin, "utf8") : "", want, `${twin} differs from module.json`);
    assert.match(readFileSync(pagesRoute(dir, m.endpoints.manifest)!, "utf8"), /from "\.\.\/_module"/, "the route imports the copy");
  });
}

test("modules: a permission declared by two modules means the same thing in both", () => {
  const seen = new Map<string, { id: string; text: string }>();
  for (const id of TEMPLATES) {
    for (const p of load(id).permissions) {
      const text = JSON.stringify(p.description);
      const first = seen.get(p.name);
      if (first) assert.equal(text, first.text, `${p.name}: ${id} and ${first.id} describe it differently`);
      else seen.set(p.name, { id, text });
    }
  }
});

// The two templates share one database: the numbers both show must agree.
test("modules: newsletter and newsletter-ui summaries agree on one database", () => {
  const dir = mkdtempSync(join(tmpdir(), "ssg-summary-"));
  try {
    const db = join(dir, "d1.sqlite");
    const ui = sampleSummary("newsletter-ui", { SUMMARY_DB: db }); // seeds the file
    const nl = sampleSummary("newsletter", { SUMMARY_DB: db }); // reads it
    const shared = nl.metrics.filter((m) => ui.metrics.some((u) => u.key === m.key));
    assert.deepEqual(shared.map((m) => m.key), ["confirmed", "pending"]);
    for (const m of shared) assert.deepEqual(ui.metrics.find((u) => u.key === m.key), m, `${m.key}: same value, same label`);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("pagesRoute: plain, index and [param] routes; a non-route is null", () => {
  const dp = join(WORKERS, "dynamic-price");
  assert.match(pagesRoute(dp, "/api/price/abc")!, /\[sku\]\.ts$/);
  assert.equal(pagesRoute(dp, "/api/nothing/here"), null);
  assert.equal(pagesRoute(dp, "/api/price"), null);
  assert.match(pagesRoute(join(WORKERS, "comments"), "/api/comments/module")!, /module\/index\.ts$/);
  assert.equal(openapiGets(dp).length, 0);
});
