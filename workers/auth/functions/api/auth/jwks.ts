// Cloudflare Pages Function: the public keys that verify access tokens.
//   GET /api/auth/jwks   →   {keys: [current, previous?]}  (RFC 7517 JWK Set)
//
// The current signing key's public half, plus AUTH_PREVIOUS_PUBLIC_KEY during
// a rotation. Public keys only — the private `d` is never copied into the
// answer (publicJwk is built field by field in _keys.ts). The one answer that
// may be cached: five minutes, so a rotation reaches verifiers quickly. Needs
// no database; 503 without a usable AUTH_SIGNING_KEY.

import { prepare } from "./_context";
import { json } from "./_lib";
import type { Env } from "./_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env, false);
  if (ctx instanceof Response) return ctx;
  return json({ keys: ctx.keys.jwks }, 200, { "cache-control": "public, max-age=300" });
};
