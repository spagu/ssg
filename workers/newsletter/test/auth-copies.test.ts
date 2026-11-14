// The portable `auth` verifier and its copy guard are byte-identical copies of
// workers/auth/portable/ (CONTRACT.md, "The copy guard"); skipped, with the
// reason, when this template was scaffolded alone.
import { existsSync } from "node:fs";
import { registerHooks } from "node:module";
import { guardCopies } from "./auth-copy-guard.ts";

guardCopies(import.meta.url, {
  "../functions/api/newsletter/_session-verifier.ts": "session-verifier.ts",
  "./auth-copy-guard.ts": "copy-guard.ts",
});

// In this repository the verifier's own suites (workers/auth/test/verifier-*)
// run once more against THIS copy — a resolve hook redirects their import, as
// workers/auth/test/demo-consumer.test.ts does — so the copy is exercised in
// full, not only compared. Scaffolded alone, there are no suites to run.
const SUITES = new URL("../../auth/test/", import.meta.url);
if (existsSync(SUITES)) {
  const copy = new URL("../functions/api/newsletter/_session-verifier.ts", import.meta.url).href;
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

test("the guard names a missing or drifted copy", () => {
  const { problems } = compareCopies(import.meta.url, { "./no-such-copy.ts": "session-verifier.ts", "./auth-fixtures.ts": "copy-guard.ts" });
  assert.equal(problems.length, 2);
  assert.match(problems[0], /missing/);
  assert.match(problems[1], /differs/);
  assert.throws(() => assertNoProblems(problems), /missing[\s\S]*differs/);
});
