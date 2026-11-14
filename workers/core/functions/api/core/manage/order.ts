// Cloudflare Pages Function: the order of the modules in the shared
// navigation and on the home page (GO-149).
//   PUT /api/core/manage/order   (x-core-admin: 1)
//     body {order: [module ids]} — every module of the registry, once
//     → {order: [ids], changed: 0 | 1}
//
// The panel moves one module up or down and sends the whole new order, so a
// position is never computed from a stale neighbour. Every module gets its
// index as position, in one batch with the change-log row.

import { actorOf, changeStatements } from "../_changes";
import { json } from "../_lib";
import type { D1Database, Env } from "../_lib";
import { knownIds, writing } from "../_manage";
import type { Registry } from "../_registry";
import { route } from "../_route";
import { checkOrder, LIMITS, readBody, refused } from "../_settings-check";
import { arrange, defaults, readSettings, upsert } from "../_settings";
import type { Caller } from "../_signin";

async function put(request: Request, env: Env, caller: Caller, registry: Registry, db: D1Database): Promise<Response> {
  const body = await readBody(request, LIMITS.bodyBytes);
  if (!body.ok) return refused(body);
  const known = knownIds(registry);
  const checked = checkOrder(body.value, known);
  if (!checked.ok) return refused(checked);
  const order = checked.value;

  const settings = await readSettings(db);
  const before = arrange(known, (id) => id, settings, true);
  if (before.every((id, i) => id === order[i])) return json({ order, changed: 0 });

  const who = actorOf(caller);
  const now = Date.now();
  const at = new Date(now).toISOString();
  await db.batch([
    ...order.map((id, position) => upsert(db, { ...(settings.get(id) ?? defaults(id)), position }, who.actor, at)),
    ...changeStatements(db, env, who, [{ module: null, action: "order", detail: { from: before, to: order } }], now),
  ]);
  return json({ order, changed: 1 });
}

export const onRequest = route({ PUT: writing(put) });
