// Cloudflare Pages Function: a one-time link to set a new password.
//   POST /api/auth/admin/users/:id/reset-link   {delivery?, lang?} (or no body)
//        → 201 {expiresAt, delivery: "mail"} or {expiresAt, delivery: "link", link}
//
// Requires `auth:admin`, and that the caller covers every permission the
// account holds (403 escalation): a link to an account is that account, so an
// administrator cannot use one to become an owner. The link works once, for
// AUTH_RESET_MINUTES (60); a newer link replaces it. Using it sets the
// password, ends every session of the account and clears its login lock.
// `delivery` as for invitations (_delivery.ts); `lang` is the language of the
// mail and the page (default: the caller's page language). 404
// user_not_found; 502 mail_failed. A link for ANOTHER account is that account,
// so it needs a recent second factor when the caller has one (403
// step_up_required, SR-05): a stale or stolen session must not mint one.

import { authorize, escalation, invalid, notFound, param } from "../../../_admin";
import { adminEvent, stepUpWhen } from "../../../_admin-sensitive";
import { chooseDelivery, handOver, linkTo } from "../../../_delivery";
import { effectivePermissions } from "../../../_grants";
import { asLang, langOf } from "../../../_i18n";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { resetMessage } from "../../../_mail";
import { beyond } from "../../../_perms";
import { readBody } from "../../../_request";
import { createReset, resetSeconds } from "../../../_resets";
import { findUserById } from "../../../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = (await readBody(request)) ?? {}; // the body is optional
  const user = await findUserById(env, param(params, "id"));
  if (!user) return notFound("user");
  const lang = b.lang === undefined ? langOf(request) : asLang(b.lang);
  if (!lang) return invalid("lang: en, pl, hi, de or fr");
  const over = beyond(caller.perms, await effectivePermissions(env, user.id));
  if (over.length) return escalation(over);
  const stepUp = await stepUpWhen(caller, user.id !== caller.user.id);
  if (stepUp) return stepUp;
  const mode = chooseDelivery(env, b.delivery);
  if (mode instanceof Response) return mode;

  const { token, expiresAt } = await createReset(env, user.id, caller.user.id, caller.ctx.now);
  const url = linkTo(request, "reset", token, lang);
  const site = new URL(request.url).host;
  const sent = await handOver(env, mode, url, () => resetMessage({ to: user.email, lang, site, url, minutes: resetSeconds(env) / 60 }));
  if (sent instanceof Response) return sent;
  // GO-138: audit reset_link_issued — how it went out, never the link.
  await adminEvent(caller, "reset_link_issued", user.id, { delivery: sent.delivery });
  return json({ expiresAt, ...sent }, 201);
};
