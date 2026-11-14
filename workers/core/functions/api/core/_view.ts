// Turning registry entries and manifests into what a caller is shown: in
// their language, with links on this host, filtered by their permissions. A
// leading underscore keeps this file out of the Pages route table.
//
// HIDING IS COSMETIC. A module core does not list for a caller is still
// served by that module, which checks access itself — core is not a security
// boundary, it never proxies a request and never authorizes on a module's
// behalf. Hiding only keeps a navigation free of doors the caller cannot open.

import { can } from "./_session-verifier";
import { checkManifest } from "./_manifests";
import type { ModuleManifest } from "./_manifests";
import type { RegistryModule } from "./_registry";
import { joinPath } from "./_target";

export const DEFAULT_LANG = "en";
const LANG = /^[a-z]{2,3}$/;

// pickLang: ?lang= first, then the first usable Accept-Language entry, else
// English. Only the primary subtag counts ("pt-BR" → "pt").
export function pickLang(request: Request): string {
  const url = new URL(request.url);
  const wanted = [url.searchParams.get("lang") ?? "", ...(request.headers.get("accept-language") ?? "").split(",")];
  for (const raw of wanted) {
    const primary = raw.split(";")[0].trim().split("-")[0].toLowerCase();
    if (LANG.test(primary)) return primary;
  }
  return DEFAULT_LANG;
}

// textIn: the text in lang, else English (every manifest text has English).
export const textIn = (text: Record<string, string>, lang: string): string => text[lang] ?? text[DEFAULT_LANG];

// registryManifest reads the manifest the build put in the registry: null when
// there is none, and a problem when there is one the reader refuses.
export function registryManifest(entry: RegistryModule): { manifest: ModuleManifest | null; problem: string | null } {
  if (entry.manifest === null) return { manifest: null, problem: null };
  const checked = checkManifest(entry.manifest, entry.id);
  return checked.ok ? { manifest: checked.manifest, problem: null } : { manifest: null, problem: checked.detail };
}

// visibleTo: perms null (Access, password) sees everything. With permissions
// (the `auth` mode) a module is shown when the caller holds the permission of
// at least one of its navigation entries — or, for a module without
// navigation, any permission it declares. A module whose manifest core does
// not have cannot show that, so it is not shown.
export function visibleTo(manifest: ModuleManifest | null, perms: string[] | null): boolean {
  if (perms === null) return true;
  if (!manifest) return false;
  const needed = manifest.nav.length ? manifest.nav.map((n) => n.permission) : manifest.permissions.map((p) => p.name);
  return needed.some((p) => can(perms, p));
}

export interface NavView {
  label: string;
  path: string; // absolute path on this host
  permission: string;
}
export interface ModuleView {
  id: string;
  kind: string | null;
  title: string;
  description: string | null;
  version: string | null;
  panel: string | null; // absolute path on this host
  nav: NavView[];
  status: string; // "registered" | "live" | "unreachable" | "invalid_manifest"
}

// moduleView: what /api/core/modules shows for one module. Navigation entries
// the caller may not use are dropped, by the same rule as visibleTo; a path
// that does not stay on this host is dropped too (the panel becomes null).
export function moduleView(entry: RegistryModule, manifest: ModuleManifest | null, status: string, lang: string, perms: string[] | null): ModuleView {
  if (!manifest) return { id: entry.id, kind: entry.kind, title: entry.id, description: null, version: entry.version, panel: null, nav: [], status };
  const nav: NavView[] = [];
  for (const n of manifest.nav) {
    const path = joinPath(entry.base, n.path);
    if (path !== null && (perms === null || can(perms, n.permission))) nav.push({ label: textIn(n.label, lang), path, permission: n.permission });
  }
  return {
    id: entry.id,
    kind: manifest.kind,
    title: textIn(manifest.title, lang),
    description: textIn(manifest.description, lang),
    version: manifest.version,
    panel: manifest.panel === null ? null : joinPath(entry.base, manifest.panel),
    nav,
    status,
  };
}
