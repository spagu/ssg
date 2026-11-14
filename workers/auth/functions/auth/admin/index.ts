// Cloudflare Pages Function: GET /auth/admin/ — the accounts admin panel (GO-138).
// The shell is ../../api/auth/_admin-shell.ts; this file only routes to it.

import { adminShell } from "../../api/auth/_admin-shell";
import { prepare } from "../../api/auth/_context";
import type { Env } from "../../api/auth/_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : adminShell(ctx);
};
