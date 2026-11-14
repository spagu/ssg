// Reading a module's manifest — from the registry or from the module itself.
// A leading underscore keeps this file out of the Pages route table.
//
// _module-manifest.ts and _module-schema.json are byte-identical copies of
// workers/module-manifest.ts and workers/module.schema.json (the template is
// scaffolded on its own and cannot import from a sibling); the copies test
// keeps them equal and runs the reader's own suite against the copy.

import { readManifest } from "./_module-manifest";
import type { ModuleManifest, Schema } from "./_module-manifest";
import schema from "./_module-schema.json" with { type: "json" };

export type { ModuleManifest };
export type Checked = { ok: true; manifest: ModuleManifest } | { ok: false; detail: string };

// checkManifest accepts a manifest only when the reader does and it describes
// the module it was asked for: a module cannot speak for another.
export function checkManifest(raw: unknown, id: string): Checked {
  const read = readManifest(raw, schema as Schema);
  if (!read.ok) return { ok: false, detail: read.errors[0] };
  if (read.manifest.id !== id) return { ok: false, detail: `the manifest describes "${read.manifest.id}", not "${id}"` };
  return read;
}
