// The portable `auth` verifier and its copy guard are byte-identical copies of
// workers/auth/portable/ (CONTRACT.md, "The copy guard"); skipped, with the
// reason, when this template was scaffolded alone.
import { existsSync } from "node:fs";
import { registerHooks } from "node:module";
import { guardCopies } from "./auth-copy-guard.ts";

guardCopies(import.meta.url, {
  "../src/session-verifier.ts": "session-verifier.ts",
  "./auth-copy-guard.ts": "copy-guard.ts",
});

// In this repository the verifier's own suites (workers/auth/test/verifier-*)
// run once more against THIS copy — a resolve hook redirects their import, as
// workers/auth/test/demo-consumer.test.ts does — so the copy is exercised in
// full, not only compared. Scaffolded alone, there are no suites to run.
const SUITES = new URL("../../auth/test/", import.meta.url);
if (existsSync(SUITES)) {
  const copy = new URL("../src/session-verifier.ts", import.meta.url).href;
  registerHooks({
    resolve(specifier, context, next) {
      const fromSuite = context.parentURL?.startsWith(`${SUITES.href}verifier-`);
      return fromSuite && /^\.\.\/portable\/session-verifier(\.ts)?$/.test(specifier) ? { url: copy, shortCircuit: true } : next(specifier, context);
    },
  });
  for (const suite of ["attacks", "keys", "config", "differential"]) await import(new URL(`verifier-${suite}.test.ts`, SUITES).href);
}

import { test } from "node:test";
import assert from "node:assert/strict";
import { assertNoProblems, compareCopies } from "./auth-copy-guard.ts";

test("the guard skips, saying why, when workers/auth is not next to the template", () => {
  const alone = compareCopies("file:///nowhere/template/test/x.test.ts", { "./x.ts": "session-verifier.ts" });
  assert.match(String(alone.skip), /scaffolded alone/);
  assert.deepEqual(alone.problems, []);
});

// Needs workers/auth next to the template: scaffolded alone the guard skips
// (the test above), so there is nothing to name (#423).
const AUTH_SOURCES = new URL("../../auth/portable/", import.meta.url);
test("the guard names a missing or drifted copy", { skip: !existsSync(AUTH_SOURCES) && "workers/auth is not next to this template (scaffolded alone)" }, () => {
  const { problems } = compareCopies(import.meta.url, { "./no-such-copy.ts": "session-verifier.ts", "./fixtures.ts": "copy-guard.ts" });
  assert.equal(problems.length, 2);
  assert.match(problems[0], /missing/);
  assert.match(problems[1], /differs/);
  assert.throws(() => assertNoProblems(problems), /missing[\s\S]*differs/);
});

// The summary kit is a copy too (docs/WORKER-MODULES.md, "Adding a summary to
// your module"): identical to workers/module-summary-kit.ts, or a skip with
// the reason when this template was scaffolded alone.
import { readFileSync } from "node:fs";
const KIT = new URL("../../module-summary-kit.ts", import.meta.url);
test("src/summary-kit.ts is byte-identical to workers/module-summary-kit.ts", { skip: !existsSync(KIT) && "workers/module-summary-kit.ts is not next to this template (scaffolded alone)" }, () => {
  assert.ok(readFileSync(new URL("../src/summary-kit.ts", import.meta.url)).equals(readFileSync(KIT)), "copy workers/module-summary-kit.ts again, never edit the copy");
});
