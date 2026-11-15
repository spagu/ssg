// Copy guard for templates that carry the portable session verifier (CONTRACT.md, "Make your
// worker a consumer"). A template is scaffolded alone and cannot import from workers/auth, so it
// holds byte-identical copies; this test helper fails when a copy drifts from its source, and
// skips with a visible reason when workers/auth is not next to the template.
//
// Use: copy this file into your template's test/ directory as auth-copy-guard.ts (it guards
// itself too), then add one test file, e.g. test/auth-copies.test.ts:
//
//   import { guardCopies } from "./auth-copy-guard.ts";
//   guardCopies(import.meta.url, {
//     "../functions/api/comments/_session-verifier.ts": "session-verifier.ts",
//     "./auth-copy-guard.ts": "copy-guard.ts",
//   });
//
// Keys are the copies, relative to that test file; values are file names in workers/auth/portable/.
// The test file must sit in workers/<template>/test/ (the source is found at ../../auth/portable/).
import { test } from "node:test";
import { existsSync, readFileSync } from "node:fs";

export const SOURCE_DIR = "../../auth/portable/";

export interface CopyCheck {
  skip: string | false; // the reason the check cannot run, or false
  problems: string[]; // one line per missing or drifted copy
}

// compareCopies reads every pair and reports, without asserting (testable on its own).
export function compareCopies(testUrl: string, pairs: Record<string, string>): CopyCheck {
  const source = new URL(SOURCE_DIR, testUrl);
  if (!existsSync(source)) {
    return { skip: "workers/auth is not next to this template (scaffolded alone): portable verifier copies not checked", problems: [] };
  }
  const problems: string[] = [];
  for (const [copy, name] of Object.entries(pairs)) {
    const copyUrl = new URL(copy, testUrl);
    const original = readFileSync(new URL(name, source));
    if (!existsSync(copyUrl)) problems.push(`${copy} is missing — copy workers/auth/portable/${name} there`);
    else if (!readFileSync(copyUrl).equals(original)) problems.push(`${copy} differs from workers/auth/portable/${name} — copy it again, never edit a copy`);
  }
  return { skip: false, problems };
}

// assertNoProblems throws one error listing every problem.
export function assertNoProblems(problems: string[]): void {
  if (problems.length) throw new Error(problems.join("\n"));
}

// guardCopies registers one node:test test: it fails listing every drifted copy, or is skipped
// with the reason when the source is not there.
export function guardCopies(testUrl: string, pairs: Record<string, string>): void {
  const { skip, problems } = compareCopies(testUrl, pairs);
  test("portable auth copies are byte-identical to workers/auth/portable/", { skip }, () => assertNoProblems(problems));
}
