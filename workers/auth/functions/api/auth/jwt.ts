// Cloudflare Pages Function: exchange an external issuer's JWT for a session (#435).
//   POST /api/auth/jwt   Authorization: Bearer <JWT>, or the form / JSON field
//                        `token`; optional `next` (a path on this host)
//
// Off until AUTH_JWT_* is configured (404 before). 303 to `next` with the
// usual session cookies on success; 303 to the login page with ?error=<code>
// on a refusal; 403 on the CSRF check, 503 jwt_misconfigured {hint}. The work
// is _external-entry.ts; docs/EXTERNAL-JWT.md is the reference.

import { externalExchange } from "./_external-entry";
import type { Env } from "./_lib";

export const onRequestPost: PagesFunction<Env> = ({ request, env }) => externalExchange(request, env);
