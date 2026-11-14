// Cloudflare Pages Function: finish adding an authenticator app (GO-140).
//   POST /api/auth/mfa/totp/confirm   {factorId, code, name?, next?} — JSON with x-ssg-auth: 1, or the enrolment page's form
//
// The first code from the app proves it holds the secret: the enrolment
// becomes a factor, the session counts as having passed a second factor (a
// new access cookie, with the roles that needed one), and that code's time
// step is used up. The account's FIRST factor also gets ten backup codes,
// shown in this one answer only. JSON: 200 {factor: {id, kind, name,
// createdAt, lastUsedAt}, backupCodes?}; a form gets the backup-codes page
// (first factor) or a 303 to `next` (default /auth/account) with
// ?ok=factor_added. 400 invalid_code (the code does not match; a form goes
// back to the QR code); 410 enrolment_expired (unknown, someone else's,
// confirmed already, or older than 15 minutes); 401, 403, 404, 503 as
// totp/start.

import type { Env } from "../../_lib";
import { redirect } from "../../_lib";
import { open } from "../../_mfa-crypto";
import { enrolled } from "../../_mfa-enrol";
import { confirmTotp, pendingTotp } from "../../_mfa-factors";
import { mfaGate } from "../../_mfa-gate";
import { ENROL_PAGE } from "../../_mfa-signin";
import { withParams } from "../../_next";
import { str } from "../../_request";
import { matchStep, normCode } from "../../_totp";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await mfaGate(request, env, true);
  if (g instanceof Response) return g;
  const { ctx, who, body } = g;
  const id = str(body.factorId);
  const row = await pendingTotp(env, who.user.id, id, ctx.now);
  if (!row) return g.fail("enrolment_expired", 410);
  const opened = await open(env, row.secret as string, row.id, who.user.id);
  if (!opened) return g.fail("mfa_unavailable", 503);
  const code = normCode(body.code);
  const step = code ? await matchStep(opened.plain, code, ctx.now) : null;
  if (step === null) return g.form ? redirect(withParams(ENROL_PAGE, { factor: id, next: g.back, error: "invalid_code" })) : g.fail("invalid_code", 400);
  const name = [...str(body.name).trim()].slice(0, 60).join("");
  if (!(await confirmTotp(env, who.user.id, id, step, name, ctx.now))) return g.fail("enrolment_expired", 410);
  return enrolled(ctx, who, "totp", id, g.form, g.back);
};
