// Cloudflare Pages Function: the options for a passkey ceremony (GO-140).
//   POST /api/auth/mfa/passkey/options   JSON {purpose: "register" | "signin" | "step-up"}; header x-ssg-auth: 1
//
//   register  a live, fresh session (_mfa-gate.ts) → 200 {publicKey: <creation options>}
//   signin    the pending-step cookie → 200 {publicKey: <request options>}
//   step-up   a live session → 200 {publicKey: <request options>}
//
// Binary fields (challenge, user.id, credential ids) are base64url; the page's
// script decodes them for navigator.credentials. Every call issues a new
// single-use challenge, server-side, bound to the account, the purpose and the
// pending handle or session (_webauthn-store.ts). 404 no_passkey (signin,
// step-up: the account has none); 401 not_authenticated |
// mfa_pending_invalid; 400 invalid_request; 403 step_up_required |
// reauth_required (register); 404 not_found while the second factor is off.
// JSON only: a passkey needs the browser's script anyway.

import { preparePost, whoAmI } from "../../_context";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { mfaEnabled } from "../../_mfa-crypto";
import { mfaGate, notFound } from "../../_mfa-gate";
import { readPending } from "../../_mfa-pending";
import { readBody, str } from "../../_request";
import { assertOptions, registerOptions } from "../../_webauthn-store";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const purpose = str((await readBody(request.clone()))?.purpose);
  if (purpose === "register") {
    const g = await mfaGate(request, env, true, false);
    if (g instanceof Response) return g;
    return json({ publicKey: await registerOptions(env, request, g.who.user, g.who.session.id, g.ctx.now) });
  }
  const ctx = await preparePost(request, env, false);
  if (ctx instanceof Response) return ctx;
  if (!mfaEnabled(env)) return notFound();
  let options: Awaited<ReturnType<typeof assertOptions>>;
  if (purpose === "signin") {
    const pending = await readPending(env, request, ctx.now);
    if (!pending) return json({ error: "mfa_pending_invalid" }, 401);
    options = await assertOptions(env, request, pending.user.id, "signin", pending.hash, ctx.now);
  } else if (purpose === "step-up") {
    const who = await whoAmI(ctx, true);
    if (!who) return json({ error: "not_authenticated" }, 401);
    options = await assertOptions(env, request, who.user.id, "step-up", who.session.id, ctx.now);
  } else {
    return json({ error: "invalid_request", detail: "purpose: register, signin or step-up" }, 400);
  }
  return options ? json({ publicKey: options }) : json({ error: "no_passkey" }, 404);
};
