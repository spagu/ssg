// Cloudflare Pages Function: connect an external issuer's identity to the
// signed-in account (#435).
//   GET  /auth/jwt/link   the confirmation page ("<issuer> says you are …")
//   POST /auth/jwt/link   its plain form (this origin only): links, then 303
//                         to /auth/account
// Needs the identity a token left at POST /api/auth/jwt/link within five
// minutes, and a session started within AUTH_RECENT_SIGNIN_MINUTES. The work
// is ../../api/auth/_external-link.ts.

import { prepare } from "../../api/auth/_context";
import { externalLinkConfirm, externalLinkPage } from "../../api/auth/_external-link";
import type { Env } from "../../api/auth/_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  return ctx instanceof Response ? ctx : externalLinkPage(ctx);
};

export const onRequestPost: PagesFunction<Env> = ({ request, env }) => externalLinkConfirm(request, env);
