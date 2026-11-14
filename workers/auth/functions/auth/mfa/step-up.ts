// Cloudflare Pages Function: GET /auth/mfa/step-up — the second factor again, before a sensitive change (GO-140).
// The page itself is ../../api/auth/_mfa-pages.ts; this file only routes to it.

import type { Env } from "../../api/auth/_lib";
import { route } from "../../api/auth/_mfa-pages";
import { stepUpPage } from "../../api/auth/_mfa-pages";

export const onRequestGet: PagesFunction<Env> = route(stepUpPage);
