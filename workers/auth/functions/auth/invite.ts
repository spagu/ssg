// Cloudflare Pages Function: GET /auth/invite?token=… — accept an invitation.
// The page itself is ../api/auth/_pages-links.ts; this file only routes to it.

import { prepare } from "../api/auth/_context";
import type { Env } from "../api/auth/_lib";
import { invitePage } from "../api/auth/_pages-links";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : invitePage(ctx);
};
