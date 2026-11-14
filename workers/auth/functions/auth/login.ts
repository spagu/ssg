// Cloudflare Pages Function: GET /auth/login — the sign-in page.
// The page itself is ../api/auth/_pages-login.ts; this file only routes to it.

import { prepare } from "../api/auth/_context";
import type { Env } from "../api/auth/_lib";
import { loginPage } from "../api/auth/_pages-login";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : loginPage(ctx);
};
