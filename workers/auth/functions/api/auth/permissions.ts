// Cloudflare Pages Function: the registry of known permissions.
//   GET /api/auth/permissions   any signed-in account
//
// 200 {items: [{name, module, description, known}], nextCursor: null,
// problems: [string]} — every permission a module declares (this worker's own
// manifest plus AUTH_KNOWN_PERMISSIONS, see _registry.ts), then every name a
// role holds that no module declares, with known: false, module and
// description null. Sorted by name. `problems` lists configuration entries
// that were skipped. 401 without a session.

import { authorize } from "./_admin";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { isKnown, registryOf } from "./_registry";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  const reg = registryOf(env);
  const { results } = await env.AUTH_DB.prepare("SELECT DISTINCT permission FROM role_permissions").all<{ permission: string }>();
  const unknown = results.map((r) => r.permission).filter((p) => !isKnown(reg, p));
  const items = [
    ...[...reg.known.values()].map((k) => ({ ...k, known: true })),
    ...unknown.map((name) => ({ name, module: null, description: null, known: false })),
  ].sort((a, b) => (a.name < b.name ? -1 : 1));
  return json({ items, nextCursor: null, problems: reg.problems });
};
