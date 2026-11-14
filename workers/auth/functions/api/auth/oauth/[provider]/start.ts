// Cloudflare Pages Function: start a sign-in at a login provider (GO-151).
//   POST /api/auth/oauth/:provider/start
//        form (the provider buttons) or JSON with x-ssg-auth: 1:
//        {purpose?: "signin" (default) | "invite" | "link", next?, token?}
//
//   signin  anyone; `next` (a path on this host) is where a success goes.
//   invite  `token` is an open invitation's token: the identity the provider
//           returns becomes the new account's way in. The token itself is not
//           kept — only its SHA-256, in the flow (_provider-flow.ts).
//   link    the signed-in account adds this provider. Needs a sign-in from
//           the last AUTH_RECENT_SIGNIN_MINUTES (default 10), otherwise the
//           browser goes to /auth/login?reauth=1 first. An account with a
//           second factor needs a recent one instead (GO-140): 403
//           step_up_required, a form goes to /auth/mfa/step-up.
//
// A form gets a 303 to the provider's authorization endpoint with the flow
// cookie; JSON gets 200 {authorizationUrl} with the same cookie. Refusals: a
// provider this site does not offer 404 not_found (as if absent), one whose
// variables are wrong 503 provider_misconfigured {hint}; 400 invalid_request
// (unknown purpose); 410 invitation_invalid; 401 not_authenticated; 403
// reauth_required; 409 provider_already_linked — a form gets a 303 to the
// page that explains (?error=<code>). CSRF: form or x-ssg-auth (_request.ts).

import { preparePost, whoAmI } from "../../_context";
import { param } from "../../_admin";
import { openByToken } from "../../_invites";
import { isToken, json, redirect, sha256hex } from "../../_lib";
import type { Env } from "../../_lib";
import { safeNext, withParams } from "../../_next";
import { providerOr } from "../../_provider";
import { startFlow } from "../../_provider-flow";
import type { Purpose } from "../../_provider-flow";
import { freshness, stepUpRedirect, stepUpRequired } from "../../_mfa-step-up";
import { listIdentities } from "../../_provider-identities";
import { recentSignIn } from "../../_provider-policy";
import { isForm, readBody, str } from "../../_request";

const PURPOSES = new Set<Purpose>(["signin", "invite", "link"]);

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const found = providerOr(env, param(params, "provider"));
  if (found instanceof Response) return found;
  const { provider: p, setup } = found;
  const form = isForm(request);
  const b = (await readBody(request)) ?? {};
  const fail = (error: string, status: number, page: string): Response => (form ? redirect(page) : json({ error }, status));

  const purpose = (str(b.purpose) || "signin") as Purpose;
  if (!PURPOSES.has(purpose)) return fail("invalid_request", 400, withParams(ctx.s.loginUrl, { error: "invalid_request" }));
  let flow: Parameters<typeof startFlow>[2] = { purpose, next: "" };
  if (purpose === "signin") flow.next = safeNext(b.next, "");
  if (purpose === "invite") {
    const token = str(b.token);
    const inv = isToken(token) ? await openByToken(env, token, ctx.now) : null;
    if (!inv) return fail("invitation_invalid", 410, withParams("/auth/invite", { error: "invitation_invalid" }));
    flow.inviteHash = await sha256hex(token);
  }
  if (purpose === "link") {
    const back = "/auth/account";
    const who = await whoAmI(ctx, true);
    if (!who) return fail("not_authenticated", 401, withParams(ctx.s.loginUrl, { next: back }));
    // GO-140: with a second factor, a recent one (a step-up) instead of a recent sign-in.
    const fresh = await freshness(ctx, who);
    if (fresh === "step_up") return form ? stepUpRedirect(back) : stepUpRequired(ctx);
    if (fresh === "no_factor" && !recentSignIn(env, who.session, ctx.now)) return fail("reauth_required", 403, withParams(ctx.s.loginUrl, { reauth: "1", next: back }));
    if ((await listIdentities(env, who.user.id)).some((i) => i.provider === p.id)) {
      return fail("provider_already_linked", 409, withParams(back, { error: "provider_already_linked" }));
    }
    flow = { purpose, next: back, userId: who.user.id, sessionId: who.session.id };
  }
  const started = await startFlow(ctx, p, flow);
  const authorizationUrl = p.authorizeUrl(setup, started);
  return form ? redirect(authorizationUrl, [started.cookie]) : json({ authorizationUrl }, 200, {}, [started.cookie]);
};
