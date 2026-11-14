// The portable session verifier (GO-135) and its copy guard are byte-identical
// copies of workers/auth/portable/; skipped visibly when core stands alone.
// The verifier's own suites in workers/auth/test/ then run against core's COPY
// (a resolve hook redirects their import), so the copy is exercised by the
// same tests as its source.
import { existsSync } from "node:fs";
import { registerHooks } from "node:module";
import { guardCopies } from "./auth-copy-guard.ts";

guardCopies(import.meta.url, {
  "../functions/api/core/_session-verifier.ts": "session-verifier.ts",
  "./auth-copy-guard.ts": "copy-guard.ts",
});

const AUTH_TESTS = new URL("../../auth/test/", import.meta.url);
const COPY = new URL("../functions/api/core/_session-verifier.ts", import.meta.url).href;
const SUITES = ["verifier-attacks", "verifier-config", "verifier-keys", "verifier-differential"].map((n) => new URL(`${n}.test.ts`, AUTH_TESTS));
if (SUITES.every((u) => existsSync(u))) {
  registerHooks({
    resolve(specifier, context, next) {
      if (specifier === "../portable/session-verifier.ts" && context.parentURL?.startsWith(AUTH_TESTS.href)) return { url: COPY, shortCircuit: true };
      return next(specifier, context);
    },
  });
  for (const suite of SUITES) await import(suite.href);
}
