#!/usr/bin/env node
// Prints the AUTH_KNOWN_PERMISSIONS value for the auth worker from the
// module.json files of the workers it should know (GO-133):
//
//   node workers/auth/scripts/auth-permissions.mjs workers/newsletter-ui/module.json workers/comments/module.json
//
// English descriptions only by default: Cloudflare caps one variable at 5 KB,
// and five languages of every description do not fit. --all-languages keeps
// them all (for a wrangler.toml [vars] entry you check yourself).
//
// Each module's title and panel go along too, so "Your panels" (/auth/) can
// name and link it. A manifest's panel path is relative to the module's base:
// the site root for a Pages template, so it is used as is; a standalone Worker
// (newsletter-ui) lives under a base path of its own — pass it as
// --base <id>=<path> (e.g. --base newsletter-ui=/newsletter-admin), or its
// panel is left out with a note.
import { readFileSync } from "node:fs";

const args = process.argv.slice(2);
const all = args.includes("--all-languages");
const bases = new Map();
const files = [];
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--all-languages") continue;
  if (args[i] === "--base") {
    const [id, base] = String(args[++i] ?? "").split("=");
    if (!id || !/^(\/[^/\\?#\s]+)+\/?$/.test(base ?? "") || base.includes("..")) {
      console.error(`--base needs <id>=<path on this host>, like newsletter-ui=/newsletter-admin`);
      process.exit(2);
    }
    bases.set(id, base.replace(/\/$/, ""));
    continue;
  }
  files.push(args[i]);
}
if (!files.length) {
  console.error("usage: auth-permissions.mjs [--all-languages] [--base <id>=<path>]... <module.json>...");
  process.exit(2);
}
const text = (t) => (all ? t : { en: t.en });
const panelOf = (m) => {
  if (typeof m.panel !== "string") return undefined;
  if (m.kind !== "worker") return m.panel;
  if (bases.has(m.id)) return bases.get(m.id) + m.panel;
  console.error(`${m.id} is a standalone Worker: its panel is left out — pass --base ${m.id}=<its base path> to link it`);
  return undefined;
};
const modules = files.map((f) => {
  const m = JSON.parse(readFileSync(f, "utf8"));
  const panel = panelOf(m);
  return {
    id: m.id,
    ...(m.title ? { title: text(m.title) } : {}),
    ...(panel ? { panel } : {}),
    permissions: m.permissions.map((p) => ({ name: p.name, description: text(p.description) })),
  };
});
const value = JSON.stringify(modules);
console.log(`AUTH_KNOWN_PERMISSIONS=${value}`);
console.error(`${value.length} bytes${value.length > 5000 ? " — over Cloudflare's 5 KB per variable; drop --all-languages or list fewer modules" : ""}`);
