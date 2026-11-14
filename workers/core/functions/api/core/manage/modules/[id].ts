// Cloudflare Pages Function: one module's settings (GO-149).
//   PATCH /api/core/manage/modules/{id}   (x-core-admin: 1)
//     body {hidden?: boolean, labels?: {lang: label}, note?: string}
//     → {item: {id, hidden, position, labels, note, updatedAt, updatedBy}, changed}
//
// labels replaces the whole map (an empty label removes that language). A
// change is written with its change-log rows in one batch; a request that
// changes nothing writes nothing (changed: 0). Errors: 404 unknown_module,
// 413 too_large, 400 invalid_json, 422 invalid_settings, 503 no_database,
// 403 forbidden (no core:manage in the `auth` mode).

import { actorOf, changeStatements } from "../../_changes";
import type { Change } from "../../_changes";
import { json, problem } from "../../_lib";
import type { D1Database, Env } from "../../_lib";
import { knownIds, writing } from "../../_manage";
import type { Registry } from "../../_registry";
import { route } from "../../_route";
import { checkPatch, LIMITS, readBody, refused } from "../../_settings-check";
import { defaults, readSettings, sameLabels, upsert } from "../../_settings";
import type { Caller } from "../../_signin";

async function patch(request: Request, env: Env, caller: Caller, registry: Registry, db: D1Database): Promise<Response> {
  const id = decodeURIComponent(new URL(request.url).pathname.replace(/\/+$/, "").split("/").pop() as string);
  if (!knownIds(registry).includes(id)) return problem("unknown_module", 404, `${id} is not a module of this site`);
  const body = await readBody(request, LIMITS.bodyBytes);
  if (!body.ok) return refused(body);
  const checked = checkPatch(body.value);
  if (!checked.ok) return refused(checked);
  const p = checked.value;

  const current = (await readSettings(db)).get(id) ?? defaults(id);
  const changes: Change[] = [];
  if (p.hidden !== undefined && p.hidden !== current.hidden) changes.push({ module: id, action: p.hidden ? "hidden" : "shown", detail: {} });
  if (p.labels !== undefined && !sameLabels(p.labels, current.labels)) changes.push({ module: id, action: "labels", detail: { from: current.labels, to: p.labels } });
  if (p.note !== undefined && p.note !== current.note) changes.push({ module: id, action: "note", detail: { from: current.note, to: p.note } });
  if (!changes.length) return json({ item: current, changed: 0 });

  const who = actorOf(caller);
  const now = Date.now();
  const at = new Date(now).toISOString();
  const next = { ...current, ...p, updatedAt: at, updatedBy: who.actor };
  await db.batch([upsert(db, next, who.actor, at), ...changeStatements(db, env, who, changes, now)]);
  return json({ item: next, changed: changes.length });
}

export const onRequest = route({ PATCH: writing(patch) });
