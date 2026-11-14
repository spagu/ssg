// How a one-time link (invitation, password reset) reaches its person: by
// mail when a transport is configured, otherwise handed to the administrator
// to pass on. A leading underscore keeps this file out of the Pages route
// table. The link is never logged.

import { invalid } from "./_admin";
import type { Lang } from "./_i18n";
import { inviteSeconds } from "./_invites";
import type { Invitation } from "./_invites";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { inviteMessage, mailState, send } from "./_mail";
import type { User } from "./_users";

export type Mode = "mail" | "link";

// chooseDelivery reads the optional `delivery` field: absent = mail when a
// transport works, the link otherwise; "link" = always the link; "mail" =
// mail or an error. A transport that is configured but can never send is an
// error (503 mail_misconfigured), never a silent fallback to the link.
export function chooseDelivery(env: Env, raw: unknown): Mode | Response {
  if (raw !== undefined && raw !== "mail" && raw !== "link") return invalid('delivery: "mail" or "link"');
  if (raw === "link") return "link";
  const state = mailState(env);
  if (state.kind === "broken") return json({ error: "mail_misconfigured", detail: state.problem }, 503);
  if (state.kind === "none") return raw === "mail" ? json({ error: "mail_not_configured" }, 409) : "link";
  return "mail";
}

// linkTo is the page that opens a token, in the person's language.
export function linkTo(request: Request, page: "invite" | "reset", token: string, lang: Lang): string {
  return `${new URL(request.url).origin}/auth/${page}?token=${token}&lang=${lang}`;
}

// handOver sends `message` (mode "mail") or returns the link (mode "link"):
// the answer's fields, or the 502 to send when the transport refused.
export async function handOver(
  env: Env,
  mode: Mode,
  link: string,
  message: () => { to: string; subject: string; text: string },
): Promise<{ delivery: Mode; link?: string } | Response> {
  if (mode === "link") return { delivery: "link", link };
  const sent = await send(env, message());
  if (sent.ok) return { delivery: "mail" };
  console.warn(`auth: mail not sent: ${sent.detail}`);
  return json({ error: "mail_failed", detail: sent.detail, temporary: sent.temporary }, 502);
}

// deliverInvitation hands an invitation's link over (create and resend).
export function deliverInvitation(env: Env, request: Request, mode: Mode, inv: Invitation, token: string, inviter: User) {
  const url = linkTo(request, "invite", token, inv.lang);
  return handOver(env, mode, url, () =>
    inviteMessage({
      to: inv.email,
      lang: inv.lang,
      site: new URL(request.url).host,
      inviter: inviter.display_name || inviter.email,
      url,
      expires: inv.expiresAt,
      hours: inviteSeconds(env) / 3600,
    }),
  );
}
