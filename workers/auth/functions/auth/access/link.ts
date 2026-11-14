// Cloudflare Pages Function: connect Cloudflare Access to the signed-in
// account (GO-139).
//   GET  /auth/access/link   the confirmation page ("Access says you are …")
//   POST /auth/access/link   its plain form: links, then 303 to /auth/account
// Behind the same Access application as /auth/access; needs a session started
// within AUTH_RECENT_SIGNIN_MINUTES. The work is ../../api/auth/_access-entry.ts.

import { prepare } from "../../api/auth/_context";
import { linkPage, linkPost } from "../../api/auth/_access-entry";
import type { Env } from "../../api/auth/_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : linkPage(ctx);
};

export const onRequestPost: PagesFunction<Env> = ({ request, env }) => linkPost(request, env);
