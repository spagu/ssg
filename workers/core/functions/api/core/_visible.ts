// The modules one caller may see, each as /api/core/modules shows it — shared
// by /modules, /overview and /info so the three never disagree about who
// sees what. A leading underscore keeps this file out of the Pages route table.
//
// Only modules the build could not describe are asked, for their manifest; a
// module that refuses the caller when asked is left out (the caller would only
// find a locked door behind its link). Leaving out is a courtesy, not
// protection — see _view.ts.

import type { Env } from "./_lib";
import type { ModuleManifest } from "./_manifests";
import { MODULE } from "./_module";
import type { Registry, RegistryModule } from "./_registry";
import type { Caller } from "./_signin";
import { askAll } from "./_status";
import { joinPath } from "./_target";
import { moduleView, registryManifest, visibleTo } from "./_view";
import type { ModuleView } from "./_view";

export interface Visible {
  entry: RegistryModule;
  manifest: ModuleManifest | null;
  view: ModuleView;
}

export async function visibleModules(request: Request, env: Env, caller: Caller, lang: string, registry: Registry, started?: number): Promise<Visible[]> {
  const known = registry.modules.map((entry) => ({ entry, ...registryManifest(entry) }));
  const unknown = known.filter((k) => k.manifest === null && k.problem === null);
  const answers = await askAll(request, env, unknown.map((k) => ({ entry: k.entry, manifest: null })), started);

  const out: Visible[] = [];
  for (const k of known) {
    let manifest: ModuleManifest | null = k.manifest;
    let status = k.problem === null ? "registered" : "invalid_manifest";
    const answer = answers.get(k.entry.id);
    if (answer) {
      if (answer.status === "denied") continue;
      manifest = answer.manifest ?? null;
      status = answer.status === "reachable" ? "live" : answer.status;
    }
    if (!visibleTo(manifest, caller.perms)) continue;
    out.push({ entry: k.entry, manifest, view: moduleView(k.entry, manifest, status, lang, caller.perms) });
  }
  return out;
}

// HOME: the admin home page, core's own panel. core is a Pages Function, so
// its static files sit at the site root.
export const HOME: string = MODULE.panel as string;

// AUTH_LOGOUT: the `auth` worker's sign-out route, relative to its base.
export const AUTH_LOGOUT = "/api/auth/logout";

// signOutPath: where a signed-in person signs out — only in the `auth` mode,
// and only on this host: the `auth` module's base from the registry, else the
// path of CORE_AUTH_ISSUER when the issuer is this origin (its default
// https://<host>/api/auth). Otherwise null: no sign-out control is shown
// rather than one that posts somewhere else.
export function signOutPath(caller: Caller, registry: Registry, env: Env, origin: string): string | null {
  if (caller.mode !== "auth") return null;
  const auth = registry.modules.find((m) => m.id === "auth");
  if (auth) return joinPath(auth.base, AUTH_LOGOUT);
  try {
    const issuer = new URL(env.CORE_AUTH_ISSUER as string);
    if (issuer.origin !== origin) return null;
    return joinPath(issuer.pathname, "/logout");
  } catch {
    return null;
  }
}
