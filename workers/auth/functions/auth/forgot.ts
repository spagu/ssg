// Cloudflare Pages Function: GET /auth/forgot — ask for a reset link (only with mail).
// The page itself is ../api/auth/_pages-links.ts; this file only routes to it.

import { prepare } from "../api/auth/_context";
import type { Env } from "../api/auth/_lib";
import { forgotPage } from "../api/auth/_pages-links";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : forgotPage(ctx);
};
