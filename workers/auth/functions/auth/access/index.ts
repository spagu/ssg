// Cloudflare Pages Function: GET /auth/access?next= — sign in through
// Cloudflare Access (GO-139). This path must sit behind the site's Access
// application (docs/ACCESS.md); the work is ../../api/auth/_access-entry.ts.

import { prepare } from "../../api/auth/_context";
import { accessEntry } from "../../api/auth/_access-entry";
import type { Env } from "../../api/auth/_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : accessEntry(ctx);
};
