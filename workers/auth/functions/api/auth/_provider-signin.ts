// What a verified provider identity may do (GO-151), per the purpose its flow
// was started for. A leading underscore keeps this file out of the Pages
// route table.
//
// Only invited people get in, and an identity attaches to an account in
// exactly two ways — the address never does it on its own:
//   signin  the pair (provider, subject) is linked → that account, if active.
//           Not linked → the address may match an OPEN INVITATION, and only an
//           address the provider asserts it verified (Facebook never does);
//           that accepts the invitation. Nothing else: a verified address
//           equal to an existing account's is NOT a way into that account.
//           No invitation → refused, and no users row is written.
//   invite  the invitee opened the invitation link and chose this provider;
//           possession of the link is the proof, so the provider's address may
//           be unverified or differ from the invited one — accepted, and the
//           difference recorded in the audit event.
//   link    a signed-in account adds this provider; the session that started
//           the flow must still be live, its own and recent.
// Every success goes through startSession (_signin.ts) — the one place a
// session is made, where GO-140's second factor sits: an account with one
// goes to the challenge page, never straight in on the provider's word.

import { emitEvent } from "./_audit";
import type { Ctx } from "./_context";
import { acceptByHash } from "./_invites-accept";
import { openByEmail, openByHash } from "./_invites";
import type { Invitation } from "./_invites";
import { redirect } from "./_lib";
import { freshness } from "./_mfa-step-up";
import { withParams } from "./_next";
import type { Identity, Provider } from "./_provider";
import { clearFlowCookie } from "./_provider-flow";
import type { Flow } from "./_provider-flow";
import { findIdentity, identityInsert, linkIdentity, touchIdentity } from "./_provider-identities";
import { recentSignIn } from "./_provider-policy";
import { getSession, live, purgeStale } from "./_sessions";
import type { Session } from "./_sessions";
import { signInRedirect, startSession } from "./_signin";
import { findUserById, touchLogin } from "./_users";
import type { User } from "./_users";

type Detail = Record<string, string | boolean>;

// refuse ends a flow with an error page and an audit event.
export async function refuse(ctx: Ctx, p: Provider, flow: Flow | null, error: string, detail: Detail = {}): Promise<Response> {
  const link = flow?.purpose === "link";
  const purpose = flow?.purpose ?? "unknown";
  await emitEvent(ctx.env, { kind: "signin_refused", userId: link ? flow.user_id : null, method: p.id, detail: { reason: error, purpose, ...detail } }, ctx.now);
  const where = link ? withParams("/auth/account", { error }) : withParams(ctx.s.loginUrl, { error, next: flow?.purpose === "signin" ? flow.next : "" });
  return redirect(where, [clearFlowCookie(p.id)]);
}

// finish signs the account in with this provider and goes to `location`.
async function finish(ctx: Ctx, p: Provider, user: User, identityId: string, location: string): Promise<Response> {
  const started = await startSession(ctx, user, p.id, location);
  if (started.ok !== false) {
    await touchLogin(ctx.env, user.id, ctx.now);
    await touchIdentity(ctx.env, identityId, ctx.now);
    await purgeStale(ctx.env, ctx.now);
  }
  return signInRedirect(ctx, started, location, [clearFlowCookie(p.id)]);
}

const seen = (id: Identity): Detail => ({ email: id.email, emailVerified: id.emailVerified });

export async function signInWith(ctx: Ctx, p: Provider, id: Identity, flow: Flow): Promise<Response> {
  const linked = await findIdentity(ctx.env, p.id, id.subject);
  if (linked) {
    const user = await findUserById(ctx.env, linked.user_id);
    if (!user) return refuse(ctx, p, flow, "not_invited", seen(id));
    if (user.status !== "active") return refuse(ctx, p, flow, "account_blocked", { userId: user.id });
    return finish(ctx, p, user, linked.id, flow.next || ctx.s.afterLoginUrl);
  }
  if (!id.emailVerified) return refuse(ctx, p, flow, "email_unverified", seen(id));
  const inv = await openByEmail(ctx.env, id.email, ctx.now);
  if (!inv) return refuse(ctx, p, flow, "not_invited", seen(id));
  const row = await ctx.env.AUTH_DB.prepare("SELECT token_hash FROM invitations WHERE id = ?").bind(inv.id).first<{ token_hash: string }>();
  return accept(ctx, p, id, flow, inv, (row as { token_hash: string }).token_hash, "verified_email");
}

export async function acceptWith(ctx: Ctx, p: Provider, id: Identity, flow: Flow): Promise<Response> {
  const inv = flow.invite_hash ? await openByHash(ctx.env, flow.invite_hash, ctx.now) : null;
  if (!inv) return refuse(ctx, p, flow, "invitation_invalid", seen(id));
  return accept(ctx, p, id, flow, inv, flow.invite_hash as string, "invitation_link");
}

// accept creates the invited account with this identity linked, in the
// invitation's own transaction (_invites-accept.ts), and signs it in.
async function accept(ctx: Ctx, p: Provider, id: Identity, flow: Flow, inv: Invitation, hash: string, via: string): Promise<Response> {
  if (await findIdentity(ctx.env, p.id, id.subject)) return refuse(ctx, p, flow, "identity_in_use", seen(id));
  const name = inv.name || id.name;
  const r = await acceptByHash(ctx.env, inv, hash, { name, passwordHash: null }, ctx.now, (userId) => [identityInsert(ctx.env, userId, p.id, id, ctx.now, true)]);
  if (r.kind === "invalid") return refuse(ctx, p, flow, "invitation_invalid", seen(id));
  if (r.kind === "account_exists") return refuse(ctx, p, flow, "account_exists", seen(id));
  const detail = { via, invitationId: inv.id, invitedEmail: inv.email, providerEmail: id.email, emailVerified: id.emailVerified, mismatch: id.email !== inv.email };
  await emitEvent(ctx.env, { kind: "invitation_accepted", userId: r.userId, method: p.id, detail }, ctx.now);
  await emitEvent(ctx.env, { kind: "method_linked", userId: r.userId, method: p.id, detail: seen(id) }, ctx.now);
  const user = (await findUserById(ctx.env, r.userId)) as User;
  const identity = (await findIdentity(ctx.env, p.id, id.subject)) as { id: string };
  const skipped = r.skipped.roles.length + r.skipped.groups.length;
  return finish(ctx, p, user, identity.id, withParams("/auth/", { welcome: "1", skipped: skipped ? String(skipped) : "" }));
}

export async function linkWith(ctx: Ctx, p: Provider, id: Identity, flow: Flow): Promise<Response> {
  const session = await getSession(ctx.env, flow.session_id as string);
  const own = session && live(session, ctx.now) && session.user_id === flow.user_id ? session : null;
  // The start route's rule, checked again here (SR-07): with a second factor,
  // a recent one (a step-up); without, a recent sign-in.
  const fresh = own ? await freshness(ctx, { user: { id: own.user_id }, session: own }) : "step_up";
  const ok = fresh === "recent" || (fresh === "no_factor" && recentSignIn(ctx.env, own as Session, ctx.now));
  const user = ok ? await findUserById(ctx.env, flow.user_id as string) : null;
  if (!user || user.status !== "active") return refuse(ctx, p, flow, "reauth_required");
  const r = await linkIdentity(ctx.env, user.id, p.id, id, ctx.now);
  if (r === "in_use") return refuse(ctx, p, flow, "identity_in_use", seen(id));
  if (r === "provider_linked") return refuse(ctx, p, flow, "provider_already_linked", seen(id));
  await emitEvent(ctx.env, { kind: "method_linked", userId: user.id, method: p.id, detail: seen(id) }, ctx.now);
  return redirect(withParams("/auth/account", { ok: "method_linked" }), [clearFlowCookie(p.id)]);
}
