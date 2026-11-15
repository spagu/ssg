// Lets Node run editor-ui's TypeScript as wrangler bundles it: the modules
// import each other without a file extension ("./lib"), which esbuild resolves
// and Node does not. Register it with --import, from the repository root:
//
//   node --import ./workers/editor-ui/test/resolve.mjs --test 'workers/editor-ui/test/*.test.ts'
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
