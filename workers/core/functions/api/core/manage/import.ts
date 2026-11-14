// Cloudflare Pages Function: settings from another environment (GO-149).
//   POST /api/core/manage/import              (x-core-admin: 1)
//     body: a file GET /api/core/manage/export wrote (at most 64 KiB)
//     → {applied: false, fingerprint, changes: [{module, field, from, to}]}
//   POST /api/core/manage/import?apply=<fingerprint>
//     → {applied: true, changes} — or 409 settings_changed when the settings
//       moved since the preview with that fingerprint was made
//
// First a preview, then the same file again with the preview's fingerprint:
// what is applied is exactly what was shown. The file is checked strictly
// (_settings-check.ts: kind, format, known fields, this site's modules only)
// and applied in one batch — every setting and the change-log row, or none.

import { actorOf, changeStatements } from "../_changes";
import { json, problem } from "../_lib";
import type { D1Database, Env } from "../_lib";
import { knownIds, writing } from "../_manage";
import type { Registry } from "../_registry";
import { route } from "../_route";
import { checkImport, LIMITS, readBody, refused } from "../_settings-check";
import { readSettings, upsert } from "../_settings";
import type { Caller } from "../_signin";
import { diff, fingerprint } from "../_transfer";

async function post(request: Request, env: Env, caller: Caller, registry: Registry, db: D1Database): Promise<Response> {
  const body = await readBody(request, LIMITS.importBytes);
  if (!body.ok) return refused(body);
  const known = knownIds(registry);
  const checked = checkImport(body.value, known);
  if (!checked.ok) return refused(checked);
  const incoming = checked.value;

  const settings = await readSettings(db);
  const changes = diff(known, settings, incoming);
  const print = await fingerprint(known, settings);
  const apply = new URL(request.url).searchParams.get("apply");
  if (apply === null) return json({ applied: false, fingerprint: print, changes });
  if (apply !== print) return problem("settings_changed", 409, "the settings changed since this preview was made: preview the file again");
  if (!changes.length) return json({ applied: true, changes });

  const who = actorOf(caller);
  const now = Date.now();
  const at = new Date(now).toISOString();
  await db.batch([
    db.prepare("DELETE FROM core_module_settings"),
    ...incoming.map((m) => upsert(db, { ...m, updatedAt: at, updatedBy: who.actor }, who.actor, at)),
    ...changeStatements(db, env, who, [{ module: null, action: "import", detail: { modules: incoming.length, changes: changes.length } }], now),
  ]);
  return json({ applied: true, changes });
}

export const onRequest = route({ POST: writing(post) });
