// Lets Node run the worker's TypeScript as Pages bundles it: the functions
// import each other without a file extension ("./_lib"), which esbuild
// resolves and Node does not. Register it with --import:
//
//   node --import ./workers/contact-form/test/resolve.mjs --test 'workers/contact-form/test/*.test.ts'
import { registerHooks } from "node:module";

registerHooks({
  resolve(specifier, context, next) {
    try {
      return next(specifier, context);
    } catch (err) {
      if (specifier.startsWith(".") && !/\.[cm]?[jt]s$/.test(specifier)) return next(specifier + ".ts", context);
      throw err;
    }
  },
});
