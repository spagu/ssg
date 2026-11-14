// Cloudflare Pages Function: GET /auth/ — Your panels.
// The page itself is ../api/auth/_pages-panels.ts; this file only routes to it.

import { prepare } from "../api/auth/_context";
import type { Env } from "../api/auth/_lib";
import { panelsPage } from "../api/auth/_pages-panels";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : panelsPage(ctx);
};
