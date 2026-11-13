// src/mail/ holds byte-identical copies of the `newsletter` template's portable
// mail modules (a scaffolded newsletter-ui lives alone and cannot import from
// a sibling directory). This file:
//   1. fails when any copy differs from its original, and
//   2. runs the original's own test suites against the COPIES (a resolve hook
//      redirects their imports), so the copies are exercised by the same tests.
// In a scaffolded copy without workers/newsletter next to it, both are skipped
// with a visible reason.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { registerHooks } from "node:module";

const MODULES = ["_transport", "_smtp", "_smtpwire", "_mime"];
const ORIGINAL = new URL("../../newsletter/functions/api/newsletter/", import.meta.url);
const COPY = new URL("../src/mail/", import.meta.url);
const SUITES = new URL("../../newsletter/test/", import.meta.url);
const present = existsSync(ORIGINAL) && existsSync(SUITES);
const skip = present ? false : "workers/newsletter is not next to this copy (scaffolded alone): mail copy identity not checked";

test("src/mail/ is byte-identical to the newsletter template's modules", { skip }, () => {
  for (const m of MODULES) {
    const original = readFileSync(new URL(`${m}.ts`, ORIGINAL));
    const copy = readFileSync(new URL(`${m}.ts`, COPY));
    assert.ok(copy.equals(original), `src/mail/${m}.ts differs from workers/newsletter/functions/api/newsletter/${m}.ts — copy it again`);
  }
});

if (present) {
  // The original suites import "../functions/api/newsletter/_x.ts"; point those
  // at src/mail/. Their fakes and fixtures still come from their own directory.
  const pattern = new RegExp(`^\\.\\./functions/api/newsletter/(${MODULES.join("|")})(\\.ts)?$`);
  registerHooks({
    resolve(specifier, context, next) {
      const m = pattern.exec(specifier);
      if (m && context.parentURL?.startsWith(SUITES.href)) return { url: new URL(`${m[1]}.ts`, COPY).href, shortCircuit: true };
      return next(specifier, context);
    },
  });
  for (const suite of ["mime", "smtp", "smtp-failures", "transport", "idempotency"]) await import(new URL(`${suite}.test.ts`, SUITES).href);
}
