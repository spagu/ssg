// The two manifest readers agree (GO-144): the site build checks module.json
// in Go (internal/modules), the workers in TypeScript (module-manifest.ts).
// Each fixture in internal/modules/testdata/manifests holds a manifest and the
// sorted errors a reader must report for it (none = valid); this test holds
// the TypeScript reader to them, internal/modules/fixtures_test.go the Go one.
// The fixtures live outside workers/ because every directory here is a
// template. Run from the repository root:
//
//   node --test 'workers/*.test.ts'
//
// After an intended change of a message, rewrite the expected errors with
//
//   UPDATE_MODULE_FIXTURES=1 node --test 'workers/module-fixtures.test.ts'
//
// and run the Go test to see that it agrees.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { readManifest } from "./module-manifest.ts";

const FIXTURES = new URL("../internal/modules/testdata/manifests/", import.meta.url).pathname;
const schema = JSON.parse(readFileSync(new URL("./module.schema.json", import.meta.url), "utf8"));
const files = readdirSync(FIXTURES).filter((f) => f.endsWith(".json")).sort();

test("fixtures: there is a shared set to agree on", () => {
  assert.ok(files.length >= 20, `only ${files.length} fixtures`);
  assert.ok(files.some((f) => f.startsWith("valid-")), "no valid fixture");
});

for (const file of files) {
  test(`fixture ${file}`, () => {
    const path = join(FIXTURES, file);
    const fixture = JSON.parse(readFileSync(path, "utf8"));
    const result = readManifest(fixture.manifest, schema);
    const errors = result.ok ? [] : [...result.errors].sort();
    if (process.env.UPDATE_MODULE_FIXTURES === "1") {
      writeFileSync(path, JSON.stringify({ manifest: fixture.manifest, errors }, null, 2) + "\n");
      return;
    }
    assert.deepEqual(errors, fixture.errors);
    assert.equal(file.startsWith("valid-"), errors.length === 0, "a fixture is named valid-* exactly when it has no errors");
  });
}
