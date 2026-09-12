// Fails when a worker source imports an npm package.
//
// The shop is deployed by Cloudflare Pages Direct Upload, which does not bundle:
// it copies the functions tree and runs it. A bare import therefore fails at
// request time, in production, on a route nobody tested that minute. The Go
// generator warns about this at build time (warnBareImports); this script is
// the same rule with an exit code, so CI refuses the change instead.

import { readdir, readFile } from "node:fs/promises";
import { join, relative } from "node:path";

const ROOT = new URL("..", import.meta.url).pathname;
const SOURCE = join(ROOT, "functions");
const EXTENSIONS = [".ts", ".js", ".mjs", ".tsx"];

/** Specifiers the runtime provides, or that resolve by path. */
const isBare = (spec) =>
  !/^[./]/.test(spec) && !/^(cloudflare|node|https?):/.test(spec);

// The lookbehind keeps a string literal that happens to end in the word
// "from" — a query parameter called from, say — out of the results.
const FROM_RE = /(?<!["'])\bfrom\s*["']([^"']+)["']/g;
const SIDE_EFFECT_RE = /^\s*import\s*["']([^"']+)["']/gm;

async function* sources(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) yield* sources(path);
    else if (EXTENSIONS.some((e) => entry.name.endsWith(e))) yield path;
  }
}

const offences = [];
for await (const path of sources(SOURCE)) {
  const text = await readFile(path, "utf8");
  for (const re of [FROM_RE, SIDE_EFFECT_RE]) {
    re.lastIndex = 0;
    for (const match of text.matchAll(re)) {
      // A type-only import disappears at build time and cannot fail at runtime.
      const typeOnly = /\bimport\s+type\b/.test(
        text.slice(Math.max(0, match.index - 80), match.index + 10),
      );
      if (isBare(match[1]) && !typeOnly) {
        offences.push(`${relative(ROOT, path)}: imports "${match[1]}"`);
      }
    }
  }
}

if (offences.length > 0) {
  console.error("These files import npm packages, which Direct Upload cannot bundle:\n");
  for (const line of [...new Set(offences)]) console.error(`  ${line}`);
  console.error("\nUse the runtime's own APIs, or vendor the code into functions/.");
  process.exit(1);
}
console.log("No bare imports. The functions tree deploys as-is.");
