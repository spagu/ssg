// Cloudflare Pages Function: remove one of my sign-in methods (GO-151).
//   DELETE /api/auth/methods/:id   header x-ssg-auth: 1 → 200 {ok: true}
//   POST   /api/auth/methods/:id   the same from the account page's plain form
//
// :id is "password" (the account's password) or the id of one of its linked
// provider identities (GET /api/auth/methods). Needs a live session started
// within AUTH_RECENT_SIGNIN_MINUTES. The last method of an account cannot be
// removed — the database refuses it, however many removals race (409
// method_last). Someone else's identity, an unknown id and an already removed
// one are all 404 method_not_found. 401 not_authenticated; 403
// reauth_required. A form gets a 303 to /auth/account with ?ok=method_removed
// or ?error=<code> (the login page when not signed in or not recently).

import { param } from "../_admin";
import { emitEvent } from "../_audit";
import type { Env } from "../_lib";
import { changeGate } from "../_provider-account";
import { listIdentities, removeIdentity, removePassword } from "../_provider-identities";
import { PASSWORD_METHOD } from "../_provider-policy";

const remove: PagesFunction<Env> = async ({ request, env, params }) => {
  const g = await changeGate(request, env);
  if (g instanceof Response) return g;
  const { ctx, who } = g;
  const id = param(params, "id");
  const identity = (await listIdentities(env, who.user.id)).find((i) => i.id === id);
  const method = id === PASSWORD_METHOD ? PASSWORD_METHOD : identity?.provider;
  if (!method) return g.fail("method_not_found", 404);
  const r = method === PASSWORD_METHOD ? await removePassword(env, who.user.id, ctx.now) : await removeIdentity(env, who.user.id, id);
  if (r === "last_method") return g.fail("method_last", 409);
  if (r === "not_found") return g.fail("method_not_found", 404);
  await emitEvent(env, { kind: "method_unlinked", userId: who.user.id, method, detail: identity ? { email: identity.email } : {} }, ctx.now);
  return g.done("method_removed", { ok: true });
};

export const onRequestDelete = remove;
export const onRequestPost = remove;
