// Cloudflare Pages Function: how I sign in (GO-151).
//   GET /api/auth/methods → 200 {password, identities: [{id, provider, name,
//       email, linkedAt, lastUsedAt}], available: [{id, name}], sessionMethod,
//       recentSignIn, recentMinutes}
//
// The caller's own methods: whether the account has a password, each linked
// provider identity with the address the provider showed when it was linked,
// the enabled providers not linked yet, how this session was started, and
// whether it is recent enough to change methods. Never a subject, a token or
// a hash. 401 not_authenticated without a valid access token.

import { authorize } from "../_admin";
import { json } from "../_lib";
import type { Env } from "../_lib";
import { methodsOf } from "../_provider-account";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  return json(await methodsOf(caller.ctx, caller));
};
