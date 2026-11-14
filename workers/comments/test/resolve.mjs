// Lets Node run the worker's TypeScript as Pages bundles it. Register it with
// --import, from the repository root:
//
//   node --import ./workers/comments/test/resolve.mjs --test 'workers/comments/test/*.test.ts'
//
// Two differences between esbuild (what Pages uses) and Node's type stripping:
//   - the functions import each other without a file extension ("./_lib");
//   - they import the Env and CommentRow interfaces as values
//     (`import { Env, json } from "./_lib"`). esbuild drops an import that is
//     only a type; Node keeps it and fails, because no such export exists at
//     runtime. The loader removes those two names from import lists of the
//     template's own files — line count unchanged, so coverage lines stay true.
import { registerHooks } from "node:module";

const TYPES = new Set(["Env", "CommentRow"]);
const OWN = new URL("../functions/", import.meta.url).href;

// dropTypeImports rewrites `import { Env, json } from "./_lib"` to
// `import { json } from "./_lib"`, and an import of types only to `import type`.
export function dropTypeImports(source) {
  return source.replace(/import \{([^}]*)\} from ("\.[^"]*");/g, (whole, list, from) => {
    const names = list.split(",").map((n) => n.trim()).filter(Boolean);
    const values = names.filter((n) => !TYPES.has(n));
    if (values.length === names.length) return whole;
    const breaks = "\n".repeat((list.match(/\n/g) || []).length);
    return values.length ? `import { ${values.join(", ")} } from ${from};${breaks}` : `import type { ${names.join(", ")} } from ${from};${breaks}`;
  });
}

registerHooks({
  resolve(specifier, context, next) {
    try {
      return next(specifier, context);
    } catch (err) {
      if (specifier.startsWith(".") && !/\.[cm]?[jt]s$/.test(specifier)) return next(specifier + ".ts", context);
      throw err;
    }
  },
  load(url, context, next) {
    const out = next(url, context);
    if (!url.startsWith(OWN) || !url.endsWith(".ts")) return out;
    return { ...out, source: dropTypeImports(String(out.source)) };
  },
});
