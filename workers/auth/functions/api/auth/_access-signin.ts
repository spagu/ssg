// Who a verified Access token signs in (GO-139). A leading underscore keeps
// this file out of the Pages route table.
//
// The algorithm is _bridge-signin.ts's (shared with the external issuer's
// JWT, #435); Access's choices in it:
//   - the identity is ("access", <Access sub>);
//   - an EXISTING account with the address is entered only with
//     AUTH_ACCESS_LINK_BY_EMAIL=on; off (the default) → access_not_linked: the
//     person signs in another way and links Access on the account page (same
//     recent-sign-in rule as any method);
//   - an open invitation for the address is always accepted;
//   - AUTH_ACCESS_AUTO_CREATE=on → a new account with the default role only;
//   - when the token carries groups, the mapped group memberships are
//     synchronised (_access-sync.ts) BEFORE the session starts.
// Access is never assumed to have checked a second factor (whatever its own
// policy did, this worker cannot see it).

import { ACCESS_METHOD } from "./_access-config";
import type { AccessSetup } from "./_access-config";
import { identityOf } from "./_access-identity";
import type { AccessPerson } from "./_access-jwt";
import { applyAccessGroups } from "./_access-sync";
import { bridgeSignIn, refuseBridge } from "./_bridge-signin";
import type { Ctx } from "./_context";

export interface Arrival {
  setup: AccessSetup;
  person: AccessPerson;
  next: string; // a safe path, or ""
  roleId: string | null; // the default role's id when auto-creation is on
}

type Detail = Record<string, string | boolean | null>;

// refuseAccess ends an Access sign-in on the login page with ?error=<code>,
// and an audit event (reasons and addresses, never the token).
export const refuseAccess = (ctx: Ctx, error: string, detail: Detail, next: string, userId: string | null = null): Promise<Response> =>
  refuseBridge(ctx, ACCESS_METHOD, error, detail, next, userId);

export function accessSignIn(ctx: Ctx, a: Arrival): Promise<Response> {
  const { setup, person } = a;
  return bridgeSignIn(ctx, {
    identity: identityOf(person),
    next: a.next,
    bridge: {
      method: ACCESS_METHOD,
      domainsOf: ACCESS_METHOD,
      linkByEmail: setup.linkByEmail,
      invite: true,
      create: setup.autoCreate ? { roleId: a.roleId, roleName: setup.defaultRole } : null,
      codes: { emailUnusable: "access_email_unusable", notLinked: "access_not_linked" },
      via: "access_email",
      before: async (user) => {
        if (person.groups !== null) await applyAccessGroups(ctx.env, user.id, person.groups, ctx.now, "signin");
      },
    },
  });
}
