// Cloudflare Pages Function: an external issuer's token arrives to be linked
// to an account (#435).
//   POST /api/auth/jwt/link   Authorization: Bearer <JWT>, or the form / JSON
//                             field `token` — as POST /api/auth/jwt
// Off until AUTH_JWT_* is configured (404 before). Links nothing: 303 to
// /auth/jwt/link, where the signed-in person confirms; refusals 303 to the
// account page with ?error=<code>. The work is ../_external-link.ts.

import { externalLinkArrival } from "../_external-link";
import type { Env } from "../_lib";

export const onRequestPost: PagesFunction<Env> = ({ request, env }) => externalLinkArrival(request, env);
