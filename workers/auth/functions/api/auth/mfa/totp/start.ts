// Cloudflare Pages Function: begin adding an authenticator app (GO-140).
//   POST /api/auth/mfa/totp/start   JSON {} with x-ssg-auth: 1, or the enrolment page's plain form {next?}
//
// Makes a new secret (160 bits), stores it encrypted (AUTH_MFA_KEY) as an
// enrolment that is not a factor yet, and answers what the app needs.
// JSON: 201 {factorId, secret (base32), uri (otpauth://), qrSvg}; a form gets
// a 303 to /auth/mfa/enrol?factor=<id> — the page that draws the QR code.
// POST /api/auth/mfa/totp/confirm with the first code finishes it; until then
// (at most 15 minutes) it signs nothing in. Needs a live session and
// freshness (_mfa-gate.ts): 401 not_authenticated, 403 step_up_required /
// reauth_required; 404 not_found while the second factor is off; 503
// mfa_unavailable when AUTH_MFA_KEY is too short to use.

import type { Env } from "../../_lib";
import { json, redirect } from "../../_lib";
import { startTotp } from "../../_mfa-factors";
import { mfaGate } from "../../_mfa-gate";
import { ENROL_PAGE } from "../../_mfa-signin";
import { withParams } from "../../_next";
import { qrSvg } from "../../_mfa-qr";
import { base32Encode, otpauthUri } from "../../_totp";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await mfaGate(request, env, true);
  if (g instanceof Response) return g;
  const { ctx, who } = g;
  const started = await startTotp(env, who.user.id, ctx.now);
  if (!started) return g.fail("mfa_unavailable", 503);
  if (g.form) return redirect(withParams(ENROL_PAGE, { factor: started.id, next: g.back }));
  const secret = base32Encode(started.secret);
  const uri = otpauthUri(new URL(request.url).host, who.user.email, secret);
  return json({ factorId: started.id, secret, uri, qrSvg: qrSvg(uri, "QR code") }, 201);
};
