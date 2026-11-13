// Lets Node run the worker's TypeScript as Pages bundles it: the functions
// import each other without a file extension ("./_lib"), which esbuild
// resolves and Node does not. It also maps "cloudflare:sockets" to a stand-in.
// Register it with --import:
//
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { registerHooks } from "node:module";

registerHooks({
  resolve(specifier, context, next) {
    // The Workers sockets API does not exist in Node: sockets.mjs stands in.
    if (specifier === "cloudflare:sockets") return { url: new URL("./sockets.mjs", import.meta.url).href, shortCircuit: true };
    try {
      return next(specifier, context);
    } catch (err) {
      if (specifier.startsWith(".") && !/\.[cm]?[jt]s$/.test(specifier)) return next(specifier + ".ts", context);
      throw err;
    }
  },
});
