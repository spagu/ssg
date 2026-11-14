// What the module-management routes share (GO-149): who may use them, the
// registry they act on, and the view GET /api/core/manage answers. A leading
// underscore keeps this file out of the Pages route table.
//
// WHO. In the `auth` mode only an account holding core:manage (declared in
// core's manifest); with a shared password or Cloudflare Access every
// signed-in admin, as with the rest of core. Reading needs the same right as
// writing: the view carries admin notes and who changed what.
//
// WHAT. Visibility, order, labels and a note — never installing, enabling or
// deploying anything. The view says so next to every switch and gives the
// module's own `disable` text: how to really switch it off.

import type { D1Database, Env } from "./_lib";
import { problem } from "./_lib";
import { recentChanges } from "./_changes";
import type { LoggedChange } from "./_changes";
import { latestStatuses, recentHistory } from "./_history";
import type { HistoryRow, Latest } from "./_history";
import { loadRegistry } from "./_registry";
import type { Registry } from "./_registry";
import type { Handler } from "./_route";
import { can, forbidden } from "./_session-verifier";
import { arrange, defaults, readSettings } from "./_settings";
import type { Setting } from "./_settings";
import type { Caller } from "./_signin";
import { moduleView, registryManifest, textIn } from "./_view";

export const MANAGE_PERMISSION = "core:manage";
export const NO_DATABASE =
  "module settings need a D1 database: create one, bind it to core as CORE_DB (wrangler.snippet.toml) and deploy; core creates its tables on first use";

export const mayManage = (caller: Caller): boolean => caller.mode !== "auth" || can(caller.perms ?? [], MANAGE_PERMISSION);

export type ManageHandler = (request: Request, env: Env, caller: Caller, registry: Registry, db: D1Database) => Promise<Response>;
export type ReadHandler = (request: Request, env: Env, caller: Caller, registry: Registry) => Promise<Response>;

async function admitted(caller: Caller): Promise<{ ok: true; registry: Registry } | { ok: false; response: Response }> {
  if (!mayManage(caller)) return { ok: false, response: forbidden(MANAGE_PERMISSION) };
  const loaded = await loadRegistry();
  return loaded.ok ? { ok: true, registry: loaded.registry } : loaded;
}

// reading wraps a GET: the permission, then the registry.
export const reading =
  (handler: ReadHandler): Handler =>
  async (request, env, caller) => {
    const got = await admitted(caller);
    return got.ok ? handler(request, env, caller, got.registry) : got.response;
  };

// writing wraps a change: the permission, the registry, then CORE_DB — a
// change without a database is refused, never kept in memory.
export const writing =
  (handler: ManageHandler): Handler =>
  async (request, env, caller) => {
    const got = await admitted(caller);
    if (!got.ok) return got.response;
    if (!env.CORE_DB) return problem("no_database", 503, NO_DATABASE);
    return handler(request, env, caller, got.registry, env.CORE_DB);
  };

export const knownIds = (registry: Registry): string[] => registry.modules.map((m) => m.id);

export interface ManageItem {
  id: string;
  kind: string | null;
  title: string; // the manifest's, in the asked language (labels are separate)
  panel: string | null;
  hidden: boolean;
  position: number | null;
  labels: Record<string, string>;
  note: string;
  updatedAt: string | null;
  updatedBy: string | null;
  disable: string | null; // how to really switch the module off, from its manifest
  status: Latest | null; // the last observed reachable/unreachable, and since when
}

// manageItems: every module of the registry — hidden ones too — in the order
// the bar shows them.
export function manageItems(registry: Registry, settings: Map<string, Setting>, latest: Map<string, Latest>, lang: string): ManageItem[] {
  const items = registry.modules.map((entry): ManageItem => {
    const { manifest } = registryManifest(entry);
    const view = moduleView(entry, manifest, "registered", lang, null);
    const s = settings.get(entry.id) ?? defaults(entry.id);
    return {
      id: entry.id,
      kind: view.kind,
      title: view.title,
      panel: view.panel,
      hidden: s.hidden,
      position: s.position,
      labels: s.labels,
      note: s.note,
      updatedAt: s.updatedAt,
      updatedBy: s.updatedBy,
      disable: manifest ? textIn(manifest.disable, lang) : null,
      status: latest.get(entry.id) ?? null,
    };
  });
  return arrange(items, (i) => i.id, settings, true);
}

export interface AvailableItem {
  id: string;
  kind: string | null;
  title: string;
  command: string;
  snippet: string; // what to add to .ssg.yaml after scaffolding
}

// availableItems: the templates the build's ssg ships and the site does not
// use, with the command that adds one and the configuration it then needs.
export function availableItems(registry: Registry): AvailableItem[] {
  return registry.available.map((a) => ({
    ...a,
    command: `ssg new worker ${a.id}`,
    snippet:
      a.kind === "worker"
        ? `modules:\n  - id: ${a.id}\n    url: /${a.id}\n    dir: workers/${a.id}`
        : `workers:\n  - name: ${a.id}\n    dir: workers/${a.id}`,
  }));
}

export interface Stored {
  settings: Map<string, Setting>;
  latest: Map<string, Latest>;
  history: HistoryRow[];
  changes: LoggedChange[];
}

export async function readStored(db: D1Database): Promise<Stored> {
  const [settings, latest, history, changes] = await Promise.all([readSettings(db), latestStatuses(db), recentHistory(db), recentChanges(db)]);
  return { settings, latest, history, changes };
}
