// Who a verified Access token signs in (GO-139). A leading underscore keeps
// this file out of the Pages route table.
//
// In order — the first that applies decides:
//   1. the address domain allowed (AUTH_ACCESS_ALLOWED_DOMAINS, like any
//      provider's), else refused;
//   2. ("access", sub) is linked → that account, if active (a blocked account
//      is refused whatever Access says);
//   3. the address cannot be matched (_access-identity.ts) → refused;
//   4. an EXISTING account has exactly this address → only with
//      AUTH_ACCESS_LINK_BY_EMAIL=on is the identity linked to it and the
//      account entered (audited as a link "via email"); off (the default) →
//      refused with access_not_linked: the person signs in another way and
//      links Access on the account page (same recent-sign-in rule as any
//      method);
//   5. an open invitation for the address → accepted, Access linked, the
//      invitation's roles and groups given;
//   6. AUTH_ACCESS_AUTO_CREATE=on → a new account with the default role only;
//   7. otherwise refused (not_invited) — and no users row is written.
// Then, when the token carries groups, the mapped group memberships are
// synchronised (_access-sync.ts) BEFORE the session starts, so its first token
// already reflects them; and the session is started by startSession
// (_signin.ts), the one place sessions are made — where a second factor
// (GO-140) is asked for when a role needs one: Access is never assumed to
// have checked a second factor (whatever its own policy did, this worker
// cannot see it).

import { emitEvent } from "./_audit";
import { ACCESS_METHOD } from "./_access-config";
import type { AccessSetup } from "./_access-config";
import { identityOf } from "./_access-identity";
import type { AccessPerson } from "./_access-jwt";
import { createAccessAccount } from "./_access-provision";
import { applyAccessGroups } from "./_access-sync";
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

export interface Arrival {
  setup: AccessSetup;
  person: AccessPerson;
  next: string; // a safe path, or ""
  roleId: string | null; // the default role's id when auto-creation is on
}

type Detail = Record<string, string | boolean | null>;

// refuseAccess ends an Access sign-in on the login page with ?error=<code>,
// and an audit event (reasons and addresses, never the token).
export async function refuseAccess(ctx: Ctx, error: string, detail: Detail, next: string, userId: string | null = null): Promise<Response> {
  await emitEvent(ctx.env, { kind: "signin_refused", userId, method: ACCESS_METHOD, detail: { reason: error, ...detail } }, ctx.now);
  return redirect(withParams(ctx.s.loginUrl, { error, next }));
}

const seen = (id: Identity): Detail => ({ email: id.email, emailVerified: id.emailVerified });

// finish: groups first, then the one entry every first factor goes through —
// which may answer with a second-factor challenge (GO-140) instead of a
// session; signInRedirect sends the browser to the right next page.
async function finish(ctx: Ctx, a: Arrival, user: User, location: string): Promise<Response> {
  if (a.person.groups !== null) await applyAccessGroups(ctx.env, user.id, a.person.groups, ctx.now, "signin");
  const started = await startSession(ctx, user, ACCESS_METHOD, location);
  if (started.ok !== false) {
    const identity = (await findIdentity(ctx.env, ACCESS_METHOD, a.person.subject)) as { id: string };
    await touchLogin(ctx.env, user.id, ctx.now);
    await touchIdentity(ctx.env, identity.id, ctx.now);
    await purgeStale(ctx.env, ctx.now);
  }
  return signInRedirect(ctx, started, location);
}

export async function accessSignIn(ctx: Ctx, a: Arrival): Promise<Response> {
  const id = identityOf(a.person);
  const where = a.next || ctx.s.afterLoginUrl;
  if (!domainAllowed(ctx.env, ACCESS_METHOD, id)) return refuseAccess(ctx, "domain_not_allowed", seen(id), a.next);
  const linked = await findIdentity(ctx.env, ACCESS_METHOD, id.subject);
  if (linked) {
    const user = await findUserById(ctx.env, linked.user_id);
    if (!user) return refuseAccess(ctx, "not_invited", seen(id), a.next);
    if (user.status !== "active") return refuseAccess(ctx, "account_blocked", seen(id), a.next, user.id);
    return finish(ctx, a, user, where);
  }
  if (!id.emailVerified) return refuseAccess(ctx, "access_email_unusable", seen(id), a.next);
  const existing = await findUserByEmail(ctx.env, id.email);
  if (existing) return byEmail(ctx, a, id, existing, where);
  const inv = await openByEmail(ctx.env, id.email, ctx.now);
  if (inv) return accept(ctx, a, id, inv);
  if (a.setup.autoCreate) return provision(ctx, a, id, where);
  return refuseAccess(ctx, "not_invited", seen(id), a.next);
}

// byEmail: an existing account with this address — entered only when the
// administrator opted in, and never a blocked one.
async function byEmail(ctx: Ctx, a: Arrival, id: Identity, user: User, where: string): Promise<Response> {
  if (!a.setup.linkByEmail) return refuseAccess(ctx, "access_not_linked", seen(id), a.next, user.id);
  if (user.status !== "active") return refuseAccess(ctx, "account_blocked", seen(id), a.next, user.id);
  const r = await linkIdentity(ctx.env, user.id, ACCESS_METHOD, id, ctx.now);
  if (r !== "ok") return refuseAccess(ctx, r === "in_use" ? "identity_in_use" : "provider_already_linked", seen(id), a.next, user.id);
  await emitEvent(ctx.env, { kind: "method_linked", userId: user.id, method: ACCESS_METHOD, detail: { ...seen(id), via: "email" } }, ctx.now);
  return finish(ctx, a, user, where);
}

// accept: the invitation for this address, accepted in its own transaction
// with the Access identity linked (_invites-accept.ts).
async function accept(ctx: Ctx, a: Arrival, id: Identity, inv: Invitation): Promise<Response> {
  const row = (await ctx.env.AUTH_DB.prepare("SELECT token_hash FROM invitations WHERE id = ?").bind(inv.id).first<{ token_hash: string }>()) as { token_hash: string };
  const r = await acceptByHash(ctx.env, inv, row.token_hash, { name: inv.name, passwordHash: null }, ctx.now, (userId) => [
    identityInsert(ctx.env, userId, ACCESS_METHOD, id, ctx.now, true),
  ]);
  if (r.kind === "invalid") return refuseAccess(ctx, "invitation_invalid", seen(id), a.next);
  if (r.kind === "account_exists") return refuseAccess(ctx, "account_exists", seen(id), a.next);
  const detail = { via: "access_email", invitationId: inv.id, invitedEmail: inv.email, providerEmail: id.email, emailVerified: true, mismatch: false };
  await emitEvent(ctx.env, { kind: "invitation_accepted", userId: r.userId, method: ACCESS_METHOD, detail }, ctx.now);
  await emitEvent(ctx.env, { kind: "method_linked", userId: r.userId, method: ACCESS_METHOD, detail: { ...seen(id), via: "invitation" } }, ctx.now);
  const skipped = r.skipped.roles.length + r.skipped.groups.length;
  return finish(ctx, a, (await findUserById(ctx.env, r.userId)) as User, withParams("/auth/", { welcome: "1", skipped: skipped ? String(skipped) : "" }));
}

// provision: AUTH_ACCESS_AUTO_CREATE — the default role only (checked by the
// route before this point, _access-provision.ts).
async function provision(ctx: Ctx, a: Arrival, id: Identity, where: string): Promise<Response> {
  const userId = await createAccessAccount(ctx.env, id, a.roleId as string, ctx.now);
  if (!userId) return refuseAccess(ctx, "account_exists", seen(id), a.next);
  await emitEvent(ctx.env, { kind: "account_created", userId, method: ACCESS_METHOD, detail: { ...seen(id), role: a.setup.defaultRole } }, ctx.now);
  return finish(ctx, a, (await findUserById(ctx.env, userId)) as User, where);
}
