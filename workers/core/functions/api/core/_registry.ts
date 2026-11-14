// The module registry the site build writes (GO-144): which modules this site
// has, where each lives, and — when the build could read it — each manifest.
// A leading underscore keeps this file out of the Pages route table.
//
// The build writes it to functions/_ssg-modules.json only when `core` is in
// the site's workers: list. The template directory itself never holds it, and
// a project can be deployed without it (scaffolded but never built with ssg,
// built by an ssg without registries, core copied by hand). A static import
// would then fail the whole deploy with a bundler error that says nothing
// useful, so the import is dynamic and caught: wrangler (esbuild) bundles the
// file when it exists and leaves an import that fails at run time when it does
// not, and core answers 503 registry_missing with the fix.

import { problem } from "./_lib";
import { readTheme } from "./_theme";
import type { Theme } from "./_theme";

export const REGISTRY_FORMAT = 1;

export interface RegistryModule {
  id: string;
  kind: string | null;
  base: string;
  bundled: boolean;
  version: string | null;
  manifest: unknown; // the full module.json, or null when the build had none
}
// A template the ssg binary that built the site ships, which the site does
// not use (GO-149's "available, not connected"): from the build, no network.
export interface AvailableModule {
  id: string;
  kind: string | null;
  title: string; // English, as the template's manifest has it
}
export interface Registry {
  format: number;
  modules: RegistryModule[];
  available: AvailableModule[];
  // The panels' look (GO-152, _theme.ts), or null for the shipped look.
  theme: Theme | null;
}

export type Loaded = { ok: true; registry: Registry } | { ok: false; response: Response };

const ID = /^[a-z][a-z0-9-]*$/;
const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

let override: { value: unknown } | null = null;

// useRegistry replaces the bundled registry (tests; undefined restores it).
export function useRegistry(value?: unknown): void {
  override = value === undefined ? null : { value };
}

// MISSING: what importRegistry returns when the deploy has no registry.
const MISSING = Symbol("registry missing");

// importRegistry is the one place that names the file. The try/catch must
// stay lexically around the import: that is what makes esbuild (wrangler)
// leave an unresolvable import for run time instead of failing the build.
// Not cached: the runtime resolves a bundled module once per isolate.
async function importRegistry(): Promise<unknown> {
  if (override) return override.value;
  try {
    // @ts-ignore -- the file exists only in a site build's output, never here.
    const mod = (await import("../../_ssg-modules.json", { with: { type: "json" } })) as { default: unknown };
    return mod.default;
  } catch {
    return MISSING;
  }
}

export type Unusable = { error: "registry_invalid" | "registry_unsupported"; detail: string };
const invalid = (detail: string): Unusable => ({ error: "registry_invalid", detail });

// checkRegistry returns the registry, or why it cannot be used. Entries are
// checked for shape only; whether a base stays on this host is decided where
// URLs are built (_target.ts), per module.
export function checkRegistry(raw: unknown): Registry | Unusable {
  if (!isObject(raw) || !Number.isInteger(raw.format)) return invalid("the file is not a module registry");
  if ((raw.format as number) > REGISTRY_FORMAT) {
    return { error: "registry_unsupported", detail: `the registry has format ${raw.format}, this core reads format ${REGISTRY_FORMAT} at most` };
  }
  if (!Array.isArray(raw.modules)) return invalid("modules is not a list");
  for (const [i, m] of raw.modules.entries()) {
    if (!isObject(m) || typeof m.id !== "string" || !ID.test(m.id)) return invalid(`modules[${i}] has no valid id`);
    if (typeof m.base !== "string") return invalid(`modules[${i}] (${m.id}) has no base path`);
  }
  const modules = (raw.modules as Record<string, unknown>[]).map((m) => ({
    id: m.id as string,
    kind: typeof m.kind === "string" ? m.kind : null,
    base: m.base as string,
    bundled: m.bundled === true,
    version: typeof m.version === "string" ? m.version : null,
    manifest: m.manifest ?? null,
  }));
  return { format: raw.format as number, modules, available: readAvailable(raw.available), theme: readTheme(raw.theme) };
}

// readAvailable keeps the entries in the shape the build writes and drops
// anything else: the list is a convenience, never a reason to refuse the
// registry (an older build has none).
function readAvailable(raw: unknown): AvailableModule[] {
  if (!Array.isArray(raw)) return [];
  const out: AvailableModule[] = [];
  for (const a of raw) {
    if (!isObject(a) || typeof a.id !== "string" || !ID.test(a.id) || a.id.length > 64) continue;
    const title = typeof a.title === "string" && a.title.trim() && a.title.length <= 120 ? a.title.trim() : a.id;
    out.push({ id: a.id, kind: a.kind === "pages-function" || a.kind === "worker" ? a.kind : null, title });
  }
  return out;
}

export async function loadRegistry(): Promise<Loaded> {
  const raw = await importRegistry();
  if (raw === MISSING) {
    return {
      ok: false,
      response: problem(
        "registry_missing",
        503,
        "functions/_ssg-modules.json is not in this deploy: list workers/core under workers: in .ssg.yaml, build the site with ssg 1.9.9 or newer and deploy the output",
      ),
    };
  }
  const checked = checkRegistry(raw);
  if ("error" in checked) {
    return { ok: false, response: problem(checked.error, 503, `${checked.detail} — rebuild the site with the ssg release this core worker came with`) };
  }
  return { ok: true, registry: checked };
}
