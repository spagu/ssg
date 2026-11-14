// Cloudflare Pages Function: GET /auth/mfa — the second step of signing in (GO-140).
// The page itself is ../../api/auth/_mfa-pages.ts; this file only routes to it.

import type { Env } from "../../api/auth/_lib";
import { route } from "../../api/auth/_mfa-pages";
import { challengePage } from "../../api/auth/_mfa-pages";

export const onRequestGet: PagesFunction<Env> = route(challengePage);
