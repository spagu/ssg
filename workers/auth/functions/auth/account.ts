// Cloudflare Pages Function: GET /auth/account — your account.
// The page itself is ../api/auth/_pages-account.ts; this file only routes to it.

import { prepare } from "../api/auth/_context";
import type { Env } from "../api/auth/_lib";
import { accountPage } from "../api/auth/_pages-account";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : accountPage(ctx);
};
