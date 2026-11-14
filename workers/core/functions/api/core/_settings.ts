// Module settings (GO-149): hidden, position, a label per language, an admin
// note — kept in CORE_DB and applied to what /api/core/modules and
// /api/core/overview show everyone. A leading underscore keeps this file out
// of the Pages route table.
//
// HIDING IS COSMETIC, like everything core shows: a hidden module leaves the
// shared navigation and the home page, and its panel keeps working at its own
// address, where the module checks access itself. To switch a module off,
// remove it from the site and deploy (its manifest's `disable` text says how).
//
// The settings are a convenience on top of the registry. Without CORE_DB, or
// when the database fails, readState answers null and everything is shown as
// the build listed it: the bar and the home page never break over a setting.

import type { D1Database, D1Statement, Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { latestStatuses } from "./_history";
import type { Latest } from "./_history";
import { byCodeUnit } from "./_lib";

export interface Setting {
  id: string;
  hidden: boolean;
  position: number | null; // null: after the positioned modules, in the registry's order
  labels: Record<string, string>; // {lang: label} — overrides the manifest's title in that language
  note: string;
  updatedAt: string | null;
  updatedBy: string | null;
}

export const defaults = (id: string): Setting => ({ id, hidden: false, position: null, labels: {}, note: "", updatedAt: null, updatedBy: null });

interface Row {
  module_id: string;
  hidden: number;
  position: number | null;
  labels: string;
  note: string;
  updated_at: string;
  updated_by: string;
}

// parseLabels reads the stored JSON, keeping only string values: a row edited
// by hand can lose its labels, never break a page.
export function parseLabels(raw: string): Record<string, string> {
  try {
    const v = JSON.parse(raw) as unknown;
    if (!v || typeof v !== "object" || Array.isArray(v)) return {};
    return Object.fromEntries(Object.entries(v).filter(([, l]) => typeof l === "string" && l.trim())) as Record<string, string>;
  } catch {
    return {};
  }
}

export async function readSettings(db: D1Database): Promise<Map<string, Setting>> {
  await ensureSchema(db);
  const { results } = await db.prepare("SELECT module_id, hidden, position, labels, note, updated_at, updated_by FROM core_module_settings").all<Row>();
  return new Map(
    results.map((r) => [
      r.module_id,
      { id: r.module_id, hidden: r.hidden === 1, position: r.position, labels: parseLabels(r.labels), note: r.note, updatedAt: r.updated_at, updatedBy: r.updated_by },
    ]),
  );
}

export interface State {
  settings: Map<string, Setting>;
  latest: Map<string, Latest>;
}

// readState: the settings and each module's last observed status, or null
// without CORE_DB or when the database fails (see the top of the file).
export async function readState(env: Env): Promise<State | null> {
  if (!env.CORE_DB) return null;
  try {
    const [settings, latest] = await Promise.all([readSettings(env.CORE_DB), latestStatuses(env.CORE_DB)]);
    return { settings, latest };
  } catch {
    return null;
  }
}

// arrange orders a list by the settings — positioned modules first, by
// position, then the rest in the order given — and leaves hidden ones out
// unless keepHidden. Without settings the list is returned as it came.
export function arrange<T>(list: T[], idOf: (item: T) => string, settings: Map<string, Setting> | undefined, keepHidden = false): T[] {
  if (!settings) return list;
  const rows = list.map((item, i) => ({ item, i, s: settings.get(idOf(item)) }));
  const shown = keepHidden ? rows : rows.filter((r) => !r.s?.hidden);
  const pos = (r: (typeof rows)[number]): number | null => r.s?.position ?? null;
  shown.sort((a, b) => Number(pos(a) === null) - Number(pos(b) === null) || (pos(a) ?? 0) - (pos(b) ?? 0) || a.i - b.i);
  return shown.map((r) => r.item);
}

// labelIn: the team's own name for a module in lang, or null for the
// manifest's title.
export const labelIn = (settings: Map<string, Setting> | undefined, id: string, lang: string): string | null => settings?.get(id)?.labels[lang] ?? null;

// relabel: a view with its title replaced by the label in lang, if any.
export function relabel<T extends { id: string; title: string }>(view: T, settings: Map<string, Setting> | undefined, lang: string): T {
  const label = labelIn(settings, view.id, lang);
  return label === null ? view : { ...view, title: label };
}

// upsert writes one module's settings whole.
export function upsert(db: D1Database, s: Setting, by: string, at: string): D1Statement {
  return db
    .prepare(
      `INSERT INTO core_module_settings (module_id, hidden, position, labels, note, updated_at, updated_by) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
       ON CONFLICT(module_id) DO UPDATE SET hidden = excluded.hidden, position = excluded.position, labels = excluded.labels,
         note = excluded.note, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
    )
    .bind(s.id, s.hidden ? 1 : 0, s.position, JSON.stringify(s.labels), s.note, at, by);
}

// sameLabels: two label maps say the same.
export function sameLabels(a: Record<string, string>, b: Record<string, string>): boolean {
  const ka = Object.keys(a).sort(byCodeUnit);
  const kb = Object.keys(b).sort(byCodeUnit);
  return ka.length === kb.length && ka.every((k, i) => k === kb[i] && a[k] === b[k]);
}
