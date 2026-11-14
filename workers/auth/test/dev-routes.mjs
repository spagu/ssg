// Every Pages handler under functions/, as the dev server's route table
// ([method, pattern, module, export]) — so the preview serves each new route
// (GO-138's admin API) without a hand-kept list. Static paths come before
// dynamic ones, as in Pages' own routing.
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL("../functions/", import.meta.url));

function files(dir) {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return files(p);
    return n.startsWith("_") || !n.endsWith(".ts") ? [] : [relative(ROOT, p)];
  });
}

export async function discover() {
  const out = [];
  for (const file of files(ROOT)) {
    const mod = file.replace(/\.ts$/, "");
    const pattern = "/" + mod.replace(/(^|\/)index$/, "").replace(/\[(\w+)\]/g, ":$1");
    for (const name of Object.keys(await import(join(ROOT, file)))) {
      const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
      if (!m) continue;
      out.push([m[1].toUpperCase(), pattern, mod, name]);
      if (mod.endsWith("/index")) out.push([m[1].toUpperCase(), pattern + "/", mod, name]);
    }
  }
  return out.sort((a, b) => Number(a[1].includes(":")) - Number(b[1].includes(":")));
}
