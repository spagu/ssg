// Lets Node run the worker's TypeScript as Pages bundles it: the functions
// import each other without a file extension ("./_lib"), which esbuild
// resolves and Node does not. Register it with --import, from the repository
// root:
//
//   node --import ./workers/core/test/resolve.mjs --test 'workers/core/test/*.test.ts'
//
// Types are imported with `import type` throughout, so nothing else needs
// rewriting. The registry import ("../../_ssg-modules.json") is not found in
// the template directory, by design: the worker answers registry_missing.
import { registerHooks } from "node:module";

registerHooks({
  resolve(specifier, context, next) {
    try {
      return next(specifier, context);
    } catch (err) {
      if (specifier.startsWith(".") && !/\.(?:[cm]?[jt]s|json)$/.test(specifier)) return next(specifier + ".ts", context);
      throw err;
    }
  },
});
