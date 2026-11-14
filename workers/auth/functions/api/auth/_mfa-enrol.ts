// What every new second factor sets off (GO-140), whatever its kind. A
// leading underscore keeps this file out of the Pages route table.
//
//   - the account's FIRST factor comes with ten backup codes, shown once;
//   - the session counts as having passed a second factor now (the person
//     just proved they hold it), so a new access token goes out at once —
//     with the roles that needed one;
//   - an audit event.

import { emitEvent } from "./_audit";
import { issueCookies } from "./_context";
import type { Ctx, Who } from "./_context";
import { json, redirect } from "./_lib";
import { listFactors, newBackupCodes, view } from "./_mfa-factors";
import type { FactorKind } from "./_mfa-factors";
import { codesPage } from "./_mfa-pages-codes";
import { markSessionFactor } from "./_mfa-policy";
import { withParams } from "./_next";

export async function enrolled(ctx: Ctx, who: Who, kind: FactorKind, factorId: string, form: boolean, back: string, status = 200): Promise<Response> {
  const factors = await listFactors(ctx.env, who.user.id);
  const codes = factors.length === 1 ? await newBackupCodes(ctx.env, who.user.id, ctx.now) : null;
  await markSessionFactor(ctx.env, who.session.id, kind, ctx.now);
  const { cookies } = await issueCookies(ctx, who.user, who.session, null);
  await emitEvent(ctx.env, { kind: "mfa_enrolled", userId: who.user.id, detail: { factor: kind, factorId, first: codes !== null } }, ctx.now);
  const factor = view(factors.find((f) => f.id === factorId) as (typeof factors)[number]);
  if (!form) return json({ factor, ...(codes ? { backupCodes: codes } : {}) }, status, {}, cookies);
  return codes ? codesPage(ctx, who, codes, back, cookies) : redirect(withParams(back, { ok: "factor_added" }), cookies);
}
