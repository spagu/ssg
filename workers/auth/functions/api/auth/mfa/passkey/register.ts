// Cloudflare Pages Function: add a passkey as a second factor (GO-140).
//   POST /api/auth/mfa/passkey/register   {credential: <navigator.credentials.create() answer,
//                                         binary fields base64url>, name?, next?} — JSON with
//                                         x-ssg-auth: 1, or the enrolment page's form
//                                         (credential as a JSON string, filled by auth.js)
//
// The answer must carry a challenge this session got from passkey/options
// (purpose register, unused, unexpired), this site's origin and rpId, user
// presence, and a key of a supported algorithm (ES256, RS256, EdDSA)
// (_webauthn.ts). The attestation statement is not verified. Then as every
// new factor (_mfa-enrol.ts): backup codes with the account's first, the
// session counts as having passed a second factor. 201 {factor, backupCodes?}
// + a new access cookie; 400 invalid_passkey (any check failed, or the
// credential is registered already); 401; 403 step_up_required |
// reauth_required; 404 not_found while the second factor is off. A form gets
// the backup-codes page (first factor), a 303 to `next` with ?ok=factor_added,
// or back to /auth/mfa/enrol?error=invalid_passkey.

import type { Env } from "../../_lib";
import { redirect } from "../../_lib";
import { ENROL_PAGE } from "../../_mfa-signin";
import { withParams } from "../../_next";
import { enrolled } from "../../_mfa-enrol";
import { mfaGate } from "../../_mfa-gate";
import { str } from "../../_request";
import { register } from "../../_webauthn-store";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await mfaGate(request, env, true);
  if (g instanceof Response) return g;
  const { ctx, who, body } = g;
  const name = [...str(body.name).trim()].slice(0, 60).join("");
  const id = await register(env, request, who.user.id, who.session.id, body.credential, name, ctx.now);
  if (!id) return g.form ? redirect(withParams(ENROL_PAGE, { error: "invalid_passkey", next: g.back })) : g.fail("invalid_passkey", 400);
  return enrolled(ctx, who, "passkey", id, g.form, g.back, 201);
};
