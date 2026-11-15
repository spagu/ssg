// GET {base}/api/me — who is signed in and what they may do here. The panel
// reads it once at start to hide what the account cannot use; a hidden button
// is not a refusal — every operation checks its permission itself.
//
//   {kind: "user" | "service", user: {id, email}, permissions: ["editor-ui:read", …],
//    sessionExpiresAt, loginUrl, logoutUrl, refreshUrl}
//
// `permissions` lists this module's permissions the token grants (wildcards
// expanded, other modules' left out). The ticket's rule: an account without
// editor-ui:read gets 403 on every API route, this one included.

import type { Op } from "./ops";
import { PERM, held } from "./permissions";

// The `auth` worker's endpoints, on this host under its fixed /api/auth/
// prefix. The panel posts a logout form, and calls refresh once when an API
// call answers 401 (the access token lives minutes), then retries.
export const LOGOUT_URL = "/api/auth/logout";
export const REFRESH_URL = "/api/auth/refresh";

export interface Me {
  kind: "user" | "service";
  user: { id: string; email: string };
  permissions: string[];
  sessionExpiresAt: string;
  loginUrl: string;
  logoutUrl: string;
  refreshUrl: string;
}

export const meOp: Op<void, Me> = {
  name: "me",
  permission: PERM.read,
  run: ({ caller, auth }) => ({
    kind: caller.kind,
    user: caller.user,
    permissions: held(caller.perms),
    sessionExpiresAt: caller.expiresAt,
    loginUrl: auth.loginUrl,
    logoutUrl: LOGOUT_URL,
    refreshUrl: REFRESH_URL,
  }),
};
