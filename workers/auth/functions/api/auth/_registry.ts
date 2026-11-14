// The registry of known permissions: which names exist, which module declares
// each, and what it means. A leading underscore keeps this file out of the
// Pages route table.
//
// Every module declares its permissions in its module.json (GO-143). A Pages
// Function cannot read another template's files at runtime, so the registry is
// fed from two sources today, merged in this order (the first declaration of
// a name wins):
//   1. this worker's own manifest — _module.ts, the generated copy of
//      module.json (auth:admin, auth:invite);
//   2. AUTH_KNOWN_PERMISSIONS — a var holding a JSON array of modules in the
//      manifest's own shape: [{"id": "newsletter-ui", "permissions": [{"name":
//      "newsletter:send", "description": {"en": "…"}}]}]. A permission may also
//      be a bare name, and a description a plain string (English); Cloudflare
//      caps one variable at 5 KB, so full five-language texts may not fit.
//      scripts/auth-permissions.mjs prints the value from module.json files.
// GO-144 will write a module registry at build time; buildRegistry takes the
// same module-shaped entries, so that file becomes a third source here, with
// no change to the format or to the callers.
//
// The registry is advisory. A role may hold a name the registry does not know
// (a worker added later, a private worker): it is allowed, kept, and flagged
// as unknown in the admin answers — never silently dropped.

import { MODULE } from "./_module";
import type { Env } from "./_lib";

export interface KnownPermission {
  name: string;
  module: string;
  description: Record<string, string> | null;
}

export interface Registry {
  known: Map<string, KnownPermission>;
  problems: string[]; // entries that were skipped, for the administrator
}

const DECLARED = /^[a-z][a-z0-9-]*:[a-z][a-z0-9-]*$/; // a declaration never holds a wildcard
const MODULE_ID = /^[a-z][a-z0-9-]*$/;

function text(v: unknown): Record<string, string> | null {
  if (typeof v === "string") return v ? { en: v } : null;
  if (!v || typeof v !== "object" || Array.isArray(v)) return null;
  const out = Object.fromEntries(Object.entries(v).filter(([, s]) => typeof s === "string" && s !== ""));
  return Object.keys(out).length ? (out as Record<string, string>) : null;
}

// buildRegistry merges module-shaped sources; malformed entries are skipped
// and reported in `problems`.
export function buildRegistry(sources: readonly unknown[]): Registry {
  const known = new Map<string, KnownPermission>();
  const problems: string[] = [];
  sources.forEach((src, i) => {
    const m = (src ?? {}) as { id?: unknown; permissions?: unknown };
    if (typeof m.id !== "string" || !MODULE_ID.test(m.id) || !Array.isArray(m.permissions)) {
      problems.push(`entry ${i}: expected {"id": "<module>", "permissions": [...]}`);
      return;
    }
    for (const p of m.permissions) {
      const entry = (typeof p === "string" ? { name: p } : (p ?? {})) as { name?: unknown; description?: unknown };
      const name = entry.name;
      if (typeof name !== "string" || name.length > 64 || !DECLARED.test(name)) {
        problems.push(`${m.id}: ${JSON.stringify(name ?? null)} is not a permission name`);
        continue;
      }
      if (!known.has(name)) known.set(name, { name, module: m.id, description: text(entry.description) });
    }
  });
  return { known, problems };
}

// configured reads AUTH_KNOWN_PERMISSIONS: a JSON string, or the array itself
// when wrangler.toml declares it as a TOML array of tables.
function configured(raw: unknown): { sources: unknown[]; problem: string | null } {
  if (raw === undefined || raw === null || raw === "") return { sources: [], problem: null };
  let v = raw;
  if (typeof raw === "string") {
    try {
      v = JSON.parse(raw);
    } catch {
      return { sources: [], problem: "AUTH_KNOWN_PERMISSIONS is not valid JSON" };
    }
  }
  return Array.isArray(v) ? { sources: v, problem: null } : { sources: [], problem: "AUTH_KNOWN_PERMISSIONS must be a JSON array of modules" };
}

let cache: { raw: unknown; registry: Registry } | null = null;

// registryOf returns the registry for this configuration, built once per
// isolate (and again when the variable changes). Problems are logged once.
export function registryOf(env: Env): Registry {
  const raw: unknown = env.AUTH_KNOWN_PERMISSIONS;
  if (cache && cache.raw === raw) return cache.registry;
  const c = configured(raw);
  const registry = buildRegistry([MODULE, ...c.sources]);
  if (c.problem) registry.problems.unshift(c.problem);
  if (registry.problems.length) console.warn(`auth: known permissions: ${registry.problems.join("; ")}`);
  cache = { raw, registry };
  return registry;
}

// isKnown: `*` always; `worker:*` when the registry knows any permission of
// that worker; a single name when the registry has it.
export function isKnown(reg: Registry, perm: string): boolean {
  if (perm === "*") return true;
  if (perm.endsWith(":*")) return [...reg.known.keys()].some((k) => k.startsWith(perm.slice(0, -1)));
  return reg.known.has(perm);
}

export const unknownOf = (reg: Registry, perms: readonly string[]): string[] => perms.filter((p) => !isKnown(reg, p));
