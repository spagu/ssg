// The portable auth verifier and this guard are byte-identical copies of
// workers/auth/portable/ (GO-141); never edit them here — copy them again.
// Next to workers/auth (this repository) the verifier's own suites also run
// once more against THIS copy (a resolve hook redirects their import), as
// workers/auth/test/demo-consumer.test.ts does; scaffolded alone, the guard is
// skipped with its reason and the suites are not there to run.
import { existsSync } from "node:fs";
import { registerHooks } from "node:module";
import { guardCopies } from "./auth-copy-guard.ts";

guardCopies(import.meta.url, {
  "../functions/api/republish/_session-verifier.ts": "session-verifier.ts",
  "./auth-copy-guard.ts": "copy-guard.ts",
});

const SUITES = new URL("../../auth/test/", import.meta.url);
if (existsSync(new URL("verifier-config.test.ts", SUITES))) {
  const copy = new URL("../functions/api/republish/_session-verifier.ts", import.meta.url).href;
  registerHooks({
    resolve(specifier, context, next) {
      const fromSuite = context.parentURL?.startsWith(`${SUITES.href}verifier-`);
      return fromSuite && /^\.\.\/portable\/session-verifier(\.ts)?$/.test(specifier) ? { url: copy, shortCircuit: true } : next(specifier, context);
    },
  });
  for (const suite of ["attacks", "keys", "config", "differential"]) await import(new URL(`verifier-${suite}.test.ts`, SUITES).href);
}
