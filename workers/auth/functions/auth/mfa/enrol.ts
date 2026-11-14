// Cloudflare Pages Function: GET /auth/mfa/enrol — set up a second factor (GO-140).
// The page itself is ../../api/auth/_mfa-pages-enrol.ts; this file only routes to it.

import type { Env } from "../../api/auth/_lib";
import { route } from "../../api/auth/_mfa-pages";
import { enrolPage } from "../../api/auth/_mfa-pages-enrol";

export const onRequestGet: PagesFunction<Env> = route(enrolPage);
