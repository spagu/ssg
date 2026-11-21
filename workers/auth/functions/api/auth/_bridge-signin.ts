// Who a verified identity from someone else's token signs in: the one
// algorithm behind Cloudflare Access (GO-139, _access-signin.ts) and an
// external issuer's JWT (#435, _external-signin.ts). A leading underscore
// keeps this file out of the Pages route table.
//
// In order — the first that applies decides:
//   1. the address domain allowed (AUTH_<ID>_ALLOWED_DOMAINS, like any
//      provider's), else refused;
//   2. (method, subject) is linked → that account, if active (a blocked
//      account is refused whatever the token says);
//   3. the address cannot be matched (_access-identity.ts) → refused;
//   4. an EXISTING account has exactly this address → only when the bridge
//      links by address is the identity linked to it and the account entered
//      (audited as a link "via email"); otherwise refused with the bridge's
//      "not linked" code;
//   5. an open invitation for the address → accepted (when the bridge accepts
//      invitations), the identity linked, the invitation's roles and groups
//      given;
//   6. the bridge creates accounts → a new account, with the one default role
//      when the bridge names one (never an administrator, _access-provision.ts);
//   7. otherwise refused (not_invited) — and no users row is written.
// Then the bridge's own step (a group or role synchronisation) runs BEFORE the
// session starts, so its first token already reflects it; and the session is
// started by startSession (_signin.ts), the one place sessions are made —
// where a second factor (GO-140) is asked for when a role needs one: no
// bridge is ever assumed to have checked a second factor.

import { emitEvent } from "./_audit";
import { createAccessAccount } from "./_access-provision";
import type { Ctx } from "./_context";
import { acceptByHash } from "./_invites-accept";
import { openByEmail } from "./_invites";
import type { Invitation } from "./_invites";
import { redirect } from "./_lib";
import { withParams } from "./_next";
import type { Identity } from "./_provider";
import { findIdentity, identityInsert, linkIdentity, touchIdentity } from "./_provider-identities";
import { domainAllowed } from "./_provider-policy";
import { purgeStale } from "./_sessions";
import { signInRedirect, startSession } from "./_signin";
import { findUserByEmail, findUserById, touchLogin } from "./_users";
import type { User } from "./_users";

export interface Bridge {
  method: string; // stored with the session, as identities.provider and in the audit trail
  domainsOf: string; // the <ID> of AUTH_<ID>_ALLOWED_DOMAINS
  linkByEmail: boolean;
  invite: boolean;
  create: { roleId: string | null; roleName: string } | null;
  codes: { emailUnusable: string; notLinked: string };
  via: string; // the invitation_accepted detail's `via`
  // before: the bridge's own step for the account, right before the session.
  before: (user: User) => Promise<void>;
}

export interface Arrival {
  bridge: Bridge;
  identity: Identity;
  next: string; // a safe path, or ""
}

type Detail = Record<string, string | boolean | null>;

// refuseBridge ends a sign-in on the login page with ?error=<code>, and an
// audit event (reasons and addresses, never the token).
export async function refuseBridge(ctx: Ctx, method: string, error: string, detail: Detail, next: string, userId: string | null = null): Promise<Response> {
  await emitEvent(ctx.env, { kind: "signin_refused", userId, method, detail: { reason: error, ...detail } }, ctx.now);
  return redirect(withParams(ctx.s.loginUrl, { error, next }));
}

const seen = (id: Identity): Detail => ({ email: id.email, emailVerified: id.emailVerified });

// finish: the bridge's step first, then the one entry every first factor goes
// through — which may answer with a second-factor challenge (GO-140) instead
// of a session; signInRedirect sends the browser to the right next page.
async function finish(ctx: Ctx, a: Arrival, user: User, location: string): Promise<Response> {
  await a.bridge.before(user);
  const started = await startSession(ctx, user, a.bridge.method, location);
  if (started.ok !== false) {
    const identity = (await findIdentity(ctx.env, a.bridge.method, a.identity.subject)) as { id: string };
    await touchLogin(ctx.env, user.id, ctx.now);
    await touchIdentity(ctx.env, identity.id, ctx.now);
    await purgeStale(ctx.env, ctx.now);
  }
  return signInRedirect(ctx, started, location);
}

export async function bridgeSignIn(ctx: Ctx, a: Arrival): Promise<Response> {
  const { bridge: b, identity: id } = a;
  const refuse = (error: string, userId: string | null = null) => refuseBridge(ctx, b.method, error, seen(id), a.next, userId);
  const where = a.next || ctx.s.afterLoginUrl;
  if (!domainAllowed(ctx.env, b.domainsOf, id)) return refuse("domain_not_allowed");
  const linked = await findIdentity(ctx.env, b.method, id.subject);
  if (linked) {
    const user = await findUserById(ctx.env, linked.user_id);
    if (!user) return refuse("not_invited");
    if (user.status !== "active") return refuse("account_blocked", user.id);
    return finish(ctx, a, user, where);
  }
  if (!id.emailVerified) return refuse(b.codes.emailUnusable);
  const existing = await findUserByEmail(ctx.env, id.email);
  if (existing) return byEmail(ctx, a, existing, where);
  const inv = b.invite ? await openByEmail(ctx.env, id.email, ctx.now) : null;
  if (inv) return accept(ctx, a, inv);
  if (b.create) return provision(ctx, a, b.create, where);
  return refuse("not_invited");
}

// byEmail: an existing account with this address — entered only when the
// bridge links by address, and never a blocked one.
async function byEmail(ctx: Ctx, a: Arrival, user: User, where: string): Promise<Response> {
  const { bridge: b, identity: id } = a;
  const refuse = (error: string) => refuseBridge(ctx, b.method, error, seen(id), a.next, user.id);
  if (!b.linkByEmail) return refuse(b.codes.notLinked);
  if (user.status !== "active") return refuse("account_blocked");
  const r = await linkIdentity(ctx.env, user.id, b.method, id, ctx.now);
  if (r !== "ok") return refuse(r === "in_use" ? "identity_in_use" : "provider_already_linked");
  await emitEvent(ctx.env, { kind: "method_linked", userId: user.id, method: b.method, detail: { ...seen(id), via: "email" } }, ctx.now);
  return finish(ctx, a, user, where);
}

// accept: the invitation for this address, accepted in its own transaction
// with the identity linked (_invites-accept.ts).
async function accept(ctx: Ctx, a: Arrival, inv: Invitation): Promise<Response> {
  const { bridge: b, identity: id } = a;
  const row = (await ctx.env.AUTH_DB.prepare("SELECT token_hash FROM invitations WHERE id = ?").bind(inv.id).first<{ token_hash: string }>()) as { token_hash: string };
  const r = await acceptByHash(ctx.env, inv, row.token_hash, { name: inv.name, passwordHash: null }, ctx.now, (userId) => [
    identityInsert(ctx.env, userId, b.method, id, ctx.now, true),
  ]);
  if (r.kind === "invalid") return refuseBridge(ctx, b.method, "invitation_invalid", seen(id), a.next);
  if (r.kind === "account_exists") return refuseBridge(ctx, b.method, "account_exists", seen(id), a.next);
  const detail = { via: b.via, invitationId: inv.id, invitedEmail: inv.email, providerEmail: id.email, emailVerified: true, mismatch: false };
  await emitEvent(ctx.env, { kind: "invitation_accepted", userId: r.userId, method: b.method, detail }, ctx.now);
  await emitEvent(ctx.env, { kind: "method_linked", userId: r.userId, method: b.method, detail: { ...seen(id), via: "invitation" } }, ctx.now);
  const skipped = r.skipped.roles.length + r.skipped.groups.length;
  return finish(ctx, a, (await findUserById(ctx.env, r.userId)) as User, withParams("/auth/", { welcome: "1", skipped: skipped ? String(skipped) : "" }));
}

// provision: a new account — the default role only, or none (checked by the
// route before this point, _access-provision.ts).
async function provision(ctx: Ctx, a: Arrival, create: NonNullable<Bridge["create"]>, where: string): Promise<Response> {
  const { bridge: b, identity: id } = a;
  const userId = await createAccessAccount(ctx.env, id, create.roleId, ctx.now, b.method);
  if (!userId) return refuseBridge(ctx, b.method, "account_exists", seen(id), a.next);
  await emitEvent(ctx.env, { kind: "account_created", userId, method: b.method, detail: { ...seen(id), role: create.roleName } }, ctx.now);
  return finish(ctx, a, (await findUserById(ctx.env, userId)) as User, where);
}
