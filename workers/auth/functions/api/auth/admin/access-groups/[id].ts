// Cloudflare Pages Function: remove one Access group mapping (GO-139).
//   DELETE /api/auth/admin/access-groups/:id → 200 {ok: true, resynced}
//
// Requires `auth:admin`. The memberships that mapping added are removed at
// once for everyone whose groups an Access sign-in recorded (unless another
// mapping still earns them; never a membership added by hand; never the last
// active owner's — that one is kept and logged); `resynced` counts the people
// with a change. Taking the group's permissions away follows the rule for
// handing them out (403 escalation; `*` / `auth:*` a recent second factor —
// SR-04). Audit: access_mapping_removed. 404 mapping_not_found.

import { authorize, notFound, param } from "../../_admin";
import { deleteMapping, listMappings } from "../../_access-groups";
import { adminEvent, mayTakeAway } from "../../_admin-sensitive";
import { groupPermissions } from "../../_grants";
import { resyncEveryone } from "../../_access-sync";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const mapping = (await listMappings(env)).find((m) => m.id === param(params, "id"));
  if (!mapping) return notFound("mapping");
  const refused = await mayTakeAway(caller, await groupPermissions(env, mapping.groupId));
  if (refused) return refused;
  if (!(await deleteMapping(env, mapping.id))) return notFound("mapping");
  await adminEvent(caller, "access_mapping_removed", null, { accessGroup: mapping.accessGroup, group: mapping.groupName });
  return json({ ok: true, resynced: await resyncEveryone(env, caller.ctx.now) });
};
