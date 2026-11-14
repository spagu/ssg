// Cloudflare Pages Function: a new set of backup codes (GO-140).
//   POST /api/auth/mfa/backup-codes   {next?} — JSON with x-ssg-auth: 1, or the account page's plain form
//
// Ten new one-time codes replace the whole earlier set (used or not); only
// their hashes are stored, so this answer is the one place they appear.
// Needs a second factor on the account and a recent one in this session
// (step-up). JSON: 200 {backupCodes: ["XXXX-XXXX-XXXX-XXXX", …]}; a form gets
// the backup-codes page. 409 no_factor; 401 not_authenticated; 403
// step_up_required; 404 not_found while the second factor is off.

import { emitEvent } from "../_audit";
import { json } from "../_lib";
import type { Env } from "../_lib";
import { hasFactor, newBackupCodes } from "../_mfa-factors";
import { mfaGate } from "../_mfa-gate";
import { codesPage } from "../_mfa-pages-codes";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await mfaGate(request, env, true);
  if (g instanceof Response) return g;
  const { ctx, who } = g;
  if (!(await hasFactor(env, who.user.id))) return g.fail("no_factor", 409);
  const codes = await newBackupCodes(env, who.user.id, ctx.now);
  await emitEvent(env, { kind: "mfa_backup_codes_regenerated", userId: who.user.id }, ctx.now);
  return g.form ? codesPage(ctx, who, codes, g.back) : json({ backupCodes: codes });
};
