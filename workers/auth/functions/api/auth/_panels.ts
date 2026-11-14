// "Your panels": which modules an account may use, and where their panels
// are. A leading underscore keeps this file out of the Pages route table.
//
// The modules and their permissions come from the registry (_registry.ts):
// this worker's own manifest plus AUTH_KNOWN_PERMISSIONS. A panel address is
// known only when an AUTH_KNOWN_PERMISSIONS entry carries one — `"panel":
// "/newsletter-admin/"`, a path on this host including the module's base,
// since a worker module's manifest alone cannot know where it is mounted —
// and `title` ({"en": …}) names the module. Without them the page lists the
// account's permissions by module and says plainly that there is no link. The
// future `core` worker, which reads the build's module registry, supersedes
// this page.

import { MODULE } from "./_module";
import type { Lang } from "./_i18n";
import type { Env } from "./_lib";
import { safeNext } from "./_next";
import { registryOf } from "./_registry";

export interface PanelPermission {
  name: string;
  description: string | null;
}

export interface PanelModule {
  id: string;
  title: string;
  panel: string | null; // a same-host path, or null when unknown
  wildcard: string | null; // "*" or "<module>:*" when the account holds all of it
  permissions: PanelPermission[]; // the known permissions it holds, sorted
}

type Text = Record<string, string>;
interface Extra {
  title: Text | null;
  panel: string | null;
}

const textOf = (v: unknown): Text | null => (v && typeof v === "object" && !Array.isArray(v) ? (v as Text) : typeof v === "string" ? { en: v } : null);
const pick = (t: Text | null, lang: Lang): string | null => (t && typeof t[lang] === "string" && t[lang]) || (t && typeof t.en === "string" && t.en) || null;

// extras reads title and panel per module id; anything unusable is ignored
// (the registry already reports malformed entries).
function extras(env: Env): Map<string, Extra> {
  const out = new Map<string, Extra>([[MODULE.id, { title: MODULE.title, panel: MODULE.panel }]]);
  let list: unknown = env.AUTH_KNOWN_PERMISSIONS;
  try {
    if (typeof list === "string") list = JSON.parse(list);
  } catch {
    list = [];
  }
  for (const m of Array.isArray(list) ? list : []) {
    const e = (m ?? {}) as { id?: unknown; title?: unknown; panel?: unknown };
    if (typeof e.id !== "string" || out.has(e.id)) continue;
    out.set(e.id, { title: textOf(e.title), panel: typeof e.panel === "string" ? e.panel : null });
  }
  return out;
}

// panelsFor groups `perms` (the access token's) by module.
export function panelsFor(env: Env, perms: readonly string[], lang: Lang): { everything: boolean; modules: PanelModule[] } {
  const reg = registryOf(env);
  const extra = extras(env);
  const everything = perms.includes("*");
  const byModule = new Map<string, PanelModule>();
  const moduleOf = (id: string): PanelModule => {
    let m = byModule.get(id);
    if (!m) {
      const x = extra.get(id);
      const panel = x?.panel ? safeNext(x.panel, "") || null : null;
      m = { id, title: pick(x?.title ?? null, lang) ?? id, panel, wildcard: everything ? "*" : null, permissions: [] };
      byModule.set(id, m);
    }
    return m;
  };
  const add = (name: string) => {
    const known = reg.known.get(name);
    const m = moduleOf(known?.module ?? name.slice(0, name.indexOf(":")));
    if (!m.permissions.some((p) => p.name === name)) m.permissions.push({ name, description: pick(known?.description ?? null, lang) });
  };
  if (everything) for (const name of reg.known.keys()) add(name);
  for (const p of perms) {
    if (p === "*") continue;
    if (!p.endsWith(":*")) {
      add(p);
      continue;
    }
    // worker:* covers every known name of that worker, whichever module
    // declares it; with none known, the worker's own name stands for it.
    const covered = [...reg.known.values()].filter((k) => k.name.startsWith(p.slice(0, -1)));
    for (const k of covered) add(k.name);
    for (const id of covered.length ? covered.map((k) => k.module) : [p.slice(0, -2)]) moduleOf(id).wildcard = p;
  }
  const key = (m: PanelModule) => `${m.title}\0${m.id}`;
  const modules = [...byModule.values()].sort((a, b) => key(a).localeCompare(key(b)));
  for (const m of modules) m.permissions.sort((a, b) => a.name.localeCompare(b.name));
  return { everything, modules };
}
