// functions/api/core/ carries byte-identical copies of shared files (a
// scaffolded core lives alone and cannot import from a sibling directory):
//   _module-manifest.ts  ← workers/module-manifest.ts   (the manifest reader)
//   _module-schema.json  ← workers/module.schema.json   (the format it reads)
//   _module-summary-schema.json ← workers/module-summary.schema.json (summaries)
// (_session-verifier.ts is guarded by auth-copies.test.ts.) This file fails when a copy differs from its original, and runs the
// reader's own suites against the COPY (a resolve hook redirects its import).
// In a scaffolded copy without the originals next to it, both are skipped
// with a visible reason.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { registerHooks } from "node:module";

const COPY = new URL("../functions/api/core/", import.meta.url);
const WORKERS = new URL("../../", import.meta.url);
const COPIES: [string, URL][] = [
  ["_module-manifest.ts", new URL("module-manifest.ts", WORKERS)],
  ["_module-schema.json", new URL("module.schema.json", WORKERS)],
  ["_module-summary-schema.json", new URL("module-summary.schema.json", WORKERS)],
];

for (const [copy, original] of COPIES) {
  const skip = existsSync(original) ? false : `${original.pathname} is not next to this copy (scaffolded alone): not compared`;
  test(`${copy} is byte-identical to its original`, { skip }, () => {
    assert.ok(readFileSync(new URL(copy, COPY)).equals(readFileSync(original)), `functions/api/core/${copy} differs from ${original.pathname} — copy it again`);
  });
}

// The reader's own suites (manifest and, since GO-148, summary) against the copy.
const suites = ["module-manifest.test.ts", "module-summary.test.ts"].map((f) => new URL(f, WORKERS)).filter((u) => existsSync(u));
if (suites.length) {
  const parents = new Set(suites.map((u) => u.href));
  registerHooks({
    resolve(specifier, context, next) {
      if (specifier === "./module-manifest.ts" && parents.has(context.parentURL ?? "")) return { url: new URL("_module-manifest.ts", COPY).href, shortCircuit: true };
      return next(specifier, context);
    },
  });
  for (const suite of suites) await import(suite.href);
}
