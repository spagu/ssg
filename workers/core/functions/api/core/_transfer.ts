// Moving module settings between environments (GO-149): the export document,
// what importing one would change, and a fingerprint of the settings a
// preview was made against. A leading underscore keeps this file out of the
// Pages route table.
//
// An import REPLACES the settings: modules the file names get its values,
// the site's other modules go back to the defaults. The preview lists every
// field that would change, both kinds included, so nothing happens unseen.

import { sha256hex } from "./_lib";
import { EXPORT_FORMAT, EXPORT_KIND } from "./_settings-check";
import type { Imported } from "./_settings-check";
import { arrange, defaults, sameLabels } from "./_settings";
import type { Setting } from "./_settings";

export interface ExportDoc {
  kind: string;
  format: number;
  exportedAt: string;
  modules: Imported[];
}

const plain = (s: Setting): Imported => ({ id: s.id, hidden: s.hidden, position: s.position, labels: s.labels, note: s.note });

// exportDoc: every module of the site, in bar order, with its settings.
export function exportDoc(known: string[], settings: Map<string, Setting>, now: number): ExportDoc {
  return {
    kind: EXPORT_KIND,
    format: EXPORT_FORMAT,
    exportedAt: new Date(now).toISOString(),
    modules: arrange(known, (id) => id, settings, true).map((id) => plain(settings.get(id) ?? defaults(id))),
  };
}

export interface FieldChange {
  module: string;
  field: "hidden" | "position" | "labels" | "note";
  from: unknown;
  to: unknown;
}

// diff: every field the import would change, module by module in the site's
// order.
export function diff(known: string[], settings: Map<string, Setting>, incoming: Imported[]): FieldChange[] {
  const byId = new Map(incoming.map((m) => [m.id, m]));
  const out: FieldChange[] = [];
  for (const id of known) {
    const a = plain(settings.get(id) ?? defaults(id));
    const b = byId.get(id) ?? plain(defaults(id));
    if (a.hidden !== b.hidden) out.push({ module: id, field: "hidden", from: a.hidden, to: b.hidden });
    if (a.position !== b.position) out.push({ module: id, field: "position", from: a.position, to: b.position });
    if (!sameLabels(a.labels, b.labels)) out.push({ module: id, field: "labels", from: a.labels, to: b.labels });
    if (a.note !== b.note) out.push({ module: id, field: "note", from: a.note, to: b.note });
  }
  return out;
}

// fingerprint: the stored settings as they stand (not who changed them or
// when), so an import applies only to what its preview was made against.
export function fingerprint(known: string[], settings: Map<string, Setting>): Promise<string> {
  const rows = [...known].sort().map((id) => {
    const s = plain(settings.get(id) ?? defaults(id));
    return [s.id, s.hidden, s.position, Object.entries(s.labels).sort(), s.note];
  });
  return sha256hex(JSON.stringify(rows));
}
